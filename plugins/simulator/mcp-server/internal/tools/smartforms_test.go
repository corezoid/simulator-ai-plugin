package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestToolInputSchemasAreValid guards every curated tool's generated input
// schema against a JSON Schema draft 2020-12 violation that Go build/vet do NOT
// catch but the Anthropic API rejects at request time: a duplicate entry in the
// `required` array (draft 2020-12 requires its items to be unique). A value that
// must reach two request slots (path + body) is one Param with In: InPathBody —
// NOT two same-named Params; this test fails if anyone reintroduces a duplicate
// Name, pointing them back at InPathBody.
func TestToolInputSchemasAreValid(t *testing.T) {
	for _, op := range allOps() {
		tool := mcp.NewTool(op.Name, toolOptions(op, nil)...)
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Errorf("%s: marshal input schema: %v", op.Name, err)
			continue
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Errorf("%s: parse input schema: %v", op.Name, err)
			continue
		}
		seen := map[string]bool{}
		for _, name := range schema.Required {
			if seen[name] {
				t.Errorf("%s: duplicate %q in `required` — invalid JSON Schema draft 2020-12", op.Name, name)
			}
			seen[name] = true
			if _, ok := schema.Properties[name]; !ok {
				t.Errorf("%s: `required` lists %q but it has no property", op.Name, name)
			}
		}
	}
}

// TestSmartFormRuntimeOps locks in how the Smart Form runtime tools assemble
// their HTTP requests against the public pages protocol
// (/papi/1.0/pages/{accId}/{ref}/{envTitle}/{page}). accId defaults to the
// client workspace ("WS") when omitted.
func TestSmartFormRuntimeOps(t *testing.T) {
	t.Run("appGetPage builds a GET with no body", func(t *testing.T) {
		c, rec := setup(t)
		res := call(t, c, opByName(t, "appGetPage"), map[string]any{
			"ref":      "smart-contract",
			"envTitle": "production",
			"page":     "index",
		})
		if res.IsError {
			t.Fatalf("appGetPage: unexpected error result: %+v", res.Content)
		}
		if rec.method != "GET" {
			t.Errorf("method = %s, want GET", rec.method)
		}
		if want := "/pages/WS/smart-contract/production/index"; rec.path != want {
			t.Errorf("path = %s, want %s", rec.path, want)
		}
		if rec.body != nil {
			t.Errorf("GET should carry no body, got %v", rec.body)
		}
	})

	t.Run("appSendForm sends page in BOTH path and body", func(t *testing.T) {
		c, rec := setup(t)
		res := call(t, c, opByName(t, "appSendForm"), map[string]any{
			"ref":      "smart-contract",
			"envTitle":  "production",
			"page":      "terms",
			"formId":    "f_terms",
			"sectionId": "s_terms",
			"buttonId":  "next",
			"data":      map[string]any{"value": float64(50000), "currency": "USD"},
		})
		if res.IsError {
			t.Fatalf("appSendForm: unexpected error result: %+v", res.Content)
		}
		if rec.method != "POST" {
			t.Errorf("method = %s, want POST", rec.method)
		}
		// The page id is required in the path (route segment)...
		if want := "/pages/WS/smart-contract/production/terms"; rec.path != want {
			t.Errorf("path = %s, want %s", rec.path, want)
		}
		// ...and also in the body, because the /send handler reads `page` from
		// the body. This is the dual-slot behaviour of the single `page` arg.
		body, ok := rec.body.(map[string]any)
		if !ok {
			t.Fatalf("expected object body, got %T", rec.body)
		}
		if body["page"] != "terms" {
			t.Errorf("body[page] = %v, want %q", body["page"], "terms")
		}
		for _, k := range []string{"formId", "sectionId", "data"} {
			if _, ok := body[k]; !ok {
				t.Errorf("body missing key %q: %v", k, body)
			}
		}
		if _, ok := body["data"].(map[string]any); !ok {
			t.Errorf("body[data] should be an object, got %T", body["data"])
		}
	})

	t.Run("appSendForm requires formId/sectionId/data (buttonId optional)", func(t *testing.T) {
		c, _ := setup(t)
		// Omit the required body fields — the handler should refuse before any call.
		// buttonId is intentionally NOT required (the backend schema doesn't require it).
		res := call(t, c, opByName(t, "appSendForm"), map[string]any{
			"ref":      "smart-contract",
			"envTitle": "production",
			"page":     "terms",
		})
		if !res.IsError {
			t.Errorf("expected an error result when required params are missing")
		}
	})
}
