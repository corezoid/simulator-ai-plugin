package ecore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

// WorkDir returns the directory engine tools should anchor file I/O to. The
// MCP server is launched from `plugins/simulator/mcp-server` so the process
// cwd is the server source tree, not the user's session directory; the launch
// wrapper exports SIMULATOR_WORK_DIR with the original $PWD so engines can
// recover it. Falls back to "." when the env var is unset (running outside the
// plugin wrapper).
func WorkDir() string {
	if dir := os.Getenv("SIMULATOR_WORK_DIR"); dir != "" {
		return dir
	}
	return "."
}

// ResolvePath joins WorkDir with the given path elements, so callers can write
// `ecore.ResolvePath(actorID, env, name)` instead of repeatedly prefixing.
func ResolvePath(elem ...string) string {
	return filepath.Join(append([]string{WorkDir()}, elem...)...)
}

// uuidRe matches a standard UUID (case-insensitive).
var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID reports whether s is a well-formed UUID.
func IsUUID(s string) bool {
	return uuidRe.MatchString(s)
}

// RequireUUID returns an MCP error result when v is not a well-formed UUID, or
// nil when it is. It guards tool arguments that are interpolated into file
// paths (<layerId>.yaml) and API URLs — without it a value like "../../etc/x"
// or "id?admin=1" would traverse the filesystem or inject into the request
// (see security review).
func RequireUUID(name, v string) *mcp.CallToolResult {
	if !IsUUID(v) {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %s must be a valid UUID, got %q", name, v))
	}
	return nil
}

// Seg escapes a value for safe interpolation as a single URL path segment.
// IDs that originate from a graph file (e.g. graph.LayerID, actor UUIDs) are
// not boundary-validated, so escaping here prevents path/query injection if
// such an ID contains "/", "?" or "#".
func Seg(s string) string { return url.PathEscape(s) }

// PapiGET sends an authenticated GET and returns the response body.
// The ctx supplies the per-request Authorization in stateless mode.
func PapiGET(ctx context.Context, apiURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeaderForContext(ctx))
	resp, err := APIHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Check status AFTER reading: a 401/500 returns an error body that is not a
	// {"data":[...]} payload; without this guard the caller would silently parse
	// it as an empty result (e.g. an empty layer export). Matches GraphSyncer.get.
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

// PapiPOST sends an authenticated POST with a JSON body and returns the response body.
func PapiPOST(ctx context.Context, apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeaderForContext(ctx))
	req.Header.Set("Content-Type", "application/json")
	resp, err := APIHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

// PapiPUT sends an authenticated PUT with a JSON body and returns the response body.
func PapiPUT(ctx context.Context, apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeaderForContext(ctx))
	req.Header.Set("Content-Type", "application/json")
	resp, err := APIHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PUT %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}
