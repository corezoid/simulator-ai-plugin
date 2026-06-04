package tools

import (
	"context"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// allOps is the full curated operation set, in registration order.
func allOps() []Operation {
	var ops []Operation
	ops = append(ops, formOps...)
	ops = append(ops, actorOps...)
	ops = append(ops, accountOps...)
	ops = append(ops, transactionOps...)
	ops = append(ops, graphOps...)
	ops = append(ops, appOps...)
	ops = append(ops, searchOps...)
	return ops
}

// BuildAll registers every curated API tool plus the auth helpers (login,
// set-workspace) on the MCP server.
func BuildAll(s *server.MCPServer, c *apiclient.Client, prof config.Profile) {
	for _, op := range allOps() {
		register(s, c, op)
	}
	registerAuth(s, c, prof)
}

// Count reports how many curated API tools are registered (auth helpers excluded).
func Count() int { return len(allOps()) }

func registerAuth(s *server.MCPServer, c *apiclient.Client, prof config.Profile) {
	s.AddTool(
		mcp.NewTool("login",
			mcp.WithDescription("Authenticate to Simulator via OAuth2 PKCE (opens a browser). Saves the token to .env. After login, call set-workspace to choose a workspace."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			creds, err := auth.PKCEFlow(prof.AccountURL, prof.OAuthClientID, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] OAuth2 login failed: %v", err)), nil
			}
			if err := auth.Save(creds); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to save token: %v", err)), nil
			}
			return mcp.NewToolResultText("Authenticated. Token saved to .env. Now call set-workspace with your workspace id (accId)."), nil
		},
	)

	s.AddTool(
		mcp.NewTool("set-workspace",
			mcp.WithDescription("Save the active workspace id (accId) to .env so it is used as the default for all tools."),
			mcp.WithString("accId", mcp.Required(), mcp.Description("Workspace id.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			accID, ok := req.GetArguments()["accId"].(string)
			if !ok || accID == "" {
				return mcp.NewToolResultError("[Error] missing or invalid accId"), nil
			}
			if err := auth.SaveWorkspaceID(accID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to save workspace id: %v", err)), nil
			}
			c.SetWorkspaceID(accID)
			return mcp.NewToolResultText(fmt.Sprintf("Workspace saved: WORKSPACE_ID=%s", accID)), nil
		},
	)
}
