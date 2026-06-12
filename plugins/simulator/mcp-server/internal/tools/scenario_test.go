package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestAccIdEmptyStringDefaulting verifies an explicit accId="" also falls back
// to the workspace — MCP Inspector forces a value into required-looking fields,
// so the empty string is the common "I have nothing" signal and must not leak
// into the request path (which would resolve to /forms/templates/).
func TestAccIdEmptyStringDefaulting(t *testing.T) {
	c, rec := setup(t)
	res := call(t, c, opByName(t, "getCurrencies"), map[string]any{"accId": ""})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if rec.path != "/currencies/WS" {
		t.Errorf("path = %s, want /currencies/WS (empty accId defaulted to workspace)", rec.path)
	}
}

// TestAccIdDefaultingFromContext verifies a per-request workspace id attached
// via apiclient.WithWorkspaceID (stateless / URL-scoped deployments) reaches
// the request when accId is omitted or empty.
func TestAccIdDefaultingFromContext(t *testing.T) {
	c, rec := setup(t)
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"accId": ""}
	ctx := apiclient.WithWorkspaceID(context.Background(), "CTXWS")
	res, err := makeHandler(c, opByName(t, "getCurrencies"))(ctx, req)
	if err != nil {
		t.Fatalf("handler returned go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if rec.path != "/currencies/CTXWS" {
		t.Errorf("path = %s, want /currencies/CTXWS (accId defaulted to ctx workspace)", rec.path)
	}
}

// TestAccIdNotRequiredInSchema verifies accId is exposed as optional in the
// tool input schema even when declared Required at the Operation level — the
// handler defaults it from the configured / ctx workspace, so marking the
// field required would force callers (Inspector, models) to invent a value
// and the fallback would never fire.
func TestAccIdNotRequiredInSchema(t *testing.T) {
	op := opByName(t, "getCurrencies")
	tool := mcp.NewTool(op.Name, toolOptions(op, nil)...)
	for _, r := range tool.InputSchema.Required {
		if r == "accId" {
			t.Errorf("accId must not be in InputSchema.Required (let the handler default it)")
		}
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

// TestCreateActorResolvesFormName verifies createActor accepts a friendly form
// name, looks it up, and POSTs to the resolved numeric formId.
func TestCreateActorResolvesFormName(t *testing.T) {
	var actorPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/forms/templates/") {
			_, _ = w.Write([]byte(`{"data":[{"id":5,"title":"Task"},{"id":334704,"title":"Car"}]}`))
			return
		}
		actorPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	c := apiclient.New(srv.URL, "WS", func() (string, error) { return "t", nil }, false)

	res := call(t, c, opByName(t, "createActor"),
		map[string]any{"formName": "Car", "data": map[string]any{"vin": "X"}})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	if actorPath != "/actors/actor/334704" {
		t.Errorf("actor POST path = %q, want /actors/actor/334704 (formName resolved)", actorPath)
	}
}

// TestOptionalBooleanQueryOmittedWhenFalse verifies an optional boolean query
// flag is dropped when false and sent when true.
func TestOptionalBooleanQueryOmittedWhenFalse(t *testing.T) {
	c, rec := setup(t)
	call(t, c, opByName(t, "getForms"), map[string]any{"withRelations": false})
	if rec.query != "" {
		t.Errorf("query = %q, want empty (false flag omitted)", rec.query)
	}

	c2, rec2 := setup(t)
	call(t, c2, opByName(t, "getForms"), map[string]any{"withRelations": true})
	if rec2.query != "withRelations=true" {
		t.Errorf("query = %q, want withRelations=true", rec2.query)
	}
}

// TestKeepFalseQueryFlagSentWhenFalse verifies a KeepFalse boolean query flag
// (one whose backend default is true, e.g. saveAccessRules' `recursive`) is sent
// even when explicitly false — so an opt-out is honoured rather than dropped and
// silently overridden by the backend default.
func TestKeepFalseQueryFlagSentWhenFalse(t *testing.T) {
	c, rec := setup(t)
	call(t, c, opByName(t, "saveAccessRules"), map[string]any{
		"objType":   "actor",
		"objId":     "a1",
		"rules":     []any{map[string]any{"action": "delete", "data": map[string]any{"userId": float64(1)}}},
		"recursive": false,
	})
	if rec.path != "/access_rules/actor/a1" {
		t.Errorf("path = %q, want /access_rules/actor/a1", rec.path)
	}
	if rec.query != "recursive=false" {
		t.Errorf("query = %q, want recursive=false (KeepFalse flag must be sent)", rec.query)
	}
}

// TestWireDecouplesArgFromBodyKey verifies a Param.Wire override sends the body
// field under its wire name while the MCP arg keeps a distinct name — the reactions
// case where the root actor is the path `{actorId}` and the reaction's own id is the
// body `actorId`, exposed as the `reactionId` argument.
func TestWireDecouplesArgFromBodyKey(t *testing.T) {
	c, rec := setup(t)
	call(t, c, opByName(t, "updateReaction"), map[string]any{
		"actorId":     "root-1",
		"reactionId":  "rx-9",
		"description": "edited",
	})
	if rec.path != "/reactions/root-1" {
		t.Errorf("path = %q, want /reactions/root-1 (root actor in path)", rec.path)
	}
	m, ok := rec.body.(map[string]any)
	if !ok {
		t.Fatalf("expected object body, got %T", rec.body)
	}
	if m["actorId"] != "rx-9" {
		t.Errorf("body actorId = %v, want rx-9 (reactionId mapped to wire name actorId)", m["actorId"])
	}
	if _, leaked := m["reactionId"]; leaked {
		t.Errorf("body should not carry the MCP arg name reactionId: %v", m)
	}
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
