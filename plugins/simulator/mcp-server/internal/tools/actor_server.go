package tools

import (
	"context"
	"maps"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// actorBinding describes how one curated operation is scoped to a single actor.
// Every operation that exists in actor mode has an entry in actorBindings() —
// an empty value means the tool is exposed unchanged in actor mode (no params
// bound), an absent entry means the tool is hidden in actor mode.
type actorBinding struct {
	// SetToActor: arg names whose value is set to the actor id.
	SetToActor []string
	// SetFixed: arg names whose value is set to the given string.
	SetFixed map[string]string
	// InjectItems: when true, sets actorId on every element of the body-root
	// `items` array (used by addAttachments / removeAttachments).
	InjectItems bool
}

// actorBindings returns the per-tool binding spec for actor-scoped mode, keyed
// by tool name. A tool absent from the map is NOT exposed in actor mode.
// The spec is shared by BuildActorScoped (compile-time binding for the
// dedicated per-actor server) and the unified server's ctx-driven actor mode
// (filter rewrites schemas + handlers inject from ctx).
func actorBindings() map[string]actorBinding {
	m := map[string]actorBinding{}

	// (1) Every curated op whose path carries an {actorId} placeholder: bind it.
	for _, op := range allOps() {
		for _, p := range op.Params {
			if p.In == InPath && p.Name == "actorId" {
				m[op.Name] = actorBinding{SetToActor: []string{"actorId"}}
				break
			}
		}
	}
	// (2) Generic access-rule tools pinned to this actor.
	for _, name := range []string{"getAccessRules", "saveAccessRules"} {
		m[name] = actorBinding{
			SetToActor: []string{"objId"},
			SetFixed:   map[string]string{"objType": "actor"},
		}
	}
	// (3) Link create/check: bind source to this actor.
	for _, name := range []string{"createLink", "existLink"} {
		m[name] = actorBinding{SetToActor: []string{"source"}}
	}
	// (4) Visible in actor mode, no binding (free args from the model).
	for _, name := range []string{"updateEdge", "deleteEdge", "uploadBase64"} {
		m[name] = actorBinding{}
	}
	// (5) Attachment link/unlink: inject actorId into every items[] element.
	for _, name := range []string{"addAttachments", "removeAttachments"} {
		m[name] = actorBinding{InjectItems: true}
	}
	return m
}

// actorScopedOps returns the curated ops that have an {actorId} path placeholder,
// in allOps() order. Kept exported for the existing TestActorScopedOps coverage.
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

// opsNamed returns the curated ops with the given names, in allOps order.
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
// argument.
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

// applyActorBinding mutates args with the binding's pre-bound values. Called
// from ctx-driven actor mode (per request) and indirectly via the static-bind
// path in BuildActorScoped.
func applyActorBinding(args map[string]any, actorID string, b actorBinding) {
	for _, p := range b.SetToActor {
		args[p] = actorID
	}
	for k, v := range b.SetFixed {
		args[k] = v
	}
	if b.InjectItems {
		injectActorIntoItems(args, actorID)
	}
}

// rewriteItemsDesc returns a copy of op with the body-root `items` description
// updated to instruct the model to omit actorId (which the server will inject).
// Used by both static-bind and ctx-bind actor-mode paths.
func rewriteItemsDesc(op Operation) Operation {
	local := op
	local.Params = append([]Param(nil), op.Params...)
	for i := range local.Params {
		if local.Params[i].In == InBodyRoot {
			local.Params[i].Desc = "Array (min 1) of {attachId:int (from uploadBase64 / getActorAttachments)}. " +
				"The file is linked to this actor automatically — do not include actorId."
		}
	}
	return local
}

// BuildActorScoped registers the per-actor tool set on s with actorID
// pre-bound at build time. Used by app/mcpserver.NewActorServer for the
// single-actor deployment where the URL itself selects the actor. Auth,
// workspace and API base URL still arrive per request via ctx; no .env /
// login helpers are registered.
//
// Inside the unified server (mcpserver.New), the same binding spec is applied
// dynamically from ctx — see registerCtxActor + ActorToolFilter.
func BuildActorScoped(s *server.MCPServer, c *apiclient.Client, actorID string) {
	bindings := actorBindings()
	for _, op := range allOps() {
		b, ok := bindings[op.Name]
		if !ok {
			continue
		}
		bound := map[string]any{}
		for _, p := range b.SetToActor {
			bound[p] = actorID
		}
		for k, v := range b.SetFixed {
			bound[k] = v
		}
		local := op
		var rewrite func(map[string]any)
		if b.InjectItems {
			local = rewriteItemsDesc(op)
			rewrite = func(args map[string]any) { injectActorIntoItems(args, actorID) }
		}
		s.AddTool(mcp.NewTool(local.Name, toolOptions(local, bound)...), makeHandlerRewrite(c, local, bound, rewrite))
	}
}

// registerCtxActor registers a curated op on the unified server with a handler
// that consults ctx for an actor id on every call and applies the binding when
// set. When ctx has no actor id, the handler behaves identically to a normal
// registration — the op is exposed with its full schema.
//
// The schema rewriting for actor mode (hiding bound params) is handled by the
// ActorToolFilter applied at tools/list time.
func registerCtxActor(s *server.MCPServer, c *apiclient.Client, op Operation, b actorBinding) {
	rewrite := func(ctx context.Context, args map[string]any) {
		actorID := apiclient.ActorIDFromContext(ctx)
		if actorID == "" {
			return
		}
		applyActorBinding(args, actorID, b)
	}
	s.AddTool(mcp.NewTool(op.Name, toolOptions(op, nil)...), makeHandlerCtxAware(c, op, rewrite))
}

// ActorToolFilter is the server.ToolFilterFunc that switches the tools/list
// view based on ctx: if ctx carries an actor id (apiclient.WithActorID), only
// the actor-scoped subset is returned and the bound params (actorId, source,
// objType/objId, …) are stripped from every schema. Otherwise the full list
// is passed through unchanged.
func ActorToolFilter(ctx context.Context, list []mcp.Tool) []mcp.Tool {
	if apiclient.ActorIDFromContext(ctx) == "" {
		return list
	}
	bindings := actorBindings()
	out := make([]mcp.Tool, 0, len(list))
	for _, t := range list {
		b, ok := bindings[t.Name]
		if !ok {
			continue
		}
		out = append(out, stripActorBoundFromSchema(t, b))
	}
	return out
}

// stripActorBoundFromSchema returns a copy of t with the actor-bound input
// params removed from the schema (so the model sees only the args it must
// supply). The items property's description is also rewritten for the
// InjectItems case so it stops asking for actorId.
func stripActorBoundFromSchema(t mcp.Tool, b actorBinding) mcp.Tool {
	hide := make(map[string]bool, len(b.SetToActor)+len(b.SetFixed))
	for _, p := range b.SetToActor {
		hide[p] = true
	}
	for k := range b.SetFixed {
		hide[k] = true
	}

	cp := t
	if len(t.InputSchema.Properties) > 0 {
		props := make(map[string]any, len(t.InputSchema.Properties))
		for k, v := range t.InputSchema.Properties {
			if hide[k] {
				continue
			}
			props[k] = v
		}
		if b.InjectItems {
			if raw, ok := props["items"].(map[string]any); ok {
				cloned := maps.Clone(raw)
				cloned["description"] = "Array (min 1) of {attachId:int (from uploadBase64 / getActorAttachments)}. " +
					"The file is linked to this actor automatically — do not include actorId."
				props["items"] = cloned
			}
		}
		cp.InputSchema.Properties = props
	}
	if len(t.InputSchema.Required) > 0 {
		req := make([]string, 0, len(t.InputSchema.Required))
		for _, r := range t.InputSchema.Required {
			if hide[r] {
				continue
			}
			req = append(req, r)
		}
		cp.InputSchema.Required = req
	}
	return cp
}
