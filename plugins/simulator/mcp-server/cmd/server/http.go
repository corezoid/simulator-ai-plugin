// HTTP serving mode: the same MCP server exposed over streamable HTTP for
// hosted (remote) deployments. Selected by --http <addr> or SIMULATOR_HTTP_ADDR;
// when neither is set, main falls through to the classic stdio transport and
// nothing in this file runs.
//
// Routes, OAuth discovery and auth handling live in app/hosted. Settings:
//
//	SIMULATOR_RESOURCE_URL              public URL of this server, e.g.
//	                                    https://mcp.simulator.company; enables
//	                                    RFC 9728 discovery (no default)
//	SIMULATOR_AUTH_SERVER_URL           OAuth authorization server (default:
//	                                    the profile's account URL)
//	SIMULATOR_RESOLVER_ACCOUNT_URL      Account service for per-workspace API
//	                                    URL resolution (default: the profile's
//	                                    account URL; "off" disables it)
//	SIMULATOR_RESOLVER_ALLOWED_ORIGINS  comma-separated extra origins a resolved
//	                                    API URL may use (on-prem Simulators)
//	OPENAI_APPS_CHALLENGE               domain-verification token served at
//	                                    /.well-known/openai-apps-challenge
//
// The fallback API URL is the profile's (--profile), never a request header.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/hosted"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/telemetry"
)

// runHTTPMode builds the stateless server and serves it over streamable HTTP.
// mcpserver.New with Stateless true refuses SIMULATOR_API_SECRET and leaves
// out the .env-mutating helper tools.
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
	cfg := hostedConfig(info, insecure)
	log.Printf("simulator MCP server %s — HTTP %s/mcp profile=%s api=%s auth=%s oauth-resource=%q resolver=%q",
		version, addr, info.Profile, info.APIBaseURL, info.AuthMode, cfg.ResourceURL, cfg.AccountURL)
	log.Printf("registered %d curated API tools + engine tools (stateless: auth helpers disabled)", mcpserver.ToolCount())

	telemetry.Init("http", version)
	defer telemetry.Stop()

	if err := serveHTTP(hosted.NewHandler(s, cfg), addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// hostedConfig reads the hosted settings, defaulting to the resolved profile.
func hostedConfig(info mcpserver.Info, insecure bool) hosted.Config {
	cfg := hosted.Config{
		ResourceURL:   strings.TrimSpace(os.Getenv("SIMULATOR_RESOURCE_URL")),
		AuthServerURL: envOr("SIMULATOR_AUTH_SERVER_URL", info.AccountURL),
		AccountURL:    envOr("SIMULATOR_RESOLVER_ACCOUNT_URL", info.AccountURL),
		FallbackURL:   info.APIBaseURL,
		Insecure:      insecure,

		OpenAIAppsChallenge: strings.TrimSpace(os.Getenv("OPENAI_APPS_CHALLENGE")),
	}
	if strings.EqualFold(cfg.AccountURL, "off") {
		cfg.AccountURL = ""
	}
	for _, o := range strings.Split(os.Getenv("SIMULATOR_RESOLVER_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.ResolverAllowedOrigins = append(cfg.ResolverAllowedOrigins, o)
		}
	}
	return cfg
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// serveHTTP runs handler on addr until SIGINT/SIGTERM, then shuts down
// gracefully (Kubernetes sends SIGTERM and waits before SIGKILL).
func serveHTTP(handler http.Handler, addr string) error {
	httpServer := &http.Server{
		Addr:    addr,
		Handler: http.MaxBytesHandler(handler, maxRequestBytes),
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
		return httpServer.Shutdown(ctx)
	}
}

// maxRequestBytes caps a request body. Large graph pushes stay well below it.
const maxRequestBytes = 32 << 20
