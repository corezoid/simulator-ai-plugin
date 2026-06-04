package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

type recorded struct {
	method string
	path   string
	query  string
	body   any
}

func setup(t *testing.T) (*apiclient.Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.query = r.Method, r.URL.Path, r.URL.RawQuery
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &rec.body)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	c := apiclient.New(srv.URL, "WS", func() (string, error) { return "Simulator t", nil }, false)
	return c, rec
}

func opByName(t *testing.T, name string) Operation {
	t.Helper()
	for _, op := range allOps() {
		if op.Name == name {
			return op
		}
	}
	t.Fatalf("operation %q not found", name)
	return Operation{}
}

func call(t *testing.T, c *apiclient.Client, op Operation, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = op.Name
	req.Params.Arguments = args
	res, err := makeHandler(c, op)(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler returned go error: %v", op.Name, err)
	}
	return res
}

// TestCuratedSetRegisters is a sanity check on the curated catalogue size + uniqueness.
func TestCuratedSetRegisters(t *testing.T) {
	ops := allOps()
	if len(ops) < 25 {
		t.Errorf("expected a sizeable curated set, got %d", len(ops))
	}
	seen := map[string]bool{}
	for _, op := range ops {
		if seen[op.Name] {
			t.Errorf("duplicate operation name %q", op.Name)
		}
		seen[op.Name] = true
		if op.Method == "" || op.Path == "" {
			t.Errorf("operation %q missing method/path", op.Name)
		}
	}
}

// TestScenarios drives one representative tool per core scenario end-to-end through
// the handler against a mock server, asserting the HTTP request is built correctly.
func TestScenarios(t *testing.T) {
	cases := []struct {
		op       string
		args     map[string]any
		method   string
		path     string // expected resolved path
		bodyKey  string // a key expected in the JSON body (object body), optional
		rootBody bool   // body is an array (InBodyRoot)
	}{
		{"createForm", map[string]any{"isTemplate": true, "title": "Car", "sections": []any{}}, "POST", "/forms/WS/true", "title", false},
		{"createActor", map[string]any{"formId": float64(334704), "data": map[string]any{"vin": "X"}, "title": "Camry"}, "POST", "/actors/actor/334704", "data", false},
		{"createAccount", map[string]any{"actorId": "a1", "nameId": "cash", "currencyId": float64(1)}, "POST", "/accounts/a1", "nameId", false},
		{"createTransaction", map[string]any{"accountId": "acc1", "amount": float64(450)}, "POST", "/transactions/acc1", "amount", false},
		{"createLink", map[string]any{"source": "s", "target": "t", "edgeTypeId": float64(7)}, "POST", "/actors/link/WS", "source", false},
		{"massLink", map[string]any{"links": []any{map[string]any{"source": "s", "target": "t"}}}, "POST", "/actors/mass_links/WS", "", true},
		{"createApplication", map[string]any{"ref": "app", "title": "App", "corezoidCredentials": map[string]any{}}, "POST", "/applications/WS", "ref", false},
		{"createSmartForm", map[string]any{"fileUrl": "u", "smartFormRef": "r", "title": "T", "ref": "x"}, "POST", "/smart_forms/WS", "fileUrl", false},
		{"getForm", map[string]any{"formId": float64(12)}, "GET", "/forms/12", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			c, rec := setup(t)
			res := call(t, c, opByName(t, tc.op), tc.args)
			if res.IsError {
				t.Fatalf("%s: unexpected error result: %+v", tc.op, res.Content)
			}
			if rec.method != tc.method {
				t.Errorf("method = %s, want %s", rec.method, tc.method)
			}
			if rec.path != tc.path {
				t.Errorf("path = %s, want %s", rec.path, tc.path)
			}
			if tc.rootBody {
				if _, ok := rec.body.([]any); !ok {
					t.Errorf("expected array body, got %T", rec.body)
				}
			} else if tc.bodyKey != "" {
				m, ok := rec.body.(map[string]any)
				if !ok {
					t.Fatalf("expected object body, got %T", rec.body)
				}
				if _, ok := m[tc.bodyKey]; !ok {
					t.Errorf("body missing key %q: %v", tc.bodyKey, m)
				}
			}
		})
	}
}

// TestAccIdDefaulting verifies an omitted accId path param falls back to the workspace.
func TestAccIdDefaulting(t *testing.T) {
	c, rec := setup(t)
	res := call(t, c, opByName(t, "getCurrencies"), map[string]any{}) // no accId supplied
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if rec.path != "/currencies/WS" {
		t.Errorf("path = %s, want /currencies/WS (accId defaulted to workspace)", rec.path)
	}
}

// TestMissingRequiredParam verifies a missing required param yields an error result.
func TestMissingRequiredParam(t *testing.T) {
	c, _ := setup(t)
	res := call(t, c, opByName(t, "createActor"), map[string]any{"formId": float64(1)}) // missing required data
	if !res.IsError {
		t.Error("expected error result for missing required 'data'")
	}
}

// TestEmptyObjectBodySent verifies a POST whose optional body fields are all
// omitted still sends "{}" (so an object-body schema does not reject it).
func TestEmptyObjectBodySent(t *testing.T) {
	c, rec := setup(t)
	res := call(t, c, opByName(t, "createTransaction"), map[string]any{"accountId": "acc1"})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	m, ok := rec.body.(map[string]any)
	if !ok {
		t.Fatalf("expected an object body ({}), got %T", rec.body)
	}
	if len(m) != 0 {
		t.Errorf("expected empty object body, got %v", m)
	}
}

// TestConcurrentWorkspaceAccess exercises the WorkspaceID read/write paths under
// -race: set-workspace writes while handlers read the default workspace.
func TestConcurrentWorkspaceAccess(t *testing.T) {
	// Dedicated non-recording server so the only shared mutable state under test
	// is the client's workspaceID (the field whose access we are validating).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	c := apiclient.New(srv.URL, "WS", func() (string, error) { return "t", nil }, false)
	h := makeHandler(c, opByName(t, "getCurrencies")) // reads c.WorkspaceID() when accId omitted
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.SetWorkspaceID("X") }()
		go func() {
			defer wg.Done()
			var req mcp.CallToolRequest
			req.Params.Arguments = map[string]any{}
			_, _ = h(context.Background(), req)
		}()
	}
	wg.Wait()
}

// TestQueryParam verifies query params are forwarded.
func TestQueryParam(t *testing.T) {
	c, rec := setup(t)
	res := call(t, c, opByName(t, "getForms"), map[string]any{"limit": float64(5)})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if rec.query != "limit=5" {
		t.Errorf("query = %q, want limit=5", rec.query)
	}
}
