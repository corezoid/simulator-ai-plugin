// Package mcpserver is the public embedding API for the simulator MCP server.
//
// It assembles the curated tool set plus engine tools on top of a configured
// *server.MCPServer and returns the server to the caller, who is free to
// serve it over any transport (stdio, SSE, Streamable HTTP, etc.).
//
// cmd/server uses this package to expose the server over stdio. External
// consumers (e.g. claude-code-api) use it to embed the same server inside
// their own process and pick a network transport.
package mcpserver

import (
	"context"
	"errors"
	"os"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultName    = "simulator"
	defaultVersion = "2.1.0"
)

// Options configures the embedded MCP server.
type Options struct {
	// Profile selects the environment profile (e.g. "prod", "local").
	// Empty falls back to SIMULATOR_PROFILE env var, then "prod".
	Profile string

	// Insecure disables TLS verification for on-prem gateways with self-signed certs.
	Insecure bool

	// Name overrides the MCP server name (default "simulator").
	Name string

	// Version overrides the MCP server version.
	Version string

	// AuthHeader returns the Authorization header value for each API call.
	// Nil falls back to reading credentials via app/auth.Load() from .env in cwd.
	// Ignored when Stateless=true (auth must arrive via request ctx instead).
	AuthHeader func() (string, error)

	// WorkspaceID overrides the workspace ID. Empty falls back to WORKSPACE_ID env var.
	// Ignored when Stateless=true (workspace arrives via request ctx instead).
	WorkspaceID string

	// Stateless switches the server into per-request mode: no .env reads or writes,
	// no process-global auth cache, and the .env-mutating helper tools
	// (login / set-workspace / set-environment) are NOT registered. Authorization,
	// workspace id and API base URL must arrive on every request via ctx using
	// apiclient.WithAuthorization / WithWorkspaceID / WithBaseURL — typically wired
	// from HTTP headers by the SSE transport's WithSSEContextFunc.
	//
	// When stateless, the same server also supports per-request actor-scoped mode:
	// a request that arrives with WithActorID(ctx, id) gets the per-actor tools/list
	// (workspace-wide tools hidden, actor identity stripped from schemas) and the
	// actor id is injected into the underlying API calls. Requests without an actor
	// id see the full curated set as before, so one server serves both modes.
	Stateless bool
}

// Info reports the resolved environment for logging / diagnostics.
type Info struct {
	Profile    string // profile name (e.g. "prod", "local")
	APIBaseURL string // public API root (incl. /papi/1.0)
	AccountURL string // OAuth2 / SA base URL
}

// New builds and returns an MCP server with every simulator tool and engine
// registered. The returned server is ready to be served over any transport.
// Info carries the resolved profile/URLs for the caller to log.
func New(opts Options) (*server.MCPServer, Info, error) {
	prof, err := config.Resolve(opts.Profile)
	if err != nil {
		return nil, Info{}, err
	}

	var (
		authHeader  func() (string, error)
		workspaceID string
	)
	if opts.Stateless {
		// In stateless mode, the default Client values are never used: auth and
		// workspace are read from request ctx on every call (set by
		// apiclient.WithAuthorization / WithWorkspaceID). We still need a non-nil
		// authHeader so Client.Do doesn't skip the header path when no ctx auth
		// is attached — return a sentinel error in that case to surface misuse.
		authHeader = func() (string, error) {
			return "", errors.New("stateless mode: Authorization header missing on request")
		}
	} else {
		authHeader = opts.AuthHeader
		if authHeader == nil {
			authHeader = defaultAuthHeader
		}
		workspaceID = opts.WorkspaceID
		if workspaceID == "" {
			workspaceID = os.Getenv("WORKSPACE_ID")
		}
	}

	client := apiclient.New(prof.APIBaseURL, workspaceID, authHeader, opts.Insecure)

	name := opts.Name
	if name == "" {
		name = defaultName
	}
	version := opts.Version
	if version == "" {
		version = defaultVersion
	}

	// In stateless mode the same server serves both full-workspace and per-actor
	// sessions; the filter switches the visible tool set based on ctx
	// (WithActorID). In stateful (stdio) mode there is only one client and no ctx
	// actor id is ever attached, so the filter is a passthrough.
	s := server.NewMCPServer(name, version, server.WithToolFilter(tools.ActorToolFilter))
	ecore.SetStateless(opts.Stateless)
	if opts.Stateless {
		tools.BuildUnified(s, client, true)
	} else {
		tools.BuildAll(s, client, prof, opts.Insecure)
	}
	engines.Configure(prof.APIBaseURL, opts.Insecure)
	engines.RegisterTools(s)
	return s, Info{
		Profile:    prof.Name,
		APIBaseURL: prof.APIBaseURL,
		AccountURL: prof.AccountURL,
	}, nil
}

// ToolCount reports how many curated API tools are registered (auth helpers excluded).
func ToolCount() int { return tools.Count() }

// IsInsecureCredentialTransport reports whether the given API base URL would
// send credentials over plaintext HTTP to a non-loopback host. Callers can use
// this to emit a startup warning.
func IsInsecureCredentialTransport(baseURL string) bool {
	return apiclient.IsInsecureCredentialTransport(baseURL)
}

// Per-request context helpers, re-exported from the internal apiclient package.
// External transports (e.g. claude-code-api's SSE wrapper) use these to attach
// the caller's Authorization header / workspace id / API base URL to ctx, so
// tool handlers running in stateless mode see the per-request values.

// WithAuthorization stores the full Authorization header value on ctx.
func WithAuthorization(ctx context.Context, value string) context.Context {
	return apiclient.WithAuthorization(ctx, value)
}

// WithWorkspaceID stores a per-request workspace id (accId) on ctx.
func WithWorkspaceID(ctx context.Context, value string) context.Context {
	return apiclient.WithWorkspaceID(ctx, value)
}

// WithBaseURL stores a per-request API base URL override on ctx.
func WithBaseURL(ctx context.Context, value string) context.Context {
	return apiclient.WithBaseURL(ctx, value)
}

// WithActorID switches the request into per-actor mode: tools/list returns only
// the actor-scoped subset (with actor identity hidden from schemas) and tool
// handlers inject the actor id where needed. Pass "" to leave the request in
// full mode. Set this from your transport (e.g. an HTTP header) before invoking
// any tool, so the model sees the actor-scoped schemas from the start.
func WithActorID(ctx context.Context, value string) context.Context {
	return apiclient.WithActorID(ctx, value)
}

// WithUIContext decodes a `control-events-context` header value (base64 JSON —
// where the user is in the Simulator UI) and stores it on ctx so tools like
// buildLink can default to the user's current view (hostOrigin, activeActor,
// activeLayer, activeGraph, workspaceId). Wire it from the transport header next
// to WithAuthorization / WithWorkspaceID. A blank or undecodable value is a no-op.
func WithUIContext(ctx context.Context, headerValue string) context.Context {
	return apiclient.WithUIContext(ctx, apiclient.ParseUIContext(headerValue))
}

func defaultAuthHeader() (string, error) {
	creds, err := auth.Load()
	if err != nil {
		return "", err
	}
	if creds == nil {
		return "", errors.New("not authenticated — run the `login` tool first")
	}
	return creds.AuthorizationHeader(), nil
}
