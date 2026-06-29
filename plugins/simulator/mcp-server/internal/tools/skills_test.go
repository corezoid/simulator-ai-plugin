package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// recordedReq captures the path + query of the last request the fake PAPI saw.
type recordedReq struct {
	path  string
	query url.Values
}

// newSkillsServer serves a minimal fake PAPI: the system-forms list (with a
// "Skills" form at id 42), and echoes a tiny JSON for actor reads, recording the
// last request so tests can assert the composed path/query.
func newSkillsServer(t *testing.T, hasSkillsForm bool, last *recordedReq) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.path = r.URL.Path
		last.query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/forms/templates/"):
			forms := []map[string]any{
				{"id": 10, "title": "Tags", "type": "system"},
				{"id": 11, "title": "Reactions", "type": "system"},
			}
			if hasSkillsForm {
				forms = append(forms, map[string]any{"id": 42, "title": "Skills", "type": "system"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": forms})
		default:
			// actor reads / filters — body content is irrelevant to these tests.
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "path": r.URL.Path})
		}
	}))
}

func callFindSkill(t *testing.T, c *apiclient.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "findSkill"
	req.Params.Arguments = args
	res, err := findSkillHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("findSkillHandler error: %v", err)
	}
	return res
}

func callGetSkill(t *testing.T, c *apiclient.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "getSkill"
	req.Params.Arguments = args
	res, err := getSkillHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("getSkillHandler error: %v", err)
	}
	return res
}

// TestResolveSkillsFormID covers the title-based system-form lookup + the
// friendly error when the form is absent.
func TestResolveSkillsFormID(t *testing.T) {
	var last recordedReq
	srv := newSkillsServer(t, true, &last)
	defer srv.Close()

	c := apiclient.New(srv.URL, "ws-resolve", func() (string, error) { return "Simulator t", nil }, false)
	id, err := resolveSkillsFormID(context.Background(), c)
	if err != nil {
		t.Fatalf("resolveSkillsFormID: %v", err)
	}
	if id != 42 {
		t.Fatalf("resolved form id = %d, want 42", id)
	}
	if got := last.query.Get("formTypes"); got != "system" {
		t.Errorf("formTypes query = %q, want system", got)
	}

	// Absent form → friendly error mentioning the system form.
	srv2 := newSkillsServer(t, false, &last)
	defer srv2.Close()
	c2 := apiclient.New(srv2.URL, "ws-resolve-missing", func() (string, error) { return "Simulator t", nil }, false)
	if _, err := resolveSkillsFormID(context.Background(), c2); err == nil ||
		!strings.Contains(err.Error(), "Skills") {
		t.Fatalf("expected missing-form error mentioning Skills, got %v", err)
	}
}

func TestFindSkillComposesFilterActors(t *testing.T) {
	var last recordedReq
	srv := newSkillsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-find", func() (string, error) { return "Simulator t", nil }, false)

	// Empty query → enumeration: no `search`, status=verified, hits the form's filter path.
	res := callFindSkill(t, c, map[string]any{})
	if res.IsError {
		t.Fatalf("findSkill (empty) error: %+v", res.Content)
	}
	if last.path != "/actors_filters/42" {
		t.Errorf("path = %q, want /actors_filters/42", last.path)
	}
	if got := last.query.Get("status"); got != "verified" {
		t.Errorf("status = %q, want verified", got)
	}
	if _, ok := last.query["search"]; ok {
		t.Errorf("empty query must not set search, got %v", last.query["search"])
	}

	// Non-empty query → search param set.
	callFindSkill(t, c, map[string]any{"query": "smart contract"})
	if got := last.query.Get("search"); got != "smart contract" {
		t.Errorf("search = %q, want %q", got, "smart contract")
	}
}

func TestGetSkillByRefAndID(t *testing.T) {
	var last recordedReq
	srv := newSkillsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "ws-get", func() (string, error) { return "Simulator t", nil }, false)

	// By ref → getActorByRef path on the resolved form.
	callGetSkill(t, c, map[string]any{"ref": "create-smart-contract"})
	if last.path != "/actors/ref/42/create-smart-contract" {
		t.Errorf("by-ref path = %q, want /actors/ref/42/create-smart-contract", last.path)
	}
	if got := last.query.Get("filter"); !strings.Contains(got, "description") {
		t.Errorf("filter must include description, got %q", got)
	}

	// By id → getActor path (no form resolution needed).
	callGetSkill(t, c, map[string]any{"id": "a4a7f284-2763-4bce-8b1b-c10bddd207ca"})
	if last.path != "/actors/a4a7f284-2763-4bce-8b1b-c10bddd207ca" {
		t.Errorf("by-id path = %q", last.path)
	}

	// Neither → error result.
	res := callGetSkill(t, c, map[string]any{})
	if !res.IsError {
		t.Errorf("getSkill with no ref/id should error")
	}
}
