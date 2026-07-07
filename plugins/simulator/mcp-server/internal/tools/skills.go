package tools

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Skill registry tools — the data-driven analog of Claude Code skills.
//
// A "skill" is an actor of the `Skills` SYSTEM form: its title + ref(slug) are
// the cheap discovery metadata and its `description` (unlimited TEXT) holds the
// full procedure body (which MCP tools to call, with concrete entity ids). The
// registry lets users teach the assistant workspace-specific playbooks without a
// plugin release; the reaction-agent and the interactive client both look one up
// by intent (findSkill) or by slug (getSkill), then follow its steps.
//
// findSkill / getSkill are LOCAL composite tools (they resolve the form and
// compose existing PAPI reads), so — like buildLink / readAttachment — they are
// registered outside allOps() and are NOT subject to the spec drift gate.
// Authoring (create/update/publish) reuses the curated createActor /
// updateActor / setActorStatus tools; see the simulator-skills skill.

// skillsFormTitle is the exact title of the Skills system form (resolved by
// (type=system, title) — the system_forms template carries no ref, so a
// title match is the lookup, mirroring how reactions resolve their form).
const skillsFormTitle = "Skills"

// skillsFormCache memoizes the resolved Skills system-form id per workspace
// (accId). The id is stable for a workspace's lifetime, so caching avoids a
// forms-list round-trip on every findSkill / getSkill call. In stateless mode
// one server serves many workspaces, hence the per-accId key.
var skillsFormCache sync.Map // accId(string) -> formID(int)

// skillActorFilter is the field projection for a single skill actor: enough to
// read and follow the playbook, without the form schema / access list bloat.
const skillActorFilter = "id,title,ref,status,description,formId,updatedAt"

// skillListFilter is the cheap projection for discovery lists — never the
// description body (that is loaded on demand by getSkill).
const skillListFilter = "id,title,ref,status"

// resolveSkillsFormID returns the Skills system-form id for the request's
// workspace, caching the result. The error is friendly: a missing form means
// the backend migration / system-forms sync has not run for this workspace.
func resolveSkillsFormID(ctx context.Context, c *apiclient.Client) (int, error) {
	return resolveSystemFormByTitle(ctx, c, skillsFormTitle, &skillsFormCache,
		"needs the backend Skills migration + system-forms sync")
}

// registerSkillTools adds the skill-registry discovery tools (findSkill,
// getSkill) to s. Called from BuildUnified alongside registerBuildLink — these
// are local composite tools, not curated Operations, so they live outside the
// op loop / drift gate.
func registerSkillTools(s *server.MCPServer, c *apiclient.Client) {
	s.AddTool(
		mcp.NewTool("findSkill",
			mcp.WithDescription(
				"Discover saved skill playbooks (actors of the `Skills` system form) by intent. "+
					"Returns a cheap list of {id,title,ref,status} — NOT the procedure body; call getSkill(ref) to load a chosen one. "+
					"Pass `query` to full-text match candidate titles; leave it EMPTY to list every active (verified) skill — use that to answer \"what skills do I have?\" or to discover slugs. "+
					"Only verified skills are returned (drafts/disabled are hidden). Call this before planning a multi-step Simulator task to reuse an existing playbook."),
			mcp.WithString("query", mcp.Description("Intent / title fragment to match. Empty lists all verified skills (enumeration).")),
			mcp.WithNumber("limit", mcp.Description("Max results (1-200, default 20).")),
		),
		findSkillHandler(c),
	)
	s.AddTool(
		mcp.NewTool("getSkill",
			mcp.WithDescription(
				"Load one skill playbook in full, including its `description` procedure body (which tools to call, with concrete entity ids). "+
					"This is the explicit-invocation path: when the user names a skill by slug (e.g. \"/skill create-smart-contract\" or \"use skill …\"), pass that slug as `ref` for an instant lookup — no discovery needed. "+
					"Provide `ref` (the skill's slug) OR `id` (actor UUID). Verify status=verified and that the entity ids it references still exist before following the steps; confirm any destructive/outward action with the user."),
			mcp.WithString("ref", mcp.Description("Skill slug (the actor's ref). Provide ref or id.")),
			mcp.WithString("id", mcp.Description("Skill actor UUID. Provide ref or id.")),
		),
		getSkillHandler(c),
	)
}

// findSkillHandler lists verified skills, optionally narrowed by a title search,
// via filterActors on the Skills form. The body is intentionally omitted.
func findSkillHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		query, _ := args["query"].(string)
		limit := 20
		if l, ok := args["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		formID, err := resolveSkillsFormID(ctx, c)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] findSkill: %v", err)), nil
		}
		q := url.Values{}
		q.Set("status", "verified")
		q.Set("filter", skillListFilter)
		q.Set("limit", strconv.Itoa(limit))
		if s := strings.TrimSpace(query); s != "" {
			q.Set("search", s)
		}
		resp, err := c.Do(ctx, "GET", "/actors_filters/"+strconv.Itoa(formID), q, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] findSkill: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resp)), nil
	}
}

// getSkillHandler loads a single skill by ref (getActorByRef on the Skills form)
// or by actor UUID (getActor), including the description body.
func getSkillHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		ref, _ := args["ref"].(string)
		id, _ := args["id"].(string)
		ref = strings.TrimSpace(ref)
		id = strings.TrimSpace(id)
		if ref == "" && id == "" {
			return mcp.NewToolResultError("[Error] getSkill: provide ref (slug) or id"), nil
		}

		q := url.Values{}
		q.Set("filter", skillActorFilter)

		var path string
		if id != "" {
			path = "/actors/" + url.PathEscape(id)
		} else {
			formID, err := resolveSkillsFormID(ctx, c)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] getSkill: %v", err)), nil
			}
			path = "/actors/ref/" + strconv.Itoa(formID) + "/" + url.PathEscape(ref)
		}

		resp, err := c.Do(ctx, "GET", path, q, nil)
		if err != nil {
			if ref != "" {
				return mcp.NewToolResultError(fmt.Sprintf(
					"[Error] getSkill: no skill with slug %q (it may not exist or not be published) — try findSkill to list available skills: %v", ref, err)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getSkill: %v", err)), nil
		}
		return mcp.NewToolResultText(string(resp)), nil
	}
}
