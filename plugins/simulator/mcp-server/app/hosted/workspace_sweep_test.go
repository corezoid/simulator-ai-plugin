package hosted

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/tools"
)

// noWorkspaceMarkers are the phrases a tool's error carries when it needed a
// workspace and found none.
var noWorkspaceMarkers = []string{"no workspace", "needs a workspace", "WORKSPACE_ID", "set-workspace"}

// On the bare /mcp route the connection names no workspace, so a tool that
// needs one must declare accId. This sweeps every hosted tool instead of
// trusting a hand-kept list (hostedWorkspaceArgTools missed findAgent,
// findSkill and getSkill): each is called with only its required arguments,
// plus accId when its hosted schema declares one, against a backend that
// answers everything — and no call may fail for want of a workspace.
func TestHostedToolsNeverMissWorkspace(t *testing.T) {
	const ws = "8be56e1c-248a-44f5-86e4-d33c7177eb4c"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(backend.Close)

	s, _, err := mcpserver.New(mcpserver.Options{Profile: "prod", Stateless: true})
	if err != nil {
		t.Fatal(err)
	}
	NewHandler(s, prodLike)
	base := mcpserver.WithBaseURL(mcpserver.WithAuthorization(context.Background(), "Simulator test"), backend.URL)
	cases := map[string]map[string]any{
		// optional arguments that lead into a workspace lookup
		"getSkill":    {"ref": "some-skill"},
		"getAgent":    {"userId": "42"},
		"createActor": {"formName": "Clients"},
	}
	for name, st := range s.ListTools() {
		args := map[string]any{}
		for _, req := range st.Tool.InputSchema.Required {
			args[req] = dummyArg(req, st.Tool.InputSchema.Properties[req])
		}
		maps.Copy(args, cases[name])
		if _, ok := st.Tool.InputSchema.Properties["accId"]; ok {
			args["accId"] = ws
		}
		// what mcpserver's stateless workspaceFromArgs middleware does
		ctx, err := tools.WorkspaceArgContext(base, args)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, _ := st.Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
		cancel()
		text := resultText(res)
		for _, m := range noWorkspaceMarkers {
			if strings.Contains(text, m) {
				t.Errorf("%s fails without a connection workspace (declare accId): %s", name, text)
				break
			}
		}
	}
}

func dummyArg(name string, prop any) any {
	typ, _ := prop.(map[string]any)["type"].(string)
	switch typ {
	case "number", "integer":
		return float64(1)
	case "boolean":
		return false
	case "object":
		return map[string]any{}
	case "array":
		return []any{}
	}
	if strings.HasSuffix(name, "Id") {
		return "00000000-0000-0000-0000-000000000001"
	}
	return "x"
}

func resultText(res *mcp.CallToolResult) string {
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
