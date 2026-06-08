// Package ecore holds the shared runtime configuration and HTTP helpers used by
// all engine sub-packages (graph, smartform). It deliberately has no dependency
// on graph- or smartform-specific types.
package ecore

import (
	"context"
	"strings"
	"sync"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
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
func AuthHeader() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return Cfg.Authorization
}

// baseURL returns the configured API base URL, safe for concurrent reads.
func baseURL() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return Cfg.BaseUrl
}

// BuildBaseURL returns the same base URL used by all other MCP tools.
func BuildBaseURL() string {
	if Cfg.Url != "" {
		return strings.TrimSuffix(Cfg.Url, "/")
	}
	if b := baseURL(); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return "https://api.simulator.company/v/1.0"
}

// EnsureAuth refreshes the cached authorization from saved credentials and
// returns a friendly error result if the user is not authenticated yet.
func EnsureAuth(ctx context.Context) *mcp.CallToolResult {
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
