package tools

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
}
