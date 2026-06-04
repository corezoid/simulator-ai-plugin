package engines

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestRegisterToolsNoPanic ensures all engine tool definitions are well-formed
// and register cleanly on a fresh MCP server.
func TestRegisterToolsNoPanic(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.0")
	RegisterTools(s) // panics if any mcp.NewTool definition is malformed
}

// TestBuildBaseURL verifies the engine base URL comes from the configured profile.
func TestBuildBaseURL(t *testing.T) {
	Configure("http://localhost:9000/papi/1.0", false)
	if got := buildBaseURL(); got != "http://localhost:9000/papi/1.0" {
		t.Errorf("buildBaseURL() = %q, want the configured base", got)
	}
}
