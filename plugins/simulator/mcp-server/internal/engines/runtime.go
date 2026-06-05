// Package engines holds the client-side "engine" tools ported from the legacy
// server: graph pull/push sync, layout, edge pruning, layer placements, picture
// upload (with SVG rasterisation), and dashboard chart creation. These wrap
// multi-call workflows and local computation on top of the public API.
//
// They share a small runtime config (auth header, base URL, TLS) set once via
// Configure, and are registered onto an MCP server via RegisterTools.
package engines

import (
	"context"
	"strconv"
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
// ensureAuth, cleared on environment switch via ResetAuth) and BaseUrl (changed by
// SetBaseURL when the user switches environment). Access those two through
// authHeader()/baseURL() for reads and ensureAuth/ResetAuth/Configure/SetBaseURL for
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

// authHeader returns the cached Authorization value, safe for concurrent reads.
func authHeader() string {
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

// ensureAuth refreshes the cached authorization from saved credentials and
// returns a friendly error result if the user is not authenticated yet.
func ensureAuth(ctx context.Context) *mcp.CallToolResult {
	if creds, _ := auth.Load(); creds != nil && !auth.IsExpired(creds) {
		cfgMu.Lock()
		Cfg.Authorization = creds.AuthorizationHeader()
		cfgMu.Unlock()
	}
	if authHeader() == "" {
		return mcp.NewToolResultError("[Error] not authenticated — run the `login` tool first")
	}
	return nil
}

// SysFormItem is a system form entry (used by the graph-sync form name cache).
type SysFormItem struct {
	ID          int                      `json:"id"          yaml:"id"`
	Title       string                   `json:"title"                yaml:"title"`
	Description string                   `json:"description,omitempty" yaml:"description,omitempty"`
	Fields      []map[string]interface{} `json:"fields,omitempty" yaml:"fields,omitempty"`
	Childs      []SysFormItem            `json:"childs,omitempty" yaml:"childs,omitempty"`
}

// loadSysForms is a no-op placeholder for the pull-side cosmetic form-name
// lookup. The push-side GraphSyncer has its own API-backed loadSysForms; on the
// pull side, form names in the exported YAML are best-effort and optional.
func loadSysForms() ([]SysFormItem, error) { return nil, nil }

// resolveFormIDToName returns "" — form names in exported YAML are optional
// (the formId is always present and is what push relies on).
func resolveFormIDToName(id int) string { return "" }

// resolveFormNameToID returns 0 (cache miss); callers fall back to a live API
// lookup. The legacy sys-forms cache is not ported.
func resolveFormNameToID(name string) int { return 0 }

// findFormInTree locates a form by id within a SysFormItem tree, reporting its
// parent id and whether it is a child (non-root) form.
func findFormInTree(forms []SysFormItem, formID, parentID int) (parent int, isChild, found bool) {
	for i := range forms {
		if forms[i].ID == formID {
			return parentID, parentID != 0, true
		}
		if len(forms[i].Childs) > 0 {
			if p, ch, ok := findFormInTree(forms[i].Childs, formID, forms[i].ID); ok {
				return p, ch, true
			}
		}
	}
	return 0, false, false
}

// omitEmptyFields recursively drops keys whose values are empty (nil, "",
// empty slice/map) — used to keep exported actor data tidy.
func omitEmptyFields(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		cleaned := cleanFieldValue(v)
		if !isEmptyFieldValue(cleaned) {
			result[k] = cleaned
		}
	}
	return result
}

func cleanFieldValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		c := omitEmptyFields(val)
		if len(c) == 0 {
			return nil
		}
		return c
	case []interface{}:
		if len(val) == 0 {
			return nil
		}
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = cleanFieldValue(item)
		}
		return out
	default:
		return v
	}
}

func isEmptyFieldValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	}
	return false
}

// toInt coerces a JSON tool-argument value (float64 / string / int) to int.
func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	}
	return 0
}
