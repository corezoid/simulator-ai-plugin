package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// resolveActorFormID lets createActor accept a friendly `formName` instead of a
// numeric `formId`: if formId is absent but formName is given, it looks the form
// up by title in the active workspace and fills formId.
func resolveActorFormID(ctx context.Context, args map[string]any, c *apiclient.Client) error {
	if _, ok := args["formId"]; ok {
		return nil // explicit formId wins
	}
	name, _ := args["formName"].(string)
	if name == "" {
		return nil // neither given — the path check reports the missing formId
	}
	accID := c.WorkspaceID()
	if accID == "" {
		return fmt.Errorf("resolving formName needs a workspace — run set-workspace or pass formId")
	}
	resp, err := c.Do(ctx, "GET", "/forms/templates/"+accID, nil, nil)
	if err != nil {
		return fmt.Errorf("list forms to resolve %q: %w", name, err)
	}
	var out struct {
		Data []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return fmt.Errorf("parse forms list: %w", err)
	}
	for _, f := range out.Data {
		if f.Title == name {
			args["formId"] = float64(f.ID) // JSON-number arg type, like the model would send
			return nil
		}
	}
	return fmt.Errorf("form %q not found in the active workspace", name)
}

// actorOps — actor (graph node) CRUD. Actors are instances of a form; their
// fields live in the free-form `data` object keyed by the form's field schema.
var actorOps = []Operation{
	{
		Name: "createActor", Method: "POST", Path: "/actors/actor/{formId}",
		Summary: "Create an actor (graph node) of a given form. Pass `formId` (number) or `formName` (resolved to its id). `data` holds the field values keyed by the form's schema.",
		Resolve: resolveActorFormID,
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Desc: "Form id this actor instantiates. Provide formId or formName."},
			{Name: "formName", In: InLocal, Type: "string", Desc: "Form name — resolved to its id via the active workspace. Provide formId or formName."},
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
	{
		Name: "searchActors", Method: "GET", Path: "/actors_filters/search/{accId}/{query}",
		Summary: "Search actors across a workspace by title/text. Use before createActor to check whether an actor already exists.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search query (title or fragment)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
	{
		Name: "searchLayerActors", Method: "GET", Path: "/layer_actors_filters/search/{actorId}/{query}",
		Summary: "Search actors placed on a specific layer by title/text.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search query (title or fragment)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
}
