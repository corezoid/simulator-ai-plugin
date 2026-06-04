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
	ops = append(ops, workspaceOps...)
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
			return mcp.NewToolResultText("Authenticated. Token saved to .env. Next: call getWorkspaces to list your workspaces, show them to the user to pick one, then call set-workspace (by accId or name)."), nil
		},
	)

	s.AddTool(
		mcp.NewTool("set-workspace",
			mcp.WithDescription("Save the active workspace to .env as the default for all tools. Pass `accId`, or `name` to resolve it among your workspaces (list them with getWorkspaces) — so you can pick a workspace without knowing its id."),
			mcp.WithString("accId", mcp.Description("Workspace id. Provide accId or name.")),
			mcp.WithString("name", mcp.Description("Workspace name — resolved to its id among your workspaces. Provide accId or name.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			accID, _ := args["accId"].(string)
			if accID == "" {
				name, _ := args["name"].(string)
				if name == "" {
					return mcp.NewToolResultError("[Error] provide accId or name (use getWorkspaces to list your workspaces)"), nil
				}
				resolved, err := resolveWorkspaceName(ctx, c, name)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] %v", err)), nil
				}
				accID = resolved
			}
			if err := auth.SaveWorkspaceID(accID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to save workspace id: %v", err)), nil
			}
			c.SetWorkspaceID(accID)
			return mcp.NewToolResultText(fmt.Sprintf("Workspace saved: WORKSPACE_ID=%s", accID)), nil
		},
	)
}
