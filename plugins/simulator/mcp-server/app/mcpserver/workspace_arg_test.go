package mcpserver

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

func TestWorkspaceFromArgs(t *testing.T) {
	const ws = "8be56e1c-248a-44f5-86e4-d33c7177eb4c"
	var seen string
	h := workspaceFromArgs(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		seen = apiclient.WorkspaceIDFromContext(ctx)
		return mcp.NewToolResultText("ok"), nil
	})
	call := func(ctx context.Context, args map[string]any) *mcp.CallToolResult {
		res, _ := h(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "x", Arguments: args}})
		return res
	}

	call(context.Background(), map[string]any{"accId": ws})
	if seen != ws {
		t.Fatalf("accId not used when the connection names no workspace: %q", seen)
	}
	const conn = "11111111-1111-1111-1111-111111111111"
	call(apiclient.WithWorkspaceID(context.Background(), conn), map[string]any{"accId": ws})
	if seen != conn {
		t.Fatalf("connection workspace overridden by an argument: %q", seen)
	}
	seen = "untouched"
	if res := call(context.Background(), map[string]any{"accId": "../../etc"}); res == nil || !res.IsError || seen != "untouched" {
		t.Fatal("non-UUID accId must be refused before the handler runs")
	}
}
