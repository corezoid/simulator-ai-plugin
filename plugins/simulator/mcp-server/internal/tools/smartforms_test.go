package tools

import (
	"encoding/json"
	"net/url"
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
			"ref":       "smart-contract",
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

	// A Smart Form carries per-session state (a token, a record id) in the page
	// `query`: a 302 answers {nextPage, query} and the next page's /get reads it as
	// body.query.*. The keys are app-defined, so the tool takes one object and
	// InQueryMap flattens it — an InQuery param would have sent the whole object as
	// a single value and the session would silently never arrive.
	t.Run("appGetPage flattens the query object into the URL", func(t *testing.T) {
		c, rec := setup(t)
		res := call(t, c, opByName(t, "appGetPage"), map[string]any{
			"ref":      "chudo-market",
			"envTitle": "develop",
			"page":     "history",
			"query":    map[string]any{"token": "tok 1", "cardCode": "777", "page": float64(2)},
		})
		if res.IsError {
			t.Fatalf("appGetPage: unexpected error result: %+v", res.Content)
		}
		if want := "/pages/WS/chudo-market/develop/history"; rec.path != want {
			t.Errorf("path = %s, want %s", rec.path, want)
		}
		q, err := url.ParseQuery(rec.query)
		if err != nil {
			t.Fatalf("parse query %q: %v", rec.query, err)
		}
		for k, want := range map[string]string{"token": "tok 1", "cardCode": "777", "page": "2"} {
			if got := q.Get(k); got != want {
				t.Errorf("query[%s] = %q, want %q (raw: %q)", k, got, want, rec.query)
			}
		}
		// The object itself must NOT appear as one opaque value.
		if q.Get("query") != "" {
			t.Errorf("query object leaked as a single %q key: %q", "query", rec.query)
		}
	})

	t.Run("appGetPage rejects a non-object query", func(t *testing.T) {
		c, _ := setup(t)
		res := call(t, c, opByName(t, "appGetPage"), map[string]any{
			"ref":      "chudo-market",
			"envTitle": "develop",
			"page":     "index",
			"query":    "token=abc",
		})
		if !res.IsError {
			t.Errorf("expected an error result when query is a string, not an object")
		}
	})

	t.Run("appGetPage skips blank keys and nil values", func(t *testing.T) {
		c, rec := setup(t)
		res := call(t, c, opByName(t, "appGetPage"), map[string]any{
			"ref":      "chudo-market",
			"envTitle": "develop",
			"page":     "index",
			"query":    map[string]any{"": "dropped", "token": nil, "keep": "yes"},
		})
		if res.IsError {
			t.Fatalf("appGetPage: unexpected error result: %+v", res.Content)
		}
		q, _ := url.ParseQuery(rec.query)
		if q.Get("keep") != "yes" {
			t.Errorf("query[keep] = %q, want %q", q.Get("keep"), "yes")
		}
		// A nil would otherwise render as the literal string "null".
		if _, present := q["token"]; present {
			t.Errorf("nil value should be omitted, got %q", rec.query)
		}
		if len(q) != 1 {
			t.Errorf("expected exactly 1 query key, got %d (%q)", len(q), rec.query)
		}
	})

	// /send takes the query on the URL, not in the body: the handler builds
	// `{ ...body, query, context }` with `query` = req.query, so a body-borne
	// `query` is clobbered by the (empty) URL query and the session is dropped.
	t.Run("appSendForm carries query in the URL, not the body", func(t *testing.T) {
		c, rec := setup(t)
		res := call(t, c, opByName(t, "appSendForm"), map[string]any{
			"ref":       "chudo-market",
			"envTitle":  "develop",
			"page":      "stores",
			"formId":    "geo",
			"sectionId": "body",
			"buttonId":  "find_btn",
			"data":      map[string]any{"lat": "50.0466"},
			"query":     map[string]any{"token": "tok"},
		})
		if res.IsError {
			t.Fatalf("appSendForm: unexpected error result: %+v", res.Content)
		}
		q, err := url.ParseQuery(rec.query)
		if err != nil {
			t.Fatalf("parse query %q: %v", rec.query, err)
		}
		if got := q.Get("token"); got != "tok" {
			t.Errorf("query[token] = %q, want %q (raw: %q)", got, "tok", rec.query)
		}
		if q.Get("query") != "" {
			t.Errorf("query object leaked as a single %q key: %q", "query", rec.query)
		}
		body, ok := rec.body.(map[string]any)
		if !ok {
			t.Fatalf("expected object body, got %T", rec.body)
		}
		// A body-borne `query` would be silently overwritten by req.query, so it
		// must NOT be sent there — a stale/duplicate key would only mislead.
		if _, present := body["query"]; present {
			t.Errorf("body must not carry `query` (the handler overwrites it with req.query): %v", body)
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
