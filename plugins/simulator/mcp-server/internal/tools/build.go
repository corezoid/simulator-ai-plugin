package tools

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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
	ops = append(ops, formAccountOps...)
	ops = append(ops, formGraphOps...)
	ops = append(ops, actorOps...)
	ops = append(ops, accountOps...)
	ops = append(ops, accountActorOps...)
	ops = append(ops, counterOps...)
	ops = append(ops, accessRuleOps...)
	ops = append(ops, publicLinkOps...)
	ops = append(ops, meetingOps...)
	ops = append(ops, transactionOps...)
	ops = append(ops, graphOps...)
	ops = append(ops, reactionOps...)
	ops = append(ops, attachmentOps...)
	ops = append(ops, uploadOps...)
	ops = append(ops, searchOps...)
	ops = append(ops, workspaceOps...)
	ops = append(ops, userOps...)
	ops = append(ops, smartFormOps...)
	return ops
}

// BuildAll registers every curated API tool plus the auth helpers (set-environment,
// login, logout, status, set-workspace) on the MCP server. insecure is threaded
// through so the public-config probes (set-environment, login self-heal, status)
// honour the same TLS mode as the client.
func BuildAll(s *server.MCPServer, c *apiclient.Client, prof config.Profile, insecure bool) {
	BuildUnified(s, c, true)
	registerAuth(s, c, prof, insecure)
}

// BuildAllStateless registers only the curated API tools (no login / set-workspace
// / set-environment helpers). Use for embedded SSE deployments where credentials
// arrive per request via ctx and the server must not read or write .env.
func BuildAllStateless(s *server.MCPServer, c *apiclient.Client) {
	BuildUnified(s, c, true)
}

// BuildUnified registers every curated API tool with ctx-aware actor-mode
// support. A request that arrives with apiclient.WithActorID(ctx, id) set
// switches the session into per-actor mode for both routing
// (ActorToolFilter hides workspace-wide tools and strips bound params from
// the actor-scoped subset's schema) and execution (handlers inject the
// actor id where the binding spec — actorBindings() — requires it).
// Auth helpers (set-environment, login, set-workspace) and engine tools are
// registered separately by the caller; both are hidden in actor mode.
//
// includeActorMode=false skips the ctx-aware wrapping (handlers behave
// identically, since the wrapper is a no-op when ctx has no actor id), which
// is useful for callers that explicitly never want actor mode to engage.
func BuildUnified(s *server.MCPServer, c *apiclient.Client, includeActorMode bool) {
	bindings := actorBindings()
	for _, op := range allOps() {
		if includeActorMode {
			if b, ok := bindings[op.Name]; ok {
				registerCtxActor(s, c, op, b)
				continue
			}
		}
		register(s, c, op)
	}
	// buildLink is a local (non-HTTP) helper, not a curated Operation, so it is
	// registered outside the op loop. It is exposed in workspace mode only —
	// ActorToolFilter hides it in actor sessions (it is absent from actorBindings).
	registerBuildLink(s, c)
	// getBbcodeTags is a local reference tool (fetches the env's bbcode-tags.json);
	// like buildLink it's not a curated Operation and is workspace-mode only.
	registerBbcodeTags(s, c)
	// readAttachment downloads a file's content; it's a local tool (not a curated
	// Operation) because it must emit image / embedded-resource content. Unlike
	// buildLink it IS exposed in actor mode (see actorBindings) so the
	// reaction-triggered agent can read files attached to its reaction.
	registerReadAttachment(s, c)
	// findSkill / getSkill are local composite tools (resolve the Skills system
	// form + compose existing PAPI reads), so — like buildLink — they live
	// outside the op loop and the drift gate. They power the skill registry (the
	// data-driven analogue of Claude Code skills).
	registerSkillTools(s, c)
	// findAgent / getAgent are the actor-analog of the skill registry: they
	// resolve a registry form / compose existing PAPI reads over the actors whose
	// `description` holds an "# Agent" competency profile — user twins by default,
	// but any actor can be an agent. Like findSkill/getSkill they are local
	// composite tools, outside the drift gate.
	registerAgentTools(s, c)
}

// Count reports how many curated API tools are registered (auth helpers excluded).
func Count() int { return len(allOps()) }

func registerAuth(s *server.MCPServer, c *apiclient.Client, prof config.Profile, insecure bool) {
	// The auth endpoints (discovery, token exchange) must follow the same TLS
	// mode as the API client, or a self-signed on-prem account service passes
	// the config probe and then dies on the token POST.
	auth.InsecureTLS = insecure
	registerSetEnvironment(s, c, prof, insecure)

	s.AddTool(
		mcp.NewTool("login",
			mcp.WithDescription("Authenticate to Simulator via OAuth2 PKCE (opens a browser). Before opening the browser it verifies ACCOUNT_URL against the chosen gateway's public config and self-heals it if a sibling tool overwrote it (works for cloud and on-prem gateways alike). Saves the token to .env. Run set-environment first if you haven't chosen an environment. After login, call set-workspace to choose a workspace."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Prefer the account URL derived by set-environment (saved as ACCOUNT_URL)
			// over the startup profile default, so login follows the chosen environment.
			apiBase := firstNonEmpty(c.BaseURL(), os.Getenv("SIMULATOR_API_BASE_URL"), prof.APIBaseURL)
			envAccount := firstNonEmpty(os.Getenv("ACCOUNT_URL"), prof.AccountURL)
			accountURL, healNote := resolveAccountURL(ctx, apiBase, envAccount, insecure)
			// OAuth 2.1 silent renewal: a stored refresh token skips the
			// browser round-trip. Any failure falls back to the normal flow.
			if rt := os.Getenv("REFRESH_TOKEN"); rt != "" {
				if creds, rerr := auth.Refresh(ctx, accountURL, prof.OAuthClientID, rt); rerr != nil {
					log.Printf("login: silent token refresh failed: %v — falling back to browser OAuth", rerr)
				} else if verr := validateSimToken(ctx, apiBase, creds, insecure); verr != nil {
					// Verified live: the account service's refresh grant currently
					// mints an account-SESSION token the papi rejects — a refreshed
					// token is only trusted after a real authenticated call succeeds.
					log.Printf("login: refresh grant returned a token the API does not accept (%v) — falling back to browser OAuth", verr)
					if creds.RefreshToken != "" {
						// The AS rotated the refresh token on use — keep the
						// newest one even though this access token was rejected,
						// or a rotating AS would burn the stored token.
						_ = auth.SaveRefreshToken(creds.RefreshToken)
					} else {
						// No rotation and the minted token is unusable — drop the
						// stored one so future logins skip these round-trips (a
						// browser login stores a fresh refresh token anyway).
						_ = auth.DeleteRefreshToken()
					}
				} else {
					if err := auth.Save(creds); err != nil {
						return mcp.NewToolResultError(strings.TrimSpace(healNote + "\n" + fmt.Sprintf("[Error] authenticated, but failed to save the token to %s: %v — fix the file permissions and re-run login.", auth.EnvPath(), err))), nil
					}
					msg := "Authenticated: access token renewed silently via the stored refresh token — no browser login was needed. Next: call getWorkspaces to list your workspaces, show them to the user to pick one, then call set-workspace (by accId or name)."
					if healNote != "" {
						msg = healNote + "\n\n" + msg
					}
					return mcp.NewToolResultText(msg), nil
				}
			}
			creds, authURL, err := auth.PKCEFlow(ctx, accountURL, prof.OAuthClientID, nil)
			if err != nil {
				return mcp.NewToolResultError(loginFailureMsg(err, authURL, accountURL, apiBase, healNote)), nil
			}
			if err := auth.Save(creds); err != nil {
				return mcp.NewToolResultError(strings.TrimSpace(healNote + "\n" + fmt.Sprintf("[Error] authenticated, but failed to save the token to %s: %v — fix the file permissions and re-run login.", auth.EnvPath(), err))), nil
			}
			msg := "Authenticated. Token saved to .env. Next: call getWorkspaces to list your workspaces, show them to the user to pick one, then call set-workspace (by accId or name)."
			if healNote != "" {
				msg = healNote + "\n\n" + msg
			}
			return mcp.NewToolResultText(msg), nil
		},
	)

	s.AddTool(
		mcp.NewTool("logout",
			mcp.WithDescription("Remove the saved token (ACCESS_TOKEN) from .env and from this process. The removed lines are first backed up to .env.bak. ⚠️ The .env file can be shared with sibling plugins (e.g. corezoid) — if they use the same token, they are logged out too."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			creds, _ := auth.Load()
			if creds == nil && os.Getenv("REFRESH_TOKEN") == "" {
				return mcp.NewToolResultText(fmt.Sprintf("No token found — nothing to log out (%s has no ACCESS_TOKEN or REFRESH_TOKEN).", auth.EnvPath())), nil
			}
			// Back up BEFORE removing: the .env is shared with sibling plugins, so an
			// accidental logout must stay recoverable by hand. Backup failure aborts.
			bak, bakErr := auth.BackupToken()
			if bakErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] could not back up the token before removal (%v) — nothing was changed. Fix the problem (is %s.bak writable?) or remove the ACCESS_TOKEN line from .env manually.", bakErr, auth.EnvPath())), nil
			}
			if err := auth.Delete(); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to remove the token from %s: %v — the token may still be present there; remove the ACCESS_TOKEN line manually.", auth.EnvPath(), err)), nil
			}
			engines.ResetAuth()
			msg := "Logged out: ACCESS_TOKEN removed from .env and from this process."
			if bak != "" {
				msg += "\nBackup of the removed lines: " + bak
			}
			msg += "\n⚠️ If the corezoid plugin was sharing this token, it is logged out too. Re-run login to authenticate again."
			return mcp.NewToolResultText(msg), nil
		},
	)

	s.AddTool(
		mcp.NewTool("status",
			mcp.WithDescription("Self-diagnosis: show the current environment (API gateway, OAuth account URL, workspace) and token state, with no side effects. Pass probe=true to also verify ACCOUNT_URL against the gateway's public config (one unauthenticated request; works for cloud and on-prem). Run this first when login or any tool misbehaves."),
			mcp.WithBoolean("probe", mcp.Description("Also fetch the gateway's public config and check ACCOUNT_URL against its declared account service (saUrl).")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			apiBase := firstNonEmpty(c.BaseURL(), os.Getenv("SIMULATOR_API_BASE_URL"), prof.APIBaseURL)
			envAccount := firstNonEmpty(os.Getenv("ACCOUNT_URL"), prof.AccountURL)
			ws := firstNonEmpty(c.WorkspaceID(), os.Getenv("WORKSPACE_ID"))

			tokenLine := "absent — run login"
			if creds, _ := auth.Load(); creds != nil {
				switch {
				case auth.IsExpired(creds):
					tokenLine = fmt.Sprintf("EXPIRED %s — re-run login", creds.ExpiresAt.UTC().Format("2006-01-02 15:04"))
				case creds.ExpiresAt.IsZero():
					tokenLine = "present (no expiry recorded)"
				default:
					tokenLine = "present, valid until " + creds.ExpiresAt.UTC().Format("2006-01-02 15:04")
				}
			}
			accLine := envAccount
			if envAccount == "" {
				accLine = "(not set — login will derive it from the gateway config)"
			}
			wsLine := ws
			if ws == "" {
				wsLine = "(not set — run set-workspace after login)"
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "simulator-mcp status (profile: %s)\n", prof.Name)
			fmt.Fprintf(&sb, "  API gateway: %s\n", apiBase)
			fmt.Fprintf(&sb, "  ACCOUNT_URL: %s\n", accLine)
			fmt.Fprintf(&sb, "  workspace:   %s\n", wsLine)
			fmt.Fprintf(&sb, "  token:       %s\n", tokenLine)
			fmt.Fprintf(&sb, "  .env:        %s\n", auth.EnvPath())

			if probeRequested(req.GetArguments()) {
				saURL, err := apiclient.FetchPublicConfig(ctx, apiBase, insecure)
				switch {
				case err != nil:
					fmt.Fprintf(&sb, "probe: FAILED — the gateway config at %s is unreachable: %v. Check the network / the gateway URL; run set-environment to pick a reachable one.\n", apiBase, err)
				case envAccount == "":
					fmt.Fprintf(&sb, "probe: NOT SET — the gateway declares its account service at %s; login will adopt and save it automatically.\n", saURL)
				case !sameBaseURL(saURL, envAccount):
					fmt.Fprintf(&sb, "probe: MISMATCH — the gateway declares its account service at %s but ACCOUNT_URL is %s (a sibling tool sharing .env may have overwritten it). login will self-heal this automatically; set-environment also fixes it.\n", saURL, envAccount)
				default:
					fmt.Fprintf(&sb, "probe: OK — ACCOUNT_URL matches the account service declared by the gateway.\n")
				}
			}
			sb.WriteString("If OTHER sessions report \"No such tool available\" for simulator tools, this server was restarted after they connected — those sessions must be RESTARTED (a plain reconnect may not be enough).")
			return mcp.NewToolResultText(sb.String()), nil
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

// resolveAccountURL self-diagnoses the OAuth account URL before login opens a
// browser. The ACCOUNT_URL key in .env is shared with sibling plugins (corezoid)
// and can be overwritten with a host that serves no OAuth endpoints (e.g. the
// admin UI) — the browser then opens a dashboard and login dies silently. The
// source of truth for ANY environment, cloud or on-prem, is what the chosen
// gateway itself declares in its public config (saUrl), so we ask the gateway
// and self-heal .env when they disagree. The second return value is a note for
// the user describing what was found and what was done ("" when all is well).
func resolveAccountURL(ctx context.Context, apiBase, envAccount string, insecure bool) (string, string) {
	saURL, err := apiclient.FetchPublicConfig(ctx, apiBase, insecure)
	if err != nil || saURL == "" {
		if envAccount == "" {
			return "", fmt.Sprintf("⚠️ ACCOUNT_URL is not set and the gateway config at %s is unreachable (%v) — falling back to the default account service. If login fails, run set-environment to pick a reachable gateway.", apiBase, err)
		}
		return envAccount, fmt.Sprintf("⚠️ Could not verify ACCOUNT_URL against the gateway config at %s (%v) — proceeding with %s as-is.", apiBase, err, envAccount)
	}
	if sameBaseURL(saURL, envAccount) {
		return envAccount, ""
	}
	note := fmt.Sprintf("🔧 Self-healed ACCOUNT_URL: .env had %q, but the gateway %s declares its account service at %s (a sibling tool sharing .env may have overwritten it). Using %s.", envAccount, apiBase, saURL, saURL)
	if envAccount == "" {
		note = fmt.Sprintf("ACCOUNT_URL was not set — derived %s from the gateway config at %s.", saURL, apiBase)
	}
	if err := auth.SaveAccountURL(saURL); err != nil {
		note += fmt.Sprintf(" ⚠️ Could not persist it to .env (%v) — it will be re-derived on the next login.", err)
	} else {
		note += " Saved to .env."
	}
	return saURL, note
}

// validateSimToken makes one authenticated papi call (GET /workspaces) with
// the candidate token, so a refresh-minted token is proven to work before it
// replaces the stored one.
func validateSimToken(ctx context.Context, apiBase string, creds *auth.Credentials, insecure bool) error {
	u := strings.TrimRight(apiBase, "/") + "/workspaces"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", creds.AuthorizationHeader())
	resp, err := auth.NewHTTPClient(insecure, 15*time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("papi answered %d to an authenticated call", resp.StatusCode)
	}
	return nil
}

// sameBaseURL compares two service URLs ignoring a trailing slash and case.
func sameBaseURL(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

// loginFailureMsg turns an OAuth failure into a self-diagnosis: what failed,
// the authorization URL for a manual retry, and the concrete next steps.
func loginFailureMsg(err error, authURL, accountURL, apiBase, healNote string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Error] OAuth2 login failed: %v\n", err)
	if healNote != "" {
		sb.WriteString(healNote + "\n")
	}
	sb.WriteString("\nWhat to check / how to fix:\n")
	step := 1
	if authURL != "" {
		fmt.Fprintf(&sb, "%d. This consent URL was used (its callback listener is closed now — verify the host looks right, then RE-RUN login rather than opening it):\n   %s\n", step, authURL)
		step++
	}
	fmt.Fprintf(&sb, "%d. Verify the environment: API gateway %s, OAuth account service %s. If either looks wrong, run set-environment (preset or url — on-prem servers included): it re-derives the account URL from the gateway itself.\n", step, apiBase, accountURL)
	step++
	fmt.Fprintf(&sb, "%d. If the page showed a dashboard instead of a consent screen, ACCOUNT_URL pointed at the wrong host — set-environment fixes that.\n", step)
	step++
	fmt.Fprintf(&sb, "%d. If it timed out, re-run login and complete the browser consent within 5 minutes.", step)
	return sb.String()
}

// probeRequested reads the status tool's probe argument, accepting a real
// boolean or the strings "true"/"1" (CLI-style leniency).
func probeRequested(args map[string]any) bool {
	if b, ok := args["probe"].(bool); ok {
		return b
	}
	if ps, ok := args["probe"].(string); ok {
		return strings.EqualFold(ps, "true") || ps == "1"
	}
	return false
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
