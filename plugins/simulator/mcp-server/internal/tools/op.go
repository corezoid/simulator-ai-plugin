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
	opts := []mcp.ToolOption{mcp.WithDescription(op.Summary)}
	for _, p := range op.Params {
		opts = append(opts, paramOption(p))
	}
	s.AddTool(mcp.NewTool(op.Name, opts...), makeHandler(c, op))
}

// paramOption converts a Param into the matching mcp.With* tool option.
func paramOption(p Param) mcp.ToolOption {
	desc := p.Desc
	if len(p.Enum) > 0 {
		desc = strings.TrimSpace(desc + " (one of: " + strings.Join(p.Enum, ", ") + ")")
	}
	propOpts := []mcp.PropertyOption{mcp.Description(desc)}
	if p.Required {
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
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
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
			// accId defaults to the configured workspace when the caller omits it.
			if !present && p.Name == "accId" {
				if ws := c.WorkspaceID(); ws != "" {
					val, present = ws, true
				}
			}
			if !present {
				if p.Required {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required parameter %q", p.Name)), nil
				}
				continue
			}
			switch p.In {
			case InPath:
				path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(toString(val)))
			case InQuery:
				// Optional boolean query flags are presence-truthy on the backend;
				// omit them when false so "false" is not misread as "enabled".
				if p.Type == "boolean" && !p.Required {
					if b, ok := val.(bool); ok && !b {
						continue
					}
				}
				query.Set(p.Name, toString(val))
			case InBody:
				body[p.Name] = val
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
