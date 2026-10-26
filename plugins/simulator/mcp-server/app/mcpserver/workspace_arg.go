package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/tools"
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
		ctx, err := tools.WorkspaceArgContext(ctx, req.GetArguments())
		if err != nil {
			return mcp.NewToolResultError("[Error] " + err.Error()), nil
		}
		return next(ctx, req)
	}
}
