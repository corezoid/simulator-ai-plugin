package tools

import (
	"context"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// When accId is omitted, the per-request workspace from the context (hosted) wins
// over the Client's default workspace. setup()'s client default is "WS".
func TestAccIdDefaultsFromContextWorkspace(t *testing.T) {
	c, rec := setup(t)
	ctx := apiclient.WithWorkspaceID(context.Background(), "CTXWS")

	var req mcp.CallToolRequest
	req.Params.Name = "getCurrencies"
	req.Params.Arguments = map[string]any{} // no accId → defaulted

	res, err := makeHandler(c, opByName(t, "getCurrencies"))(ctx, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	if rec.path != "/currencies/CTXWS" {
		t.Errorf("path = %s, want /currencies/CTXWS (ctx workspace overrides client default)", rec.path)
	}
}
