package ecore

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

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
func PapiGET(apiURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeader())
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
func PapiPOST(apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeader())
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
func PapiPUT(apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", AuthHeader())
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
