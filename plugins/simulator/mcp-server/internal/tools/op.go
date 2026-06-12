// Package tools declares the curated set of MCP tools the server exposes and
// registers them against an apiclient. Each tool is a compile-time Operation
// descriptor (no runtime spec parsing): a method, a path template, and typed
// parameters. A single generic register() turns any Operation into a typed MCP
// tool whose handler maps arguments → path/query/body → one HTTP call.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ParamIn says where a parameter is placed in the HTTP request.
type ParamIn string

const (
	InPath     ParamIn = "path"      // substitutes {name} in the path template
	InQuery    ParamIn = "query"     // appended to the query string
	InBody     ParamIn = "body"      // a named field in the JSON request body
	InBodyRoot ParamIn = "body_root" // this single param IS the entire request body
	InLocal    ParamIn = "local"     // consumed by Resolve only; never sent to the API
)

// Param is one typed tool argument.
type Param struct {
	Name     string
	In       ParamIn
	Type     string // string | number | boolean | object | array
	Required bool
	Desc     string
	Enum     []string // allowed values, surfaced in the description

	// Wire overrides the name used on the wire (the path placeholder, query key,
	// or body field) while Name stays the MCP argument the model sees. Defaults to
	// Name. Needed when the backend reuses one name for two slots — e.g. reactions
	// take the root actor as the path `{actorId}` AND the reaction's own id as the
	// body `actorId`; the tool exposes them as distinct args (actorId, reactionId)
	// with the body param set to Wire:"actorId".
	Wire string

	// KeepFalse forces an optional boolean query flag to be sent even when the
	// caller sets it to false. Needed for flags whose backend default is TRUE
	// (e.g. recursive, notify): for those, omitting "false" would let the
	// backend re-apply its true default and silently ignore an explicit opt-out.
	// Leave false for the common case (default-false flags), where omitting a
	// false value is correct — see the InQuery handling in makeHandler.
	KeepFalse bool
}

// Operation is a curated API operation exposed as one MCP tool. Name is the
// operationId (and the MCP tool name); Path is relative to the client BaseURL
// with {placeholders} for InPath params.
type Operation struct {
	Name    string
	Method  string
	Path    string
	Summary string
	Params  []Param

	// Resolve, if set, runs before the request is assembled and may mutate args
	// (e.g. resolve a friendly name to an id). A returned error aborts the call.
	Resolve func(ctx context.Context, args map[string]any, c *apiclient.Client) error
}

// fieldFilterParam returns the standard `filter` query parameter for read
// operations whose backend supports server-side field selection (projection).
//
// The pong-server API applies `filter` via filterActorData / filterData: it is a
// comma-separated allow-list of top-level fields to keep in the response. Dotted
// paths like `data.status` select nested fields inside the free-form `data`
// object (its parent `data` is preserved, pruned to the listed sub-fields).
// Using it keeps responses — and token cost — small, so prefer it whenever only
// part of the entity is needed. `example` shows entity-appropriate field names.
func fieldFilterParam(example string) Param {
	return Param{
		Name: "filter", In: InQuery, Type: "string",
		Desc: "Comma-separated allow-list of fields to return (server-side projection). " +
			"Only the listed fields are kept; use dotted paths like `data.status` for nested data fields. " +
			"Prefer setting this to just the fields you need — it sharply reduces response size and token cost. " +
			"Omit to return the full object. Example: \"" + example + "\".",
	}
}

// register builds the MCP tool for op and wires its handler to the client.
func register(s *server.MCPServer, c *apiclient.Client, op Operation) {
	registerBound(s, c, op, nil)
}

// registerBound is register with pre-bound argument values. Params named in bound
// are NOT exposed in the tool's input schema (the model cannot set them) and are
// injected authoritatively at call time. Used to bind actor-scoped params — e.g.
// `actorId` taken from the per-actor URL — so a per-actor tool takes only the
// meaningful arguments.
func registerBound(s *server.MCPServer, c *apiclient.Client, op Operation, bound map[string]any) {
	s.AddTool(mcp.NewTool(op.Name, toolOptions(op, bound)...), makeHandlerBound(c, op, bound))
}

// toolOptions builds the MCP tool options for op, skipping any pre-bound param
// (hidden from the model).
func toolOptions(op Operation, bound map[string]any) []mcp.ToolOption {
	opts := []mcp.ToolOption{mcp.WithDescription(op.Summary)}
	for _, p := range op.Params {
		if _, ok := bound[p.Name]; ok {
			continue
		}
		opts = append(opts, paramOption(p))
	}
	return opts
}

// paramOption converts a Param into the matching mcp.With* tool option.
func paramOption(p Param) mcp.ToolOption {
	desc := p.Desc
	if len(p.Enum) > 0 {
		desc = strings.TrimSpace(desc + " (one of: " + strings.Join(p.Enum, ", ") + ")")
	}
	propOpts := []mcp.PropertyOption{mcp.Description(desc)}
	// accId is special: the handler defaults it to the configured workspace
	// (Client.WorkspaceIDForContext) when the caller omits it. Don't mark it
	// required in the schema — otherwise clients like MCP Inspector force a
	// value into the field, and the empty string overrides the ctx-scoped
	// workspace before the fallback can fire.
	if p.Required && p.Name != "accId" {
		propOpts = append(propOpts, mcp.Required())
	}
	switch p.Type {
	case "number":
		return mcp.WithNumber(p.Name, propOpts...)
	case "boolean":
		return mcp.WithBoolean(p.Name, propOpts...)
	case "object":
		return mcp.WithObject(p.Name, propOpts...)
	case "array":
		return mcp.WithArray(p.Name, propOpts...)
	default:
		return mcp.WithString(p.Name, propOpts...)
	}
}

// makeHandler returns the tool handler that assembles and issues the request.
func makeHandler(c *apiclient.Client, op Operation) server.ToolHandlerFunc {
	return makeHandlerBound(c, op, nil)
}

// makeHandlerBound is makeHandler with pre-bound argument values injected
// (authoritative) before resolution and request assembly.
func makeHandlerBound(c *apiclient.Client, op Operation, bound map[string]any) server.ToolHandlerFunc {
	return makeHandlerRewrite(c, op, bound, nil)
}

// makeHandlerRewrite is makeHandlerBound with an extra hook that may mutate the
// assembled args after pre-bound injection and before Resolve. It exists for the
// per-actor server, where a bound identity must be injected into each element of
// a root-array body (e.g. setting actorId on every attachment link item) — which
// a flat key bind cannot express. rewrite may be nil.
func makeHandlerRewrite(c *apiclient.Client, op Operation, bound map[string]any, rewrite func(map[string]any)) server.ToolHandlerFunc {
	if len(bound) == 0 && rewrite == nil {
		return makeHandlerCtxAware(c, op, nil)
	}
	adjust := func(_ context.Context, args map[string]any) {
		if len(bound) > 0 {
			maps.Copy(args, bound)
		}
		if rewrite != nil {
			rewrite(args)
		}
	}
	return makeHandlerCtxAware(c, op, adjust)
}

// makeHandlerCtxAware is the canonical handler factory: it lets the caller
// mutate args on every call with full ctx visibility. The unified server's
// per-request actor mode uses it to inject the ctx-bound actor id into the
// args before the request is assembled. adjust may be nil.
func makeHandlerCtxAware(c *apiclient.Client, op Operation, adjust func(context.Context, map[string]any)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if adjust != nil {
			if args == nil {
				args = map[string]any{}
			}
			adjust(ctx, args)
		}
		if op.Resolve != nil {
			if err := op.Resolve(ctx, args, c); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] %s: %v", op.Name, err)), nil
			}
		}
		path := op.Path
		query := url.Values{}
		body := map[string]any{}
		var bodyRoot any

		for _, p := range op.Params {
			val, present := args[p.Name]
			// accId defaults to the configured workspace when the caller omits it,
			// or passes an empty string — MCP Inspector etc. force a value into
			// fields named in the schema, so "" is the common "I have nothing"
			// signal and must not override the ctx-scoped workspace.
			if p.Name == "accId" {
				if s, ok := val.(string); !present || (ok && s == "") {
					if ws := c.WorkspaceIDForContext(ctx); ws != "" {
						val, present = ws, true
					}
				}
			}
			if !present {
				if p.Required {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required parameter %q", p.Name)), nil
				}
				continue
			}
			// wire is the name used on the request (path placeholder / query key /
			// body field); defaults to the MCP arg name unless Wire overrides it.
			wire := p.Name
			if p.Wire != "" {
				wire = p.Wire
			}
			switch p.In {
			case InPath:
				path = strings.ReplaceAll(path, "{"+wire+"}", url.PathEscape(toString(val)))
			case InQuery:
				// Optional boolean query flags are presence-truthy on the backend;
				// omit them when false so "false" is not misread as "enabled".
				// KeepFalse opts out (for flags whose backend default is true, an
				// explicit false must be sent or the default silently wins).
				if p.Type == "boolean" && !p.Required && !p.KeepFalse {
					if b, ok := val.(bool); ok && !b {
						continue
					}
				}
				query.Set(wire, toString(val))
			case InBody:
				body[wire] = val
			case InBodyRoot:
				bodyRoot = val
			case InLocal:
				// consumed by op.Resolve; not placed in the request
			}
		}

		if strings.Contains(path, "{") {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] unresolved path parameter in %s", path)), nil
		}

		var reqBody any
		switch {
		case bodyRoot != nil:
			reqBody = bodyRoot
		case len(body) > 0:
			reqBody = body
		case op.hasObjectBody() && methodTakesBody(op.Method):
			// All body fields optional and omitted — still send "{}" so a
			// Fastify object-body schema does not reject a missing body.
			reqBody = map[string]any{}
		}

		resp, err := c.Do(ctx, op.Method, path, query, reqBody)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] %s: %v", op.Name, err)), nil
		}
		return mcp.NewToolResultText(string(resp)), nil
	}
}

// hasObjectBody reports whether the operation has named (object) body fields.
func (op Operation) hasObjectBody() bool {
	for _, p := range op.Params {
		if p.In == InBody {
			return true
		}
	}
	return false
}

// methodTakesBody reports whether the HTTP method conventionally carries a body.
func methodTakesBody(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

// toString renders a JSON argument value for use in a path/query segment.
func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers decode to float64; render integers without a trailing ".0".
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
