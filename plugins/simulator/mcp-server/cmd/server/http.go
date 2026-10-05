// HTTP serving mode: the same MCP server exposed over streamable HTTP for
// hosted (remote) deployments. Selected by --http <addr> or SIMULATOR_HTTP_ADDR;
// when neither is set, main falls through to the classic stdio transport and
// nothing in this file runs.
//
// The hosted server is always STATELESS and multi-tenant: it never reads or
// writes .env, keeps no process-global auth, and the .env-mutating helper tools
// (login / set-workspace / set-environment) are not registered. Credentials and
// workspace arrive on every request via HTTP headers:
//
//	Authorization:              forwarded VERBATIM to the Simulator API
//	                            ("Simulator <jwt>" or "Bearer <api key>")
//	X-Simulator-Workspace-Id:   workspace (accId) for workspace-scoped calls
//	X-Simulator-Actor-Id:       optional; switches the request into
//	                            actor-scoped mode (reduced tools/list)
//	control-events-context:     optional; base64 UI context for buildLink etc.
//
// The API base URL is fixed by --profile at startup and deliberately NOT
// overridable per request: accepting a base URL from a client header would let
// a caller redirect another caller's Authorization header to an arbitrary host.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/telemetry"
	"github.com/mark3labs/mcp-go/server"
)

// mcpEndpointPath is where the streamable-HTTP MCP endpoint is mounted.
// Load-balancer health checks go to healthzPath and never touch the MCP stack.
const (
	mcpEndpointPath = "/mcp"
	healthzPath     = "/healthz"
)

// runHTTPMode builds the stateless server and serves it over streamable HTTP.
// Mirrors main()'s stdio startup, minus everything .env / API-key related:
// mcpserver.New with Stateless true already refuses SIMULATOR_API_SECRET and
// skips registering the .env-mutating helper tools.
func runHTTPMode(addr, profile string, insecure bool) {
	s, info, err := mcpserver.New(mcpserver.Options{
		Profile:   profile,
		Insecure:  insecure,
		Version:   version,
		Stateless: true,
	})
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("simulator MCP server %s — HTTP %s%s profile=%s api=%s account=%s auth=%s",
		version, addr, mcpEndpointPath, info.Profile, info.APIBaseURL, info.AccountURL, info.AuthMode)
	log.Printf("registered %d curated API tools + engine tools (stateless: auth helpers disabled)", mcpserver.ToolCount())

	telemetry.Init("http", version)
	defer telemetry.Stop()

	if err := serveHTTP(s, addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// httpContextFunc copies the per-request headers listed in the package comment
// onto ctx, where the stateless apiclient reads them back on every API call.
// Empty headers are attached as-is: the With* helpers treat "" as unset, and a
// request without Authorization fails inside the API client with a clear
// "Authorization header missing" error rather than at the transport.
func httpContextFunc(ctx context.Context, r *http.Request) context.Context {
	ctx = mcpserver.WithAuthorization(ctx, r.Header.Get("Authorization"))
	ctx = mcpserver.WithWorkspaceID(ctx, r.Header.Get("X-Simulator-Workspace-Id"))
	ctx = mcpserver.WithActorID(ctx, r.Header.Get("X-Simulator-Actor-Id"))
	ctx = mcpserver.WithUIContext(ctx, r.Header.Get("control-events-context"))
	return ctx
}

// newHTTPHandler builds the full HTTP handler: the MCP streamable endpoint at
// mcpEndpointPath plus a trivial health endpoint. Split from serveHTTP so tests
// can drive it with httptest without binding a port.
func newHTTPHandler(s *server.MCPServer) http.Handler {
	streamable := server.NewStreamableHTTPServer(s,
		server.WithHTTPContextFunc(httpContextFunc),
		// No server-side sessions: every request is self-contained (auth via
		// headers), so any task behind the load balancer can serve any request.
		server.WithStateLess(true),
		// Periodic SSE heartbeats keep ALB idle timeouts from severing
		// long-running tool calls.
		server.WithHeartbeatInterval(30*time.Second),
	)

	mux := http.NewServeMux()
	mux.Handle(mcpEndpointPath, streamable)
	mux.HandleFunc(healthzPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// serveHTTP runs the hosted-mode server on addr until SIGINT/SIGTERM, then
// shuts down gracefully (ECS sends SIGTERM and waits before SIGKILL).
func serveHTTP(s *server.MCPServer, addr string) error {
	httpServer := &http.Server{
		Addr:    addr,
		Handler: newHTTPHandler(s),
		// Slowloris protection; no global WriteTimeout because MCP responses
		// may stream for the duration of a tool call.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Printf("received %s — shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			return err
		}
		telemetry.Stop()
		return nil
	}
}
