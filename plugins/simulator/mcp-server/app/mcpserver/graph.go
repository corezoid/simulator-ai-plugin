package mcpserver

import (
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/graph"
)

// Graph types and the PushGraphFile function are re-exported here so external
// consumers (e.g. claude-code-api's /v1/layers/push endpoint) can build and
// sync a graph without depending on the internal/ package layout.

// GraphFile, GraphActor, GraphEdge mirror the YAML data model used by the
// pushGraphFile MCP tool.
type (
	GraphFile  = graph.GraphFile
	GraphActor = graph.GraphActor
	GraphEdge  = graph.GraphEdge
)

// PushGraphResult holds the outcome of PushGraphFile.
type PushGraphResult = graph.PushGraphResult

// PushGraphFile syncs a parsed graph to the simulator API without touching the
// filesystem or environment variables. See graph.PushGraphFile for details.
func PushGraphFile(g GraphFile, workspaceID, layerID, authorization, baseURL string) (PushGraphResult, error) {
	return graph.PushGraphFile(g, workspaceID, layerID, authorization, baseURL)
}
