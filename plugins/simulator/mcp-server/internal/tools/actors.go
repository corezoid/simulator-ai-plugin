package tools

// actorOps — actor (graph node) CRUD. Actors are instances of a form; their
// fields live in the free-form `data` object keyed by the form's field schema.
var actorOps = []Operation{
	{
		Name: "createActor", Method: "POST", Path: "/actors/actor/{formId}",
		Summary: "Create an actor (graph node) of a given form. `data` holds the field values keyed by the form's schema.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id this actor instantiates."},
			{Name: "data", In: InBody, Type: "object", Required: true, Desc: "Field values keyed by the form's field names."},
			{Name: "title", In: InBody, Type: "string", Desc: "Actor title shown on the graph."},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
			{Name: "color", In: InBody, Type: "string", Desc: "Hex node color (e.g. #409547)."},
			{Name: "picture", In: InBody, Type: "string", Desc: "Storage path / URL of the node icon."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Optional external reference (1-255 chars), unique per form."},
			{Name: "contextLayerId", In: InQuery, Type: "string", Desc: "Optional layer to place the new actor on."},
		},
	},
	{
		Name: "getActor", Method: "GET", Path: "/actors/{actorId}",
		Summary: "Get an actor by its UUID.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
		},
	},
	{
		Name: "getActorByRef", Method: "GET", Path: "/actors/ref/{formId}/{ref}",
		Summary: "Look up an actor by its (formId, ref) external reference.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "External reference."},
		},
	},
	{
		Name: "updateActor", Method: "PUT", Path: "/actors/actor/{formId}/{actorId}",
		Summary: "Update an actor's fields/metadata. Only the provided fields change.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id the actor belongs to."},
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "data", In: InBody, Type: "object", Desc: "Field values to update."},
			{Name: "title", In: InBody, Type: "string", Desc: "New title."},
			{Name: "description", In: InBody, Type: "string", Desc: "New description."},
			{Name: "color", In: InBody, Type: "string", Desc: "New hex color."},
			{Name: "status", In: InBody, Type: "string", Desc: "New status value."},
		},
	},
	{
		Name: "deleteActor", Method: "DELETE", Path: "/actors/{actorId}",
		Summary: "Delete an actor by UUID. This is irreversible — confirm with the user first.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
		},
	},
	{
		Name: "setActorStatus", Method: "PUT", Path: "/actors/status/{actorId}",
		Summary: "Set an actor's status.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "status", In: InBody, Type: "string", Desc: "New status value."},
		},
	},
}
