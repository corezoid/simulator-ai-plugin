package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

// workspaceFromArgs lets a stateless call name its workspace with an accId
// argument when the connection does not (the bare /mcp route has no
// workspace). Curated operations already read accId themselves; engine tools
// (pictures, charts, graph export/import) read the workspace from ctx only,
// so without this they could not run on that route. A workspace set by the
// connection always wins, and the API still checks the caller's token
// against it.
func workspaceFromArgs(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if apiclient.WorkspaceIDFromContext(ctx) == "" {
			if acc, ok := req.GetArguments()["accId"].(string); ok && acc != "" {
				if r := ecore.RequireUUID("accId", acc); r != nil {
					return r, nil
				}
				ctx = apiclient.WithWorkspaceID(ctx, acc)
			}
		}
		return next(ctx, req)
	}
}
