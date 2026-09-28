package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestUpdateFormPreservesFields guards against updateForm wiping a form's parent
// link (UAT inheritance), ref and color: PUT /forms/{formId} is a full replace,
// so fields the caller omits must be carried over from the current form, while
// explicitly passed ones win.
func TestUpdateFormPreservesFields(t *testing.T) {
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if r.URL.Path != "/forms/42" {
				t.Errorf("GET path = %q, want /forms/42", r.URL.Path)
			}
			_, _ = io.WriteString(w, `{"data":{"id":42,"parentId":7,"ref":"child","color":"#445566","picture":null,"description":"old"}}`)
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &putBody)
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "WS", func() (string, error) { return "t", nil }, false)

	var op Operation
	for _, o := range allOps() {
		if o.Name == "updateForm" {
			op = o
		}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"formId": float64(42), "title": "T", "sections": []any{}, "description": "new",
	}
	res, err := makeHandler(c, op)(context.Background(), req)
	if err != nil || res.IsError {
		t.Fatalf("call failed: %v %+v", err, res)
	}

	want := map[string]any{"parentId": float64(7), "ref": "child", "color": "#445566", "description": "new"}
	for k, v := range want {
		if putBody[k] != v {
			t.Errorf("PUT body %s = %v, want %v", k, putBody[k], v)
		}
	}
	if _, ok := putBody["picture"]; ok {
		t.Errorf("null picture must not be sent, got %v", putBody["picture"])
	}
}
