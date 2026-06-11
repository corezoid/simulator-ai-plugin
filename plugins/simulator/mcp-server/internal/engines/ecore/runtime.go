// Package ecore holds the shared runtime configuration and HTTP helpers used by
// all engine sub-packages (graph, smartform). It deliberately has no dependency
// on graph- or smartform-specific types.
package ecore

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// Config is the runtime configuration shared by all engine tools. Field names
// mirror the legacy Cfg so the ported code reads Cfg.<Field>.
type Config struct {
	Authorization string // full Authorization header value, e.g. "Simulator <jwt>"
	Url           string // explicit API base override (unused by default)
	BaseUrl       string // API base incl. /papi/1.0 prefix
	Insecure      bool   // skip TLS verification (self-signed gateways)
}

// Cfg is the process-wide engine configuration, populated by Configure.
//
// cfgMu guards the two fields mutated at runtime — Authorization (set per call by
// EnsureAuth, cleared on environment switch via ResetAuth) and BaseUrl (changed by
// SetBaseURL when the user switches environment). Access those two through
// AuthHeader()/baseURL() for reads and EnsureAuth/ResetAuth/Configure/SetBaseURL for
// writes, rather than touching the fields directly. Insecure and Url are set once at
// startup and read without locking.
var (
	cfgMu sync.RWMutex
	Cfg   Config
)

// Configure sets the API base URL and TLS mode for the engine tools. The auth
// header and workspace id are read per-call (from .env credentials / env) so a
// login or set-workspace mid-session takes effect without a restart.
func Configure(baseURL string, insecure bool) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	Cfg.BaseUrl = baseURL
	Cfg.Insecure = insecure
}

// statelessMu guards the stateless flag, which switches EnsureAuth / AuthHeader
// / BuildBaseURL to read exclusively from request ctx (set by apiclient.WithXxx
// helpers) instead of falling back to process-global state or auth.Load().
//
// Stateless mode is meant for embedded SSE deployments where every request
// carries its own credentials in HTTP headers and the .env-based stateful
// path (login / set-workspace / set-environment) is not used.
var (
	statelessMu sync.RWMutex
	stateless   bool
)

// SetStateless toggles the engine into stateless mode. In stateless mode the
// engine never touches .env (no auth.Load, no global token cache) — auth and
// base URL come from request ctx via apiclient.AuthorizationFromContext /
// BaseURLFromContext.
func SetStateless(on bool) {
	statelessMu.Lock()
	defer statelessMu.Unlock()
	stateless = on
}

// IsStateless reports whether the engine is in stateless (per-request ctx) mode.
func IsStateless() bool {
	statelessMu.RLock()
	defer statelessMu.RUnlock()
	return stateless
}

// WorkspaceIDForContext returns the per-request workspace id (from ctx) if set,
// otherwise the WORKSPACE_ID env var. In stateless mode the env fallback is
// suppressed so callers cannot accidentally leak process-wide state.
func WorkspaceIDForContext(ctx context.Context) string {
	if id := apiclient.WorkspaceIDFromContext(ctx); id != "" {
		return id
	}
	if IsStateless() {
		return ""
	}
	return os.Getenv("WORKSPACE_ID")
}

// SetBaseURL updates the engine API base URL at runtime (used by set-environment),
// mirroring apiclient.Client.SetBaseURL so a mid-session environment switch takes
// effect for engine tools too.
func SetBaseURL(baseURL string) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	Cfg.BaseUrl = baseURL
}

// ResetAuth clears the cached authorization (used when switching environment, so
// the next engine call re-loads credentials for the new environment).
func ResetAuth() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	Cfg.Authorization = ""
}

// AuthHeader returns the cached Authorization value, safe for concurrent reads.
//
// Prefer AuthHeaderForContext(ctx) when a ctx is available — it honours the
// per-request override stored by apiclient.WithAuthorization in stateless mode.
func AuthHeader() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return Cfg.Authorization
}

// AuthHeaderForContext returns the per-request Authorization value if present,
// otherwise the process-global cached value. Engine code that has a ctx in
// scope should call this so stateless / SSE deployments work correctly.
func AuthHeaderForContext(ctx context.Context) string {
	if v := apiclient.AuthorizationFromContext(ctx); v != "" {
		return v
	}
	if IsStateless() {
		return ""
	}
	return AuthHeader()
}

// baseURL returns the configured API base URL, safe for concurrent reads.
func baseURL() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return Cfg.BaseUrl
}

// BuildBaseURL returns the same base URL used by all other MCP tools.
//
// Prefer BuildBaseURLForContext(ctx) when a ctx is available so stateless
// deployments can override the base URL per request.
func BuildBaseURL() string {
	if Cfg.Url != "" {
		return strings.TrimSuffix(Cfg.Url, "/")
	}
	if b := baseURL(); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return "https://api.simulator.company/v/1.0"
}

// BuildBaseURLForContext returns the per-request API base URL override if set,
// otherwise falls back to the explicit Cfg.Url, the configured Cfg.BaseUrl, or
// the public default — in that order.
func BuildBaseURLForContext(ctx context.Context) string {
	if b := apiclient.BaseURLFromContext(ctx); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return BuildBaseURL()
}

// EnsureAuth refreshes the cached authorization from saved credentials and
// returns a friendly error result if the user is not authenticated yet.
//
// In stateless mode it never touches .env — auth must arrive via ctx
// (apiclient.WithAuthorization) and is not cached process-globally.
func EnsureAuth(ctx context.Context) *mcp.CallToolResult {
	if IsStateless() {
		if apiclient.AuthorizationFromContext(ctx) == "" {
			return mcp.NewToolResultError("[Error] missing Authorization header")
		}
		return nil
	}
	if creds, _ := auth.Load(); creds != nil && !auth.IsExpired(creds) {
		cfgMu.Lock()
		Cfg.Authorization = creds.AuthorizationHeader()
		cfgMu.Unlock()
	}
	if AuthHeader() == "" {
		return mcp.NewToolResultError("[Error] not authenticated — run the `login` tool first")
	}
	return nil
}
