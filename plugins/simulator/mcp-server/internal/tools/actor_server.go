package tools

import (
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// actorScopedOps is the subset of the curated set that operates on a single
// actor — every operation whose path carries an {actorId} placeholder
// (getActor, getAccounts, getReactions, createReaction, getRelatedActors, …).
// These are the tools that make sense on a per-actor MCP endpoint; workspace-wide
// tools (create form, list all actors) are intentionally excluded.
func actorScopedOps() []Operation {
	var ops []Operation
	for _, op := range allOps() {
		for _, p := range op.Params {
			if p.In == InPath && p.Name == "actorId" {
				ops = append(ops, op)
				break
			}
		}
	}
	return ops
}

// opsNamed returns the curated ops with the given names, in allOps order. Used to
// pull specific tools into the per-actor server with a tailored binding (their
// path is not keyed by {actorId}, so actorScopedOps does not select them).
func opsNamed(names ...string) []Operation {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var ops []Operation
	for _, op := range allOps() {
		if want[op.Name] {
			ops = append(ops, op)
		}
	}
	return ops
}

// injectActorIntoItems sets actorId on every object in the root-array `items`
// argument. The per-actor link/unlink tools (addAttachments / removeAttachments)
// use it so a request targets only this actor — the model supplies just attachId.
func injectActorIntoItems(args map[string]any, actorID string) {
	items, ok := args["items"].([]any)
	if !ok {
		return
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			m["actorId"] = actorID
		}
	}
}

// registerActorItems registers a body-root-array op (addAttachments /
// removeAttachments) for the per-actor server: actorID is injected into every
// items[] element at call time, and the items description is rewritten so the
// model supplies only the file id — never the actor. The shared op is copied, not
// mutated.
func registerActorItems(s *server.MCPServer, c *apiclient.Client, op Operation, actorID string) {
	local := op
	local.Params = append([]Param(nil), op.Params...)
	for i := range local.Params {
		if local.Params[i].In == InBodyRoot {
			local.Params[i].Desc = "Array (min 1) of {attachId:int (from uploadBase64 / getActorAttachments)}. " +
				"The file is linked to this actor automatically — do not include actorId."
		}
	}
	rewrite := func(args map[string]any) { injectActorIntoItems(args, actorID) }
	s.AddTool(mcp.NewTool(local.Name, toolOptions(local, nil)...), makeHandlerRewrite(c, local, nil, rewrite))
}

// BuildActorScoped registers the per-actor tool set on s: the curated tools scoped
// to a single actor, all with the actor's identity pre-bound and hidden from the
// tool schemas, so the model cannot target a different actor. Auth, workspace and
// API base URL arrive per request via ctx (apiclient.WithAuthorization /
// WithWorkspaceID / WithBaseURL); no .env / login helpers are registered. The
// hosting gateway routes /mcp/actors/{actorId} to a server built this way — see
// app/mcpserver.NewActorServer.
func BuildActorScoped(s *server.MCPServer, c *apiclient.Client, actorID string) {
	// (1) Tools whose path is keyed by {actorId}: bind it to this actor.
	idBound := map[string]any{"actorId": actorID}
	for _, op := range actorScopedOps() {
		registerBound(s, c, op, idBound)
	}

	// (2) Generic access-rule tools, pinned to this actor (objType="actor",
	// objId=<actor>, both hidden) — manage this actor's access rules.
	accessBound := map[string]any{"objType": "actor", "objId": actorID}
	for _, op := range opsNamed("getAccessRules", "saveAccessRules") {
		registerBound(s, c, op, accessBound)
	}

	// (3) Link CRUD: create/check outgoing links from this actor (source bound +
	// hidden); edit/delete the actor's existing edges by id (ids come from
	// getActorLinks — backend access rules still apply).
	sourceBound := map[string]any{"source": actorID}
	for _, op := range opsNamed("createLink", "existLink") {
		registerBound(s, c, op, sourceBound)
	}
	for _, op := range opsNamed("updateEdge", "deleteEdge") {
		register(s, c, op)
	}

	// (4) Attachment management for this actor: upload a file to the workspace
	// (accId from the request context), then link/unlink it to this actor — the
	// actorId is injected into every items[] entry, so the model passes only attachId.
	for _, op := range opsNamed("uploadBase64") {
		register(s, c, op)
	}
	for _, op := range opsNamed("addAttachments", "removeAttachments") {
		registerActorItems(s, c, op, actorID)
	}
}
