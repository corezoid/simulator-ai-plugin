package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// resolveHierarchyEdgeType defaults a tool's edgeTypeId to the workspace's
// hierarchy link type when the caller omits it — actor links are almost always
// the hierarchy edge (both for traversal in getRelatedActors and for creating
// links in createLink). Edge type ids are workspace-scoped, so the id is resolved
// by name via the edge_types listing. An explicit edgeTypeId always wins.
func resolveHierarchyEdgeType(ctx context.Context, args map[string]any, c *apiclient.Client) error {
	if _, ok := args["edgeTypeId"]; ok {
		return nil // explicit edgeTypeId wins
	}
	id, err := hierarchyEdgeTypeID(ctx, c)
	if err != nil {
		return err
	}
	args["edgeTypeId"] = float64(id) // JSON-number arg type, like the model would send
	return nil
}

// resolveHierarchyForLinks defaults each link's edgeTypeId in a massLink call to
// the hierarchy type when omitted — same rationale as resolveHierarchyEdgeType,
// applied per array element. Links with an explicit edgeTypeId are left as-is.
func resolveHierarchyForLinks(ctx context.Context, args map[string]any, c *apiclient.Client) error {
	raw, ok := args["links"].([]any)
	if !ok {
		return nil
	}
	needsDefault := false
	for _, l := range raw {
		if m, ok := l.(map[string]any); ok {
			if _, has := m["edgeTypeId"]; !has {
				needsDefault = true
				break
			}
		}
	}
	if !needsDefault {
		return nil
	}
	id, err := hierarchyEdgeTypeID(ctx, c)
	if err != nil {
		return err
	}
	for _, l := range raw {
		if m, ok := l.(map[string]any); ok {
			if _, has := m["edgeTypeId"]; !has {
				m["edgeTypeId"] = float64(id)
			}
		}
	}
	return nil
}

// hierarchyEdgeTypeID resolves the active workspace's hierarchy edge type id by
// name (edge type ids are workspace-scoped).
func hierarchyEdgeTypeID(ctx context.Context, c *apiclient.Client) (int, error) {
	accID := c.WorkspaceIDForContext(ctx)
	if accID == "" {
		return 0, fmt.Errorf("resolving the hierarchy edge type needs a workspace — run set-workspace or pass edgeTypeId")
	}
	resp, err := c.Do(ctx, "GET", "/edge_types/"+accID, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("list edge types to resolve hierarchy: %w", err)
	}
	var out struct {
		Data []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return 0, fmt.Errorf("parse edge types list: %w", err)
	}
	for _, et := range out.Data {
		if et.Name == "hierarchy" {
			return et.ID, nil
		}
	}
	return 0, fmt.Errorf("hierarchy edge type not found in the active workspace — pass edgeTypeId explicitly")
}

// graphOps — build process graphs: links (edges) between actors, placing nodes
// on layers, and reading edge types / layer membership. Layers themselves are
// actors (created via createActor with the Layer system form).
var graphOps = []Operation{
	{
		Name: "createLink", Method: "POST", Path: "/actors/link/{accId}",
		Summary: "Create a directed link (edge) between two actors. Defaults to the " +
			"workspace's hierarchy link type — omit edgeTypeId for normal actor links.",
		Resolve: resolveHierarchyEdgeType,
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "source", In: InBody, Type: "string", Required: true, Desc: "Source actor UUID."},
			{Name: "target", In: InBody, Type: "string", Required: true, Desc: "Target actor UUID."},
			{Name: "edgeTypeId", In: InBody, Type: "number", Desc: "Edge type id. Defaults to the workspace's hierarchy link type if omitted — omit it for normal actor links; pass it only to use a different edge type (see getEdgeTypes)."},
			{Name: "name", In: InBody, Type: "string", Desc: "Optional edge label."},
			{Name: "weight", In: InBody, Type: "number", Desc: "Optional edge weight."},
			{Name: "curveStyle", In: InBody, Type: "string", Desc: "Optional curve style."},
			{Name: "linkedActorId", In: InBody, Type: "string", Desc: "Optional actor UUID this edge is associated with (e.g. a reaction/widget actor on the link)."},
			{Name: "pinned", In: InBody, Type: "boolean", Desc: "Pin the edge (excluded from auto-prune)."},
			{Name: "forceDirection", In: InQuery, Type: "boolean", Desc: "Force the edge direction (skip the hierarchy invert-dedup)."},
		},
	},
	{
		Name: "massLink", Method: "POST", Path: "/actors/mass_links/{accId}",
		Summary: "Create up to 50 links in one call. Pass an array of {source, target, edgeTypeId?, name?, weight?, curveStyle?, linkedActorId?, pinned?} edge objects. edgeTypeId defaults to the workspace's hierarchy link type per object when omitted.",
		Resolve: resolveHierarchyForLinks,
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "links", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of edge objects: {source, target, edgeTypeId?, name?, weight?, curveStyle?, linkedActorId?, pinned?} (max 50). Omit edgeTypeId for normal actor links — it defaults to the workspace's hierarchy link type; set it only for a different edge type (see getEdgeTypes)."},
			{Name: "forceDirection", In: InQuery, Type: "boolean", Desc: "Force edge directions."},
		},
	},
	{
		Name: "getEdge", Method: "GET", Path: "/actors/link/{edgeId}",
		Summary: "Get a single link (edge) by its UUID, including its source/target actors and access privileges.",
		Params: []Param{
			{Name: "edgeId", In: InPath, Type: "string", Required: true, Desc: "Edge UUID."},
			{Name: "linkedActor", In: InQuery, Type: "boolean", Desc: "Also resolve and include the edge's linkedActor."},
		},
	},
	{
		Name: "updateEdge", Method: "PUT", Path: "/actors/link/{edgeId}",
		Summary: "Update a link's editable fields (label, associated actor, curve style, pinned). Only the provided fields change.",
		Params: []Param{
			{Name: "edgeId", In: InPath, Type: "string", Required: true, Desc: "Edge UUID."},
			{Name: "name", In: InBody, Type: "string", Desc: "Edge label."},
			{Name: "linkedActorId", In: InBody, Type: "string", Desc: "Associated actor UUID (null/empty to clear)."},
			{Name: "curveStyle", In: InBody, Type: "string", Desc: "Curve style."},
			{Name: "pinned", In: InBody, Type: "boolean", Desc: "Pin/unpin the edge."},
		},
	},
	{
		Name: "deleteEdge", Method: "DELETE", Path: "/actors/link/{edgeId}",
		Summary: "Delete a link (edge) by its UUID. Irreversible — confirm with the user first. " +
			"Permanent/system edge types (e.g. the hierarchy link) and form-field edges cannot be deleted.",
		Params: []Param{
			{Name: "edgeId", In: InPath, Type: "string", Required: true, Desc: "Edge UUID."},
		},
	},
	{
		Name: "existLink", Method: "POST", Path: "/actors/exist_link",
		Summary: "Check whether a link exists between two actors for an edge type. Returns the matching edge(s). " +
			"Use before createLink to avoid duplicates, or to find an edge's id by its (source, target, edgeTypeId).",
		Params: []Param{
			{Name: "source", In: InBody, Type: "string", Required: true, Desc: "Source actor UUID."},
			{Name: "target", In: InBody, Type: "string", Required: true, Desc: "Target actor UUID."},
			{Name: "edgeTypeId", In: InBody, Type: "number", Required: true, Desc: "Edge type id (see getEdgeTypes)."},
			{Name: "bidirected", In: InBody, Type: "boolean", Desc: "Also check the reverse direction (target→source). Default false."},
		},
	},
	{
		Name: "deleteEdgesByNodes", Method: "DELETE", Path: "/actors/bulk/actors_link",
		Summary: "Delete links identified by their (source, target, edgeTypeId) endpoints rather than edge ids, in bulk (1-200). " +
			"Irreversible — confirm first. Each item may set bidirected to also remove the reverse edge.",
		Params: []Param{
			{Name: "links", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of {source, target, edgeTypeId, bidirected?} objects (1-200)."},
			{Name: "force", In: InQuery, Type: "boolean", Desc: "Force removal of edges that are otherwise protected (e.g. form-field edges)."},
		},
	},
	{
		Name: "getEdgeTypes", Method: "GET", Path: "/edge_types/{accId}",
		Summary: "List the edge (link) types available in a workspace, including the hierarchy type.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "isSystem", In: InQuery, Type: "boolean", Desc: "Only system edge types."},
			fieldFilterParam("id,name"),
		},
	},
	{
		Name: "getLayerActors", Method: "GET", Path: "/graph_layers/{actorId}",
		Summary: "List the actors placed on a layer (the layer is itself an actor). `filter` projects the fields of each placed actor (node), keeping the response small.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "noDuplicate", In: InQuery, Type: "boolean", Desc: "Deduplicate placements."},
			fieldFilterParam("id,title,formId,status,x,y"),
		},
	},
	{
		Name: "manageLayerActors", Method: "POST", Path: "/graph_layers/actors/{actorId}",
		Summary: "Place or remove nodes/edges on a layer. Pass an array of {action: create|delete, data: {id, type: node|edge, position?}} items. An edge placement may also carry a line style — see `items`.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of {action, data:{id, type, position?, laIdSource?, laIdTarget?}} actions. " +
				"For an EDGE (type:\"edge\"), data also accepts `layerSettings:{lineStyle:\"solid\"|\"dashed\"|\"dotted\", color:\"#RRGGBB\" (line colour), width:\"2\" (stroke width, string)}` to style the line — dash pattern, colour and thickness (omit → solid default). " +
				"To CHANGE an existing edge's style, delete its placement and create it again with the new lineStyle — re-creating without deleting first adds a duplicate edge placement."},
			{Name: "withNum", In: InQuery, Type: "boolean", Desc: "Return counts in the response."},
		},
	},
	{
		Name: "getRelatedActors", Method: "GET", Path: "/graph/{type}/{actorId}",
		Summary: "List actors linked to an actor. `type` selects direction: linked (both), parents (incoming edges), children (outgoing edges). Traverses the workspace's hierarchy link type by default — only pass edgeTypeId to traverse a different edge type. Returns a paginated, sortable, filterable list of related actors with a total. For neighbours across multiple edge types or to include system/pinned links, use getLinkedActors instead. Pass `filter` to return only the fields you need — result lists can be large.",
		Resolve: resolveHierarchyEdgeType,
		Params: []Param{
			{Name: "type", In: InPath, Type: "string", Required: true, Enum: []string{"linked", "parents", "children"}, Desc: "Relation direction relative to the anchor actor."},
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Anchor actor UUID — the actor whose related actors you want."},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Edge type id to traverse. Defaults to the workspace's hierarchy link type (resolved automatically). Pass it only to traverse a non-hierarchy edge type (see getEdgeTypes)."},
			{Name: "formId", In: InQuery, Type: "number", Desc: "Only return related actors of this form."},
			{Name: "exceptFormId", In: InQuery, Type: "number", Desc: "Exclude related actors of this form."},
			{Name: "search", In: InQuery, Type: "string", Desc: "Full-text search on related actor title."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Created-after filter, unixtime in seconds."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Created-before filter, unixtime in seconds."},
			{Name: "orderBy", In: InQuery, Type: "string", Enum: []string{"created_at", "updated_at", "formTitle", "title", "owner"}, Desc: "Sort field (default created_at)."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"DESC", "ASC"}, Desc: "Sort direction (default DESC)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "pinned", In: InQuery, Type: "boolean", Desc: "Only pinned edges."},
			fieldFilterParam("id,title,ref,formId,status"),
		},
	},
	{
		Name: "getActorLinks", Method: "GET", Path: "/graph/actor_links/{actorId}",
		Summary: "List the links (edges) of an actor — every edge where it is the source or target. Pass `filter` to return only the fields you need — result lists can be large.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID whose links to list."},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Only links of this edge type (see getEdgeTypes)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,source,target,edgeTypeId,name,weight"),
		},
	},
	{
		Name: "getLinkedActors", Method: "GET", Path: "/graph/linked_actors/{actorId}",
		Summary: "List the actors directly linked to an actor (across edge types), with edge-type and system filters. " +
			"Use getRelatedActors instead when you need a single hierarchy direction (parents/children).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Anchor actor UUID."},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Only neighbours reached via this edge type."},
			{Name: "edgeTypes", In: InQuery, Type: "string", Desc: "Comma-separated edge type ids to traverse."},
			{Name: "withSystem", In: InQuery, Type: "boolean", Desc: "Include system edges/actors."},
			{Name: "pinned", In: InQuery, Type: "boolean", Desc: "Only neighbours reached via pinned edges."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,title,ref,formId,status"),
		},
	},
	{
		Name: "layerStats", Method: "GET", Path: "/graph_layers/stats/{actorId}",
		Summary: "Get statistics for a layer (node/edge counts and related aggregates).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
		},
	},
	{
		Name: "existLayerElement", Method: "POST", Path: "/graph_layers/exist/{actorId}",
		Summary: "Check whether a node or edge is present on a layer (use before placing to avoid duplicates).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "id", In: InBody, Type: "string", Required: true, Desc: "Element id (actor UUID for a node, edge UUID for an edge)."},
			{Name: "type", In: InBody, Type: "string", Required: true, Enum: []string{"node", "edge"}, Desc: "Element kind."},
		},
	},
	{
		Name: "moveActors", Method: "POST", Path: "/graph_layers/move/{sourceActorId}/{targetActorId}",
		Summary: "Move actors from one layer to another (up to 10 at once), optionally at given positions.",
		Params: []Param{
			{Name: "sourceActorId", In: InPath, Type: "string", Required: true, Desc: "Source layer actor UUID."},
			{Name: "targetActorId", In: InPath, Type: "string", Required: true, Desc: "Target layer actor UUID."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-10) of {actorId:string (required), laId?:int (layer-actor placement id), position?:{x:number,y:number}}."},
		},
	},
	{
		Name: "updateLayerPositions", Method: "PUT", Path: "/graph_layers/actors/{actorId}",
		Summary: "Update the positions (x, y) of actors already present on a layer. Use this to reposition actors within the same layer — not to move actors between layers (use moveActors for that).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID containing the actors to reposition."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-100) of {id: string (laId — unique element ID on this layer, NOT actorId), position: {x: int, y: int}}."},
		},
	},
	{
		Name: "cleanGraphLayer", Method: "DELETE", Path: "/graph_layers/clean/{actorId}",
		Summary: "Remove ALL actors and links from a layer (the layer actor itself stays). Irreversible — confirm first.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID to clear."},
		},
	},
}
