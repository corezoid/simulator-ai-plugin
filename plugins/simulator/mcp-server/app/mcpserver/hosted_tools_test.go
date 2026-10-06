package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// fileTools read or write the server's working directory. A hosted
// (stateless) server shares that disk between callers, so it must not offer
// them; a local (stdio) server keeps them — that is where they belong.
var fileTools = []string{"pullGraphFile", "pushGraphFile", "pullSmartForm", "pushSmartForm", "simulationSnapshot"}

func TestHostedServerOmitsFileTools(t *testing.T) {
	t.Setenv(APISecretEnv, "")

	s, _, err := New(Options{Profile: "prod", Stateless: true})
	if err != nil {
		t.Fatal(err)
	}
	tools := s.ListTools()
	for _, name := range fileTools {
		if _, ok := tools[name]; ok {
			t.Errorf("hosted server offers %s", name)
		}
	}
	for _, name := range []string{"uploadActorPicture", "simulationRun", "deploySmartForm", "getAllLayerPlacements"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("hosted server lost API tool %s", name)
		}
	}

	// localPath is refused before any file is touched.
	ctx := WithWorkspaceID(WithAuthorization(context.Background(), "Simulator test"), "ws-test")
	res, err := tools["uploadActorPicture"].Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "uploadActorPicture",
		Arguments: map[string]any{"actorId": "00000000-0000-0000-0000-000000000000", "formId": 1, "localPath": "/etc/hostname.png"},
	}})
	if err != nil || res == nil || !res.IsError || !strings.Contains(toolText(res), "not available on the hosted server") {
		t.Errorf("localPath on hosted server: %v %+v", err, res)
	}

	// Simulation file paths are refused; inline text still works the same way.
	res, err = tools["simulationCheck"].Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "simulationCheck",
		Arguments: map[string]any{"modelPath": "model.yaml"},
	}})
	if err != nil || res == nil || !res.IsError || !strings.Contains(toolText(res), "not available on the hosted server") {
		t.Errorf("modelPath on hosted server: %v %s", err, toolText(res))
	}
}

func TestLocalServerKeepsFileTools(t *testing.T) {
	t.Setenv(APISecretEnv, "")
	s, _, err := New(Options{Profile: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	tools := s.ListTools()
	for _, name := range fileTools {
		if _, ok := tools[name]; !ok {
			t.Errorf("local server lost %s", name)
		}
	}
}

func toolText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
