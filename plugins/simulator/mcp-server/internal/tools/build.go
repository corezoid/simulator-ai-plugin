package tools

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines"
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
	// appOps (applications & smart forms) are intentionally NOT registered at
	// this stage — only the Smart Forms documentation + public spec are shipped,
	// not the tools. Re-enable by appending appOps here again (see apps.go).
	_ = appOps // keep referenced while deregistered
	ops = append(ops, searchOps...)
	ops = append(ops, workspaceOps...)
	return ops
}

// BuildAll registers every curated API tool plus the auth helpers (set-environment,
// login, set-workspace) on the MCP server. insecure is threaded through so the
// set-environment tool's public-config probe honours the same TLS mode as the client.
func BuildAll(s *server.MCPServer, c *apiclient.Client, prof config.Profile, insecure bool) {
	for _, op := range allOps() {
		register(s, c, op)
	}
	registerAuth(s, c, prof, insecure)
}

// Count reports how many curated API tools are registered (auth helpers excluded).
func Count() int { return len(allOps()) }

func registerAuth(s *server.MCPServer, c *apiclient.Client, prof config.Profile, insecure bool) {
	registerSetEnvironment(s, c, prof, insecure)

	s.AddTool(
		mcp.NewTool("login",
			mcp.WithDescription("Authenticate to Simulator via OAuth2 PKCE (opens a browser). Saves the token to .env. Run set-environment first if you haven't chosen an environment. After login, call set-workspace to choose a workspace."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Prefer the account URL derived by set-environment (saved as ACCOUNT_URL)
			// over the startup profile default, so login follows the chosen environment.
			accountURL := firstNonEmpty(os.Getenv("ACCOUNT_URL"), prof.AccountURL)
			creds, err := auth.PKCEFlow(accountURL, prof.OAuthClientID, nil)
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

// registerSetEnvironment adds the set-environment tool: it lets the user pick a
// Simulator environment (a cloud preset or a custom/local URL) before authenticating.
// It derives the correct OAuth account URL from that gateway's public config
// (getConfigReq → saUrl), persists the choice, and clears any existing token +
// workspace so the user re-authenticates against the newly chosen environment.
func registerSetEnvironment(s *server.MCPServer, c *apiclient.Client, prof config.Profile, insecure bool) {
	// The local (localhost:9000) preset is offered only in a local-dev session
	// (startup profile "local"); regular cloud users see only mw / sim.
	presets := config.OfferedEnvironments(prof.Name == "local")
	var presetLines strings.Builder
	for _, e := range presets {
		fmt.Fprintf(&presetLines, "%s = %s; ", e.Name, e.APIBaseURL)
	}
	names := presetNames(presets)

	s.AddTool(
		mcp.NewTool("set-environment",
			mcp.WithDescription("Choose the Simulator environment to work with BEFORE login. Pass `preset` for a listed gateway ("+strings.TrimRight(presetLines.String(), "; ")+") or `url` for a custom / on-prem / local server (host or full URL; /papi/1.0 is added if omitted). It fetches the gateway's public config to derive the correct OAuth account URL, saves the choice to .env, and clears any existing token + workspace — so you must run login (then set-workspace) afterwards. Use it again at any time to switch environments."),
			mcp.WithString("preset", mcp.Description("Environment selector. One of: "+names+". Provide preset or url.")),
			mcp.WithString("url", mcp.Description("Custom/local server URL or host (e.g. http://localhost:9000 or my-onprem.example.com). Provide preset or url.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			preset, _ := args["preset"].(string)
			rawURL, _ := args["url"].(string)

			var base string
			switch {
			case preset != "":
				env, ok := config.PresetByName(presets, preset)
				if !ok {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] unknown preset %q — valid presets: %s (or pass url for a custom server)", preset, names)), nil
				}
				base = env.APIBaseURL
			case rawURL != "":
				base = config.NormalizeAPIBaseURL(rawURL)
			default:
				return mcp.NewToolResultError("[Error] provide preset (" + names + ") or url"), nil
			}

			// Fetch the public config to derive the account URL. This also validates
			// that the chosen environment is reachable and is a Simulator gateway.
			saURL, err := apiclient.FetchPublicConfig(ctx, base, insecure)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] could not read public config from %s: %v", base, err)), nil
			}

			// Persist the environment so it survives a restart (config.Resolve reads
			// SIMULATOR_API_BASE_URL and ACCOUNT_URL). Written in one pass so a failure
			// can't leave a new base URL with a stale account URL.
			if err := auth.SaveEnvironment(base, saURL); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to save environment: %v", err)), nil
			}

			// Switching environment always forces a fresh login: clear the token and
			// workspace (workspaces are per-environment), in both .env and the process.
			_ = auth.Delete()
			_ = auth.ClearWorkspaceID()
			engines.ResetAuth()

			// Apply live so the switch takes effect without a restart.
			c.SetBaseURL(base)
			c.SetWorkspaceID("")
			engines.SetBaseURL(base)

			msg := fmt.Sprintf(
				"Environment set:\n  API base: %s\n  Account (OAuth): %s\nPrevious token and workspace cleared. Next: call login, then getWorkspaces + set-workspace.",
				base, saURL)
			if apiclient.IsInsecureCredentialTransport(base) {
				warn := fmt.Sprintf("the API base %q uses plaintext HTTP to a non-local host — your auth token will be sent unencrypted. Use HTTPS.", base)
				log.Printf("WARNING: %s", warn)
				msg += "\n\n⚠️  WARNING: " + warn
			}
			return mcp.NewToolResultText(msg), nil
		},
	)
}

// presetNames returns the comma-separated list of preset selectors.
func presetNames(presets []config.CloudEnv) string {
	names := make([]string, 0, len(presets))
	for _, e := range presets {
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

// firstNonEmpty returns the first non-empty string argument, or "".
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
