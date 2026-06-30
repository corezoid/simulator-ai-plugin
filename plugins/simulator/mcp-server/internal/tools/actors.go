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
	accID := c.WorkspaceIDForContext(ctx)
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

// actorDataDesc documents the actor `data` payload. Actor data is keyed by each
// form field's `id` ("item_<digits>"), and each value's shape depends on that
// field's class. See docs/entities/actors.md and docs/entities/forms.md.
const actorDataDesc = "Field values keyed by each form field's `id` (\"item_<digits>\" — NOT its title or `key`; read the form with getForm first). " +
	"Value shape per field class: " +
	"edit text/password/email/phone → string; edit int/float → number; " +
	"check → boolean; radio → the selected option's `value` (string); " +
	"static select/multiSelect → array of the chosen option objects [{title,value,color?}]; " +
	"dynamic select → array of [{type,title,value}] where type is actor (value=actor UUID, for layer/actorFilter/actorsBag/actors/formFilter sources — formFilter lists the actors of a given form), " +
	"currency (value=currency id number), accountName (value=account-name UUID), workspaceMembers (value=user id number), or form (value=form id, for the forms source); " +
	"calendar → {startDate,endDate,timeZoneOffset,sendInvite} with startDate/endDate as unix SECONDS; upload → array of file refs. " +
	"Display-only classes (label, button, image) take NO data entry; omit hidden/disabled fields you do not set. " +
	"MULTIFORM actors instantiate several forms at once: fields of the actor's own (root) formId use the plain item id, while fields belonging to ANOTHER form are keyed \"__form__<thatFormId>:<itemId>\" (e.g. \"__form__16951:position\"). Use the plain id for this form's own fields."

// holeDesc documents the `hole` flag, exposed on createActor/updateActor. The
// backend declares `hole` (boolean) on the actor create/update body; a hole is
// an empty placeholder node that becomes a normal actor once it is filled.
const holeDesc = "Mark this actor as a HOLE — an empty placeholder slot on the graph (rendered as a " +
	"hollow node). A hole stands for a not-yet-filled position in the structure and becomes a normal " +
	"actor once it is filled with data. Default false."

// pictureObjectDesc documents the `pictureObject` field: ANY custom image rendered
// AS the node body (the backend's "napkin" element) instead of a standard form
// node — a line, a shape, an icon, a logo, a small diagram. A divider line is just
// one use.
const pictureObjectDesc = "Render a custom image AS the node itself — the backend's \"napkin\" element — instead of a normal form node. " +
	"The image can be ANYTHING a PNG/SVG holds: a line, a circle, a rectangle, an icon, a logo, a hand-drawn shape, a small diagram. " +
	"Object shape: {\"img\": \"data:image/png;base64,…\" (a PNG/SVG data URI), \"width\": 800, \"height\": 8 (display size in px), \"type\": \"napkin\"}. " +
	"The image is anchored at its CENTRE and keeps the source's aspect ratio (set width; height follows) — e.g. for a thin divider line use a wide-and-short source PNG. " +
	"Uses: dividers/separators, a custom shape or icon the form catalogue does not cover, an embedded picture or logo on a graph."

// actorOps — actor (graph node) CRUD. Actors are instances of a form; their
// fields live in the free-form `data` object keyed by the form's field schema.
var actorOps = []Operation{
	{
		Name: "createActor", Method: "POST", Path: "/actors/actor/{formId}",
		Summary: "Create an actor (graph node) of a given form. Pass `formId` (number) or `formName` (resolved to its id). `data` holds the field values keyed by the form's schema. " +
			"In a UAT (form-tree) workspace the formId must be the ROOT form of the tree: if the requested form has a non-empty parentId (a leaf/child form), creating directly under it returns \"400: Form <id> is not UAT\" — walk up parentId to the root, create under the root, and put the leaf form's fields under \"__form__<leafFormId>:<itemId>\" data keys.",
		Resolve: resolveActorFormID,
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Desc: "Form id this actor instantiates. Provide formId or formName."},
			{Name: "formName", In: InLocal, Type: "string", Desc: "Form name — resolved to its id via the active workspace. Provide formId or formName."},
			{Name: "data", In: InBody, Type: "object", Required: true, Desc: actorDataDesc},
			{Name: "title", In: InBody, Type: "string", Desc: "Actor title shown on the graph."},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
			{Name: "color", In: InBody, Type: "string", Desc: "Hex node color (e.g. #409547)."},
			{Name: "picture", In: InBody, Type: "string", Desc: "Storage path / URL of the node icon."},
			{Name: "pictureObject", In: InBody, Type: "object", Desc: pictureObjectDesc},
			{Name: "ref", In: InBody, Type: "string", Desc: "Optional external reference (1-255 chars), unique per form."},
			{Name: "appId", In: InBody, Type: "string", Desc: appIdDesc},
			{Name: "appSettings", In: InBody, Type: "object", Desc: appSettingsDesc},
			{Name: "hole", In: InBody, Type: "boolean", Desc: holeDesc},
			{Name: "contextLayerId", In: InQuery, Type: "string", Desc: "Optional layer to place the new actor on."},
		},
	},
	{
		Name: "getActor", Method: "GET", Path: "/actors/{actorId}",
		Summary: "Get an actor by its UUID. Needs no workspace/accId — the UUID resolves it. " +
			"ALWAYS pass `filter`: unfiltered, this returns the actor's entire form schema TWICE " +
			"(`form`, kept for backward compatibility, plus `forms[]`) and the full `access[]` member list — " +
			"tens of thousands of tokens you rarely need. For a normal actor summary pass e.g. " +
			"filter=\"id,title,description,status,data,formId,formTitle,ownerId,createdAt,updatedAt,access\"; " +
			"omit `filter` only when you specifically need the form schema.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			fieldFilterParam("id,title,description,status,data,formId,formTitle,ownerId,createdAt,updatedAt,access"),
		},
	},
	{
		Name: "getActorByRef", Method: "GET", Path: "/actors/ref/{formId}/{ref}",
		Summary: "Look up an actor by its (formId, ref) external reference. Pass `filter` to fetch only the fields you need.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "External reference."},
			fieldFilterParam("id,title,ref,description,status,data,formId,formTitle,createdAt,updatedAt"),
		},
	},
	{
		Name: "updateActor", Method: "PUT", Path: "/actors/actor/{formId}/{actorId}",
		Summary: "Update an actor's fields/metadata. Only the provided fields change.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id the actor belongs to."},
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "data", In: InBody, Type: "object", Desc: "Field values to update. " + actorDataDesc + " Only the keys you include change; omit the rest."},
			{Name: "title", In: InBody, Type: "string", Desc: "New title."},
			{Name: "description", In: InBody, Type: "string", Desc: "New description."},
			{Name: "color", In: InBody, Type: "string", Desc: "New hex color."},
			{Name: "pictureObject", In: InBody, Type: "object", Desc: "Set/replace the node's custom image. " + pictureObjectDesc},
			{Name: "status", In: InBody, Type: "string", Desc: "New status value."},
			{Name: "appId", In: InBody, Type: "string", Desc: appIdDesc},
			{Name: "appSettings", In: InBody, Type: "object", Desc: appSettingsDesc},
			{Name: "hole", In: InBody, Type: "boolean", Desc: holeDesc},
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
		Summary: "Search actors across a workspace by title/text. Use before createActor to check whether an actor already exists. Pass `filter` to return only the fields you need — result lists can be large.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search query (title or fragment)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,title,ref,formId,status"),
		},
	},
	{
		Name: "searchLayerActors", Method: "GET", Path: "/layer_actors_filters/search/{actorId}/{query}",
		Summary: "Search actors placed on a specific layer by title/text. Pass `filter` to return only the fields you need — result lists can be large.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Layer actor UUID."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search query (title or fragment)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,title,ref,formId,status"),
		},
	},
	{
		Name: "filterActors", Method: "GET", Path: "/actors_filters/{formId}",
		Summary: "List/rank the actors of a form, optionally filtered by an account's balance. " +
			"Set linkedToActorId to restrict candidates to a single anchor actor's graph neighbours " +
			"(both directions along the hierarchy link) — that answers \"the actors related to X whose " +
			"account N balance is > / < some value\". Give accountNameId+currencyId to select the account, " +
			"amountFrom/amountTo for the balance threshold (amountFrom = balance >=, amountTo = balance <=), " +
			"and orderBy=balance to rank by it. Returned balances are real decimal values (e.g. 1600 = 1600 USD); " +
			"currency precision is display-only — do NOT divide by 10^precision. " +
			"This filters on CURRENT balance only; for turnover over a period read each actor's accounts with " +
			"getAccounts using from/to. " +
			"Pass `filter` to return only the fields you need — result lists can be large.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id whose actors are filtered/ranked."},
			{Name: "linkedToActorId", In: InQuery, Type: "string", Desc: "Anchor actor UUID. When set, only actors directly linked to this actor (parents or children along the hierarchy edge) are considered — use it for \"actors related to this one\". Omit for a form-wide listing."},
			{Name: "accountNameId", In: InQuery, Type: "string", Desc: "Account name id to read the balance from (see getAccountNames). Required to filter/rank by balance."},
			{Name: "currencyId", In: InQuery, Type: "number", Desc: "Currency id of the account (see getCurrencies). Pairs with accountNameId."},
			{Name: "incomeType", In: InQuery, Type: "string", Enum: []string{"credit", "debit"}, Desc: "Restrict the balance to one direction. Omit to net credit minus debit."},
			{Name: "accountType", In: InQuery, Type: "string", Enum: []string{"fact", "plan", "min", "max", "avg"}, Desc: "Account value type (fact (default) | plan | min | max | avg) — same enum as createAccount/getAccounts. The account's meaning comes from its name, not this type."},
			{Name: "amountFrom", In: InQuery, Type: "number", Desc: "Only actors whose account balance is >= this value (\"greater than\")."},
			{Name: "amountTo", In: InQuery, Type: "number", Desc: "Only actors whose account balance is <= this value (\"less than\")."},
			{Name: "orderBy", In: InQuery, Type: "string", Enum: []string{"updated_at", "created_at", "title", "owner", "balance", "reacted_at"}, Desc: "Sort field. Use balance to rank by the selected account's balance."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"DESC", "ASC"}, Desc: "Sort direction (default DESC)."},
			{Name: "search", In: InQuery, Type: "string", Desc: "Full-text search on actor title."},
			{Name: "q", In: InQuery, Type: "string", Desc: "Data-field filter expression on actor data (e.g. status=active; chatType=p2p for p2p-chat Events actors)."},
			{Name: "members", In: InQuery, Type: "string", Desc: "Filter to actors whose ACCESS RULES include all of these members. Comma-separated `userId:priv` pairs (priv = view|modify|remove|sign|ds|execute), e.g. \"4210:view,4310:view\". The match is containment (actor's access ⊇ the listed pairs). Use with q=chatType=p2p to find the existing p2p chat between specific users before creating a new one (chat participants are access-rule members, not data fields)."},
			{Name: "userId", In: InQuery, Type: "string", Desc: "Filter to actors a single user has access to (one grantee; see `members` for several)."},
			{Name: "status", In: InQuery, Type: "string", Desc: "Comma-separated status filter (e.g. verified,pending)."},
			{Name: "qFormId", In: InQuery, Type: "string", Desc: "Restrict or expand the candidate set across several forms."},
			{Name: "withStats", In: InQuery, Type: "boolean", Desc: "Include the total count of matching actors."},
			{Name: "withForm", In: InQuery, Type: "boolean", Desc: "Include each actor's form template in the response."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (0-200, default 20)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,title,ref,formId,status,data"),
		},
	},
	{
		Name: "getSystemActor", Method: "GET", Path: "/actors/system/{accId}/{objType}/{objId}",
		Summary: "Get-or-create the 1:1 system 'twin' actor that represents a user as a graph node. " +
			"Pass objType=user and objId=<userId> to get the actor that carries that user's accounts and is " +
			"their node on the graph. Use it for ACTOR-level work on a person: transactions/transfers, " +
			"reading/creating accounts, or placing the user on a layer. For SHARING/access instead, grant to " +
			"the userId directly (saveAccessRules) — do NOT share onto the twin. Find the userId first with " +
			"searchUsers/getUsers. objType is user only (other entity twins are not exposed here). " +
			"Returns a full actor (form schema + access) like getActor — always pass `filter` (e.g. just \"id,title,formId\" when you only need the twin's id).",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "objType", In: InPath, Type: "string", Required: true, Enum: []string{"user"}, Desc: "Entity kind whose twin actor to fetch — user only."},
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "The entity id — for objType=user, the user id (see searchUsers/getUsers)."},
			fieldFilterParam("id,title,formId,status,data"),
		},
	},
	{
		Name: "getCorezoidProcesses", Method: "GET", Path: "/actors/corezoid_processes/{actorId}",
		Summary: "List the Corezoid processes available to an actor — the processes shared to it via its access API keys. " +
			"Use to answer \"what functions/processes can this actor call?\" (the actor's callable integrations).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID whose connected Corezoid processes to list."},
		},
	},
}
