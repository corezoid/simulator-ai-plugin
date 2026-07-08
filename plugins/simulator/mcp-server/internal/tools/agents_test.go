package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// newAgentsServer serves a minimal fake PAPI: the system-forms list (with a
// "System" form at id 77), and echoes a tiny JSON for user/search/actor reads,
// recording the last request so tests can assert the composed path/query.
// (recordedReq is defined in skills_test.go — same package.)
func newAgentsServer(t *testing.T, hasSystemForm bool, last *recordedReq) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.path = r.URL.Path
		last.query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/forms/templates/"):
			forms := []map[string]any{
				{"id": 10, "title": "Tags", "type": "system"},
				{"id": 11, "title": "Skills", "type": "system"},
			}
			if hasSystemForm {
				forms = append(forms, map[string]any{"id": 77, "title": "System", "type": "system"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": forms})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "path": r.URL.Path})
		}
	}))
}

func callFindAgent(t *testing.T, c *apiclient.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "findAgent"
	req.Params.Arguments = args
	res, err := findAgentHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("findAgentHandler error: %v", err)
	}
	return res
}

func callGetAgent(t *testing.T, c *apiclient.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "getAgent"
	req.Params.Arguments = args
	res, err := getAgentHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("getAgentHandler error: %v", err)
	}
	return res
}

// TestResolveSystemFormID covers the title-based System-form lookup + the
// friendly error when the form is absent.
func TestResolveSystemFormID(t *testing.T) {
	var last recordedReq
	srv := newAgentsServer(t, true, &last)
	defer srv.Close()

	c := apiclient.New(srv.URL, "ws-sys", func() (string, error) { return "Simulator t", nil }, false)
	id, err := resolveSystemFormID(context.Background(), c)
	if err != nil {
		t.Fatalf("resolveSystemFormID: %v", err)
	}
	if id != 77 {
		t.Fatalf("resolved form id = %d, want 77", id)
	}

	srv2 := newAgentsServer(t, false, &last)
	defer srv2.Close()
	c2 := apiclient.New(srv2.URL, "ws-sys-missing", func() (string, error) { return "Simulator t", nil }, false)
	if _, err := resolveSystemFormID(context.Background(), c2); err == nil ||
		!strings.Contains(err.Error(), "System") {
		t.Fatalf("expected missing-form error mentioning System, got %v", err)
	}
}

func TestFindAgentEnumerateVsSearch(t *testing.T) {
	var last recordedReq
	srv := newAgentsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-find", func() (string, error) { return "Simulator t", nil }, false)

	// Empty query → enumerate members via /users/{accId}.
	res := callFindAgent(t, c, map[string]any{})
	if res.IsError {
		t.Fatalf("findAgent (empty) error: %+v", res.Content)
	}
	if last.path != "/users/ws-find" {
		t.Errorf("empty-query path = %q, want /users/ws-find", last.path)
	}

	// Non-empty query → semantic search over the System form's twins.
	callFindAgent(t, c, map[string]any{"query": "counters subsystem"})
	if !strings.HasPrefix(last.path, "/search/ws-find/") {
		t.Errorf("query path = %q, want /search/ws-find/...", last.path)
	}
	if got := last.query.Get("filters"); got != "actors" {
		t.Errorf("filters = %q, want actors", got)
	}
	if got := last.query.Get("formId"); got != "77" {
		t.Errorf("formId = %q, want 77", got)
	}
	if got := last.query.Get("searchType"); got != "semantic" {
		t.Errorf("searchType = %q, want semantic (default)", got)
	}

	// Explicit searchType passes through.
	callFindAgent(t, c, map[string]any{"query": "counters", "searchType": "text"})
	if got := last.query.Get("searchType"); got != "text" {
		t.Errorf("searchType = %q, want text", got)
	}
}

func TestFindAgentRegistryScope(t *testing.T) {
	var last recordedReq
	srv := newAgentsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-scope", func() (string, error) { return "Simulator t", nil }, false)

	// Empty query + a registry formId → enumerate that form's actors (filterActors).
	res := callFindAgent(t, c, map[string]any{"formId": "42"})
	if res.IsError {
		t.Fatalf("findAgent (formId enumerate) error: %+v", res.Content)
	}
	if last.path != "/actors_filters/42" {
		t.Errorf("formId-enumerate path = %q, want /actors_filters/42", last.path)
	}

	// Query + a registry formId → search scoped to that form, not the System form.
	callFindAgent(t, c, map[string]any{"query": "billing bot", "formId": "42"})
	if !strings.HasPrefix(last.path, "/search/ws-scope/") {
		t.Errorf("formId-search path = %q, want /search/ws-scope/...", last.path)
	}
	if got := last.query.Get("formId"); got != "42" {
		t.Errorf("formId = %q, want 42 (registry override, not System 77)", got)
	}

	// A numeric formId (JSON number, not a string) is accepted, not silently
	// dropped to the default registry.
	callFindAgent(t, c, map[string]any{"query": "billing bot", "formId": float64(42)})
	if got := last.query.Get("formId"); got != "42" {
		t.Errorf("numeric formId = %q, want 42 (coerced, not dropped to System)", got)
	}

	// Non-numeric formId is rejected the same way on the search path...
	if res := callFindAgent(t, c, map[string]any{"query": "bot", "formId": "System"}); !res.IsError {
		t.Errorf("non-numeric formId (search) should error")
	}
	// ...and on the enumerate path (empty query) — same friendly guard.
	if res := callFindAgent(t, c, map[string]any{"formId": "teams"}); !res.IsError {
		t.Errorf("non-numeric formId (enumerate) should error")
	}
}

func TestFindAgentInputGuards(t *testing.T) {
	var last recordedReq
	srv := newAgentsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-guard", func() (string, error) { return "Simulator t", nil }, false)

	// Query shorter than the /search 2-char minimum → client-side error, no request composed.
	if res := callFindAgent(t, c, map[string]any{"query": "R"}); !res.IsError {
		t.Errorf("1-char query should error before hitting /search")
	}

	// Invalid searchType → client-side error (searchAll accepts text|semantic only).
	if res := callFindAgent(t, c, map[string]any{"query": "counters", "searchType": "fuzzy"}); !res.IsError {
		t.Errorf("invalid searchType should error")
	}

	// limit above the /search page cap is clamped to 100.
	callFindAgent(t, c, map[string]any{"query": "counters", "limit": float64(150)})
	if got := last.query.Get("limit"); got != "100" {
		t.Errorf("search limit = %q, want clamped to 100", got)
	}
}

func TestGetAgentByUserAndActor(t *testing.T) {
	var last recordedReq
	srv := newAgentsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-get", func() (string, error) { return "Simulator t", nil }, false)

	// By userId → get-or-create system-actor path.
	callGetAgent(t, c, map[string]any{"userId": "123"})
	if last.path != "/actors/system/ws-get/user/123" {
		t.Errorf("by-user path = %q, want /actors/system/ws-get/user/123", last.path)
	}
	if got := last.query.Get("filter"); !strings.Contains(got, "description") {
		t.Errorf("filter must include description, got %q", got)
	}

	// By actorId → getActor path (no form resolution needed).
	callGetAgent(t, c, map[string]any{"actorId": "a4a7f284-2763-4bce-8b1b-c10bddd207ca"})
	if last.path != "/actors/a4a7f284-2763-4bce-8b1b-c10bddd207ca" {
		t.Errorf("by-actor path = %q", last.path)
	}

	// Neither → error result.
	res := callGetAgent(t, c, map[string]any{})
	if !res.IsError {
		t.Errorf("getAgent with no userId/actorId should error")
	}
}
