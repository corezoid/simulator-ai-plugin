// Package engines is the registration layer for all engine tools.
// Domain logic lives in sub-packages: graph (graph sync, layout, chart, upload)
// and smartform (CDU create/pull/push, releases, file history).
package engines

import (
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/graph"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/smartform"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools registers all engine tools on the MCP server.
func RegisterTools(s *server.MCPServer) {
	graph.Register(s)
	smartform.Register(s)
}
