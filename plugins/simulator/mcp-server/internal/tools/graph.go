package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// resolveHierarchyEdgeType defaults getRelatedActors to the workspace's
// hierarchy link type when the caller omits edgeTypeId — linked actors are
// almost always traversed along the hierarchy edge. Edge type ids are
// workspace-scoped, so the id is resolved by name via the edge_types listing.
func resolveHierarchyEdgeType(ctx context.Context, args map[string]any, c *apiclient.Client) error {
	if _, ok := args["edgeTypeId"]; ok {
		return nil // explicit edgeTypeId wins
	}
	accID := c.WorkspaceID()
	if accID == "" {
		return fmt.Errorf("resolving the hierarchy edge type needs a workspace — run set-workspace or pass edgeTypeId")
	}
	resp, err := c.Do(ctx, "GET", "/edge_types/"+accID, nil, nil)
	if err != nil {
		return fmt.Errorf("list edge types to resolve hierarchy: %w", err)
	}
	var out struct {
		Data []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return fmt.Errorf("parse edge types list: %w", err)
	}
	for _, et := range out.Data {
		if et.Name == "hierarchy" {
			args["edgeTypeId"] = float64(et.ID) // JSON-number arg type, like the model would send
			return nil
		}
	}
	return fmt.Errorf("hierarchy edge type not found in the active workspace — pass edgeTypeId explicitly")
}

// graphOps — build process graphs: links (edges) between actors, placing nodes
// on layers, and reading edge types / layer membership. Layers themselves are
// actors (created via createActor with the Layer system form).
var graphOps = []Operation{
	{
		Name: "createLink", Method: "POST", Path: "/actors/link/{accId}",
		Summary: "Create a directed link (edge) between two actors.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "source", In: InBody, Type: "string", Required: true, Desc: "Source actor UUID."},
			{Name: "target", In: InBody, Type: "string", Required: true, Desc: "Target actor UUID."},
			{Name: "edgeTypeId", In: InBody, Type: "number", Required: true, Desc: "Edge type id (see getEdgeTypes)."},
			{Name: "name", In: InBody, Type: "string", Desc: "Optional edge label."},
			{Name: "weight", In: InBody, Type: "number", Desc: "Optional edge weight."},
			{Name: "curveStyle", In: InBody, Type: "string", Desc: "Optional curve style."},
			{Name: "forceDirection", In: InQuery, Type: "boolean", Desc: "Force the edge direction."},
		},
	},
	{
		Name: "massLink", Method: "POST", Path: "/actors/mass_links/{accId}",
		Summary: "Create up to 50 links in one call. Pass an array of {source, target, edgeTypeId, ...} edge objects.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "links", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of edge objects: {source, target, edgeTypeId, name?, weight?} (max 50)."},
			{Name: "forceDirection", In: InQuery, Type: "boolean", Desc: "Force edge directions."},
		},
	},
	{
		Name: "getEdgeTypes", Method: "GET", Path: "/edge_types/{accId}",
		Summary: "List the edge (link) types available in a workspace, including the hierarchy type.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "isSystem", In: InQuery, Type: "boolean", Desc: "Only system edge types."},
		},
	},
	{
		Name: "getLayerActors", Method: "GET", Path: "/graph_layers/{actorId}",
		Summary: "List the actors placed on a layer (the layer is itself an actor).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "noDuplicate", In: InQuery, Type: "boolean", Desc: "Deduplicate placements."},
		},
	},
	{
		Name: "manageLayerActors", Method: "POST", Path: "/graph_layers/actors/{actorId}",
		Summary: "Place or remove nodes/edges on a layer. Pass an array of {action: create|delete, data: {id, type: node|edge, position?}} items.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of {action, data:{id, type, position?, laIdSource?, laIdTarget?}} actions."},
			{Name: "withNum", In: InQuery, Type: "boolean", Desc: "Return counts in the response."},
		},
	},
	{
		Name: "getRelatedActors", Method: "GET", Path: "/graph/{type}/{actorId}",
		Summary: "List actors linked to an actor. `type` selects direction: linked (both), parents (incoming edges), children (outgoing edges). Traverses the workspace's hierarchy link type by default — only pass edgeTypeId to traverse a different edge type. Returns a paginated, sortable, filterable list of related actors with a total.",
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
			{Name: "filter", In: InQuery, Type: "string", Desc: "Data filter expression on actor fields."},
		},
	},
}
