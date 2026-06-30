package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// buildLink is a local (non-HTTP) tool that turns an entity reference into an
// absolute Simulator web-app deep-link the user can click. The web routes are
// the ones published at <web-base>/routes.json; this tool mirrors the useful
// subset so the model never has to guess a URL.
//
// It resolves two things the model cannot reliably infer on its own:
//   - the web base URL, derived from the configured API base by dropping the
//     "/papi/1.0" suffix (e.g. https://sim.simulator.company/papi/1.0 →
//     https://sim.simulator.company);
//   - the workspace `acc` segment, which defaults to the active workspace and
//     is shortened to the 8-char form the URLs use when a full UUID is given.
//
// It is registered only in workspace mode (BuildUnified): ActorToolFilter hides
// it in actor-scoped sessions because it is not listed in actorBindings().

// linkEntity is one buildable deep-link kind, mapping an `entity` argument to a
// path template and the rules for filling it in.
type linkEntity struct {
	name string // the `entity` enum value the model passes
	help string // what ids this entity needs, surfaced in the tool description

	// build assembles the path (relative to the web base, leading slash) from the
	// already-shortened acc and the supplied ids, or returns an error naming the
	// missing required id.
	build func(acc string, p linkParams) (string, error)
}

// linkParams carries the id arguments after defaulting/normalisation.
type linkParams struct {
	id          string // primary resource id; meaning depends on the entity
	streamId    string // event/chat stream
	secondaryId string // event sub-stream, or chat conversation id
	mode        string // graph display mode: layers | actors | trees
	focusId     string // element opened within a layer/graph view (e.g. the layer actor after /layers/, or an actor)
}

// graphMode returns the requested graph display mode, defaulting to "layers".
func (p linkParams) graphMode() string {
	if p.mode == "" {
		return "layers"
	}
	return p.mode
}

// require returns val or an error stating which id the entity needs.
func require(val, name, entity string) (string, error) {
	if val == "" {
		return "", fmt.Errorf("entity %q requires %s", entity, name)
	}
	return val, nil
}

// linkEntities is the curated route table, mirroring the relevant entries of
// routes.json. Add a row here to support a new deep-link kind.
var linkEntities = []linkEntity{
	{
		name: "actor",
		help: "Actor detail page. id = actor UUID.",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (actor UUID)", "actor")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("/actors_graph/%s/view/%s", acc, id), nil
		},
	},
	{
		name: "actorList",
		help: "Tabular list of all actors in the workspace. No id needed.",
		build: func(acc string, _ linkParams) (string, error) {
			return fmt.Sprintf("/actors_graph/%s/list", acc), nil
		},
	},
	{
		name: "graph",
		help: "Main graph canvas (all actors/edges). No id needed; optional mode (layers|actors|trees).",
		build: func(acc string, p linkParams) (string, error) {
			return fmt.Sprintf("/actors_graph/%s/graph", acc), nil
		},
	},
	{
		name: "layer",
		help: "Graph layer/folder canvas, shaped /actors_graph/<acc>/graph/<id>/layers/<focusId>. " +
			"id = the graph/folder being opened — a Graphs container actor (the UUID after /graph/), a layer/folder actor, or \"0\" for the root layer. " +
			"Optional mode (layers|actors|trees, default layers) and focusId — the element opened within it, e.g. the specific layer actor that appears after /layers/. " +
			"(To READ a layer's nodes/edges, don't build a link — call getLayerActorsPaginated with that layer's actorId.)",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (layer UUID or \"0\")", "layer")
			if err != nil {
				return "", err
			}
			path := fmt.Sprintf("/actors_graph/%s/graph/%s/%s", acc, id, p.graphMode())
			if p.focusId != "" {
				path += "/" + p.focusId
			}
			return path, nil
		},
	},
	{
		name: "event",
		help: "Event stream list. Optional streamId, then secondaryId (sub-stream) to narrow the feed.",
		build: func(acc string, p linkParams) (string, error) {
			path := fmt.Sprintf("/events/%s/list", acc)
			if p.streamId != "" {
				path += "/" + p.streamId
				if p.secondaryId != "" {
					path += "/" + p.secondaryId
				}
			}
			return path, nil
		},
	},
	{
		name: "chat",
		help: "Chat conversation. id = chat conversation (chat-actor / Events-actor) UUID — opens that chat. " +
			"Optional streamId names the chat stream (defaults to the standard \"chats\" stream). Omit id to open the chat list.",
		build: func(acc string, p linkParams) (string, error) {
			if p.id == "" {
				// No specific conversation: open the workspace chat list.
				return fmt.Sprintf("/chats/%s", acc), nil
			}
			// Conversation deep-link: /chats/<acc>/list/<stream>/<chatId>?tab=chat.
			// The stream segment is the literal "chats" for the standard chat
			// stream unless the caller names a different stream.
			stream := p.streamId
			if stream == "" {
				stream = "chats"
			}
			return fmt.Sprintf("/chats/%s/list/%s/%s?tab=chat", acc, stream, p.id), nil
		},
	},
	{
		name: "form",
		help: "Form editor. id = form UUID to edit; omit id to open a blank new form.",
		build: func(acc string, p linkParams) (string, error) {
			path := fmt.Sprintf("/form/%s/edit", acc)
			if p.id != "" {
				path += "/" + p.id
			}
			return path, nil
		},
	},
	{
		name: "transaction",
		help: "Financial transaction detail. id = transaction UUID.",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (transaction UUID)", "transaction")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("/actors_graph/%s/transactions/view/%s", acc, id), nil
		},
	},
	{
		name: "transfer",
		help: "Financial transfer detail. id = transfer UUID.",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (transfer UUID)", "transfer")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("/actors_graph/%s/transfers/view/%s", acc, id), nil
		},
	},
	{
		name: "accounts",
		help: "Financial accounts view for the workspace. No id needed.",
		build: func(acc string, _ linkParams) (string, error) {
			return fmt.Sprintf("/actors_graph/%s/accounts", acc), nil
		},
	},
	{
		name: "chart",
		help: "Dashboard chart/widget. id = chart UUID.",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (chart UUID)", "chart")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("/dashboard/%s/view/%s", acc, id), nil
		},
	},
	{
		name: "meeting",
		help: "Video meeting room tied to an actor. id = actor UUID.",
		build: func(acc string, p linkParams) (string, error) {
			id, err := require(p.id, "id (actor UUID)", "meeting")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("/meetingRoom/%s/%s", acc, id), nil
		},
	},
	{
		name: "settings",
		help: "Workspace settings panel. No id needed.",
		build: func(acc string, _ linkParams) (string, error) {
			return fmt.Sprintf("/settings/%s", acc), nil
		},
	},
}

// linkEntityNames returns the supported entity selectors, in table order.
func linkEntityNames() []string {
	names := make([]string, 0, len(linkEntities))
	for _, e := range linkEntities {
		names = append(names, e.name)
	}
	return names
}

// linkEntityHelp returns the "entity — what it needs" lines for the description.
func linkEntityHelp() string {
	var b strings.Builder
	for _, e := range linkEntities {
		fmt.Fprintf(&b, "\n  • %s — %s", e.name, e.help)
	}
	return b.String()
}

// findLinkEntity returns the entity row for name, or false.
func findLinkEntity(name string) (linkEntity, bool) {
	for _, e := range linkEntities {
		if e.name == name {
			return e, true
		}
	}
	return linkEntity{}, false
}

// webBaseURL derives the web-app base URL from the API base URL by dropping the
// "/papi/1.0" suffix. Both trailing slashes and the suffix are handled.
func webBaseURL(apiBase string) string {
	b := strings.TrimRight(apiBase, "/")
	b = strings.TrimSuffix(b, "/papi/1.0")
	return strings.TrimRight(b, "/")
}

// shortAcc returns the 8-char workspace segment used in web URLs. A full account
// UUID (8 hex chars, then a dash) is truncated to its first 8 chars; any other
// value (already short) is returned unchanged.
func shortAcc(acc string) string {
	if len(acc) > 8 && acc[8] == '-' {
		return acc[:8]
	}
	return acc
}

// registerBuildLink adds the buildLink deep-link builder to s.
func registerBuildLink(s *server.MCPServer, c *apiclient.Client) {
	desc := "Build an absolute Simulator web-app URL (deep-link) the user can click for an entity — " +
		"an actor, event, chat, graph layer, transaction, etc. Resolves the web base URL and the " +
		"workspace automatically; you supply the entity kind and its id(s). " +
		"accId defaults to the active workspace. When the UI context is present (AI-agent flow), the " +
		"web base, workspace, and the user's current view default from where they are — so for the " +
		"OPEN actor omit id on entity=actor, and for the OPEN layer omit id on entity=layer. " +
		"Supported entities and the ids they need:" +
		linkEntityHelp()

	tool := mcp.NewTool("buildLink",
		mcp.WithDescription(desc),
		mcp.WithString("entity", mcp.Required(),
			mcp.Description("Kind of link to build. One of: "+strings.Join(linkEntityNames(), ", ")+".")),
		mcp.WithString("accId", mcp.Description("Workspace id. Defaults to the active workspace if omitted.")),
		mcp.WithString("id", mcp.Description("Primary resource id; meaning depends on entity (actor/transaction/transfer/chart/meeting/form UUID, the graph/folder id after /graph/ or \"0\" for a layer link, or chat conversation (chat-actor) UUID). See the entity list in the description.")),
		mcp.WithString("streamId", mcp.Description("Stream UUID — for event links, and to override the default \"chats\" stream on chat links.")),
		mcp.WithString("secondaryId", mcp.Description("Event sub-stream UUID — narrows an event link further.")),
		mcp.WithString("mode", mcp.Description("Graph display mode for layer/graph links."),
			mcp.Enum("layers", "actors", "trees")),
		mcp.WithString("focusId", mcp.Description("UUID opened within a layer/graph view — e.g. the specific layer actor (the segment after /layers/), or an actor to focus on.")),
	)

	s.AddTool(tool, buildLinkHandler(c))
}

// buildLinkHandler returns the buildLink tool handler. Split out from
// registerBuildLink so it can be exercised directly in tests.
func buildLinkHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		entityName, _ := args["entity"].(string)
		if entityName == "" {
			return mcp.NewToolResultError("[Error] buildLink: missing required parameter \"entity\""), nil
		}
		entity, ok := findLinkEntity(entityName)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] buildLink: unknown entity %q — one of: %s",
				entityName, strings.Join(linkEntityNames(), ", "))), nil
		}

		// UI context (control-events-context) — where the user is in the web UI.
		// Used as defaults so "link to the current actor/layer" needs no ids.
		ui := apiclient.UIContextFromContext(ctx)

		acc, _ := args["accId"].(string)
		if acc == "" {
			acc = c.WorkspaceIDForContext(ctx)
		}
		if acc == "" {
			acc = ui.WorkspaceID // the workspace the user is currently in
		}
		if acc == "" {
			return mcp.NewToolResultError("[Error] buildLink: no workspace set — pass accId or run set-workspace first"), nil
		}

		params := linkParams{
			id:          str(args["id"]),
			streamId:    str(args["streamId"]),
			secondaryId: str(args["secondaryId"]),
			mode:        str(args["mode"]),
			focusId:     str(args["focusId"]),
		}
		// Default the primary id from the user's current view when the caller
		// omits it: the open actor for actor/meeting links, the open layer for
		// layer links. An explicit id always wins.
		if params.id == "" {
			switch entityName {
			case "actor", "meeting":
				params.id = ui.ActiveActor
			case "layer":
				// Canonical open-layer URL is /graph/<graph folder>/layers/<layer>:
				// the graph (folder) fills the /graph/ path slot and the open layer
				// is the focused element after /layers/. Use it only when the context
				// gives a graph folder distinct from the layer itself.
				if ui.ActiveGraph != "" && ui.ActiveGraph != ui.ActiveLayer {
					params.id = ui.ActiveGraph
					if params.focusId == "" {
						params.focusId = ui.ActiveLayer
					}
				} else {
					// Older platforms send only ActiveLayer (no ActiveGraph), or a
					// degenerate ActiveGraph == ActiveLayer — put the layer in the path
					// slot (/graph/<layer>/layers). The layer now occupies that slot, so
					// drop any focusId rather than emit /graph/<layer>/layers/<focusId>,
					// which would read the focus segment as a nested layer.
					params.id = ui.ActiveLayer
					params.focusId = ""
				}
			}
		}
		path, err := entity.build(shortAcc(acc), params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] buildLink: %v", err)), nil
		}

		// Web base: prefer the origin the user is actually on (UI context), then a
		// per-request override, then the configured API base (minus /papi/1.0).
		base := strings.TrimRight(ui.HostOrigin, "/")
		if base == "" {
			apiBase := apiclient.BaseURLFromContext(ctx)
			if apiBase == "" {
				apiBase = c.BaseURL()
			}
			base = webBaseURL(apiBase)
		}
		if base == "" {
			return mcp.NewToolResultError("[Error] buildLink: no environment set — run set-environment first"), nil
		}
		return mcp.NewToolResultText(base + path), nil
	}
}

// str coerces an argument to a trimmed string ("" if absent or not a string).
func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
