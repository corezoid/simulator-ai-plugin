package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// A pre-bound param is hidden from the tool schema and injected at call time, so
// the model never sets it and a Required param (actorId) is satisfied by the bind.
func TestBoundArgHiddenAndInjected(t *testing.T) {
	op := opByName(t, "getActor") // GET /actors/{actorId}, actorId InPath + Required
	// A real bind carries a full actor UUID (getActor validates the format).
	bound := map[string]any{"actorId": "00000000-0000-4000-8000-0000000b0001"}

	// (1) hidden — the marshaled input schema has no actorId property.
	tool := mcp.NewTool(op.Name, toolOptions(op, bound)...)
	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal tool: %v", err)
	}
	if strings.Contains(string(raw), `"actorId"`) {
		t.Errorf("bound actorId must be hidden from the schema; tool = %s", raw)
	}

	// (2) injected — a call with NO actorId still resolves /actors/bound-1
	//     (and the Required actorId is satisfied, i.e. no error).
	c, rec := setup(t)
	var req mcp.CallToolRequest
	req.Params.Name = op.Name
	req.Params.Arguments = map[string]any{} // model sends nothing
	res, err := makeHandlerBound(c, op, bound)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error (bound actorId should satisfy Required): %+v", res.Content)
	}
	if rec.path != "/actors/00000000-0000-4000-8000-0000000b0001" {
		t.Errorf("path = %s, want /actors/00000000-0000-4000-8000-0000000b0001 (bound actorId injected)", rec.path)
	}
}
