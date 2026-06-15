package tools

import "strings"

// privsDesc documents the per-rule `privs` object shared by every save operation.
const privsDesc = "Privilege flags: " +
	"{view, modify, remove (all booleans, required), sign, ds, execute (optional booleans)}. " +
	"`view` is implied when any other privilege is set."

// ruleDesc documents one entry of the access-rule body array.
const ruleDesc = "Array of rule operations. Each item = " +
	"{action:\"create\"|\"update\"|\"delete\", data:{...}}. " +
	"`data` identifies the grantee by exactly one of userId | saId | groupId, " +
	"plus `privs` (" + privsDesc + ") and optional " +
	"`reactionOrders`:{sign,ds,execute} (positive integers ordering signature/DS/execute reactions). " +
	"For action=\"delete\", `data` needs only the grantee id."

// accessObjTypes are the ACCESS_OBJ_TYPE values accepted by the generic routes.
var accessObjTypes = []string{"actor", "form", "account", "templateActors", "formTemplate", "treeLayer"}

// accessRuleOps — manage who can view/modify/remove/sign/execute an object
// (actor, form/account template, or account). Mirrors the backend access_rules
// public API. Grants are applied asynchronously: save calls return a taskId.
var accessRuleOps = []Operation{
	{
		Name: "getAccessRules", Method: "GET", Path: "/access_rules/{objType}/{objId}",
		Summary: "List the users/groups that have access to an object, with their privileges. " +
			"Works for actors, forms, accounts and templates — pick objType accordingly.",
		Params: []Param{
			{Name: "objType", In: InPath, Type: "string", Required: true, Enum: accessObjTypes, Desc: "Object kind."},
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Object id (actor UUID, or numeric form/account id as a string)."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-200, default 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
		},
	},
	{
		Name: "saveAccessRules", Method: "POST", Path: "/access_rules/{objType}/{objId}",
		Summary: "Grant, change or revoke access on an object (actor, form, account, template). " +
			"Applied asynchronously — returns a taskId. Use action=create/update/delete per grantee.",
		Params: []Param{
			{Name: "objType", In: InPath, Type: "string", Required: true, Enum: accessObjTypes, Desc: "Object kind."},
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Object id (actor UUID, or numeric form/account id as a string)."},
			{Name: "rules", In: InBodyRoot, Type: "array", Required: true, Desc: ruleDesc},
			{Name: "recursive", In: InQuery, Type: "boolean", KeepFalse: true, Desc: "Cascade the change to child objects (default true). Set false to apply only to this object."},
			{Name: "notify", In: InQuery, Type: "boolean", KeepFalse: true, Desc: "Send access-change email/notifications (default true). Set false to grant/revoke quietly."},
			{Name: "filterIncorrect", In: InQuery, Type: "boolean", Desc: "Silently skip grantees not in the workspace instead of erroring."},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Optional edge type id used when sharing along a relation."},
		},
	},
	{
		Name: "getTemplateActorsAccess", Method: "GET", Path: "/access_rules/templateActors/{objId}",
		Summary: "List the users/groups with access to the actors created from a form template. " +
			"objId is the form-template id.",
		Params: []Param{
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Form-template id."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-200, default 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
		},
	},
	{
		Name: "saveTemplateActorsAccess", Method: "POST", Path: "/access_rules/templateActors/{objId}",
		Summary: "Grant/revoke access to the actors of a form template (objId = form-template id). " +
			"Applied asynchronously — returns a taskId.",
		Params: []Param{
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Form-template id."},
			{Name: "rules", In: InBodyRoot, Type: "array", Required: true, Desc: ruleDesc},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Optional edge type id used when sharing along a relation."},
			{Name: "notify", In: InQuery, Type: "boolean", KeepFalse: true, Desc: "Send access-change notifications (default true). Set false to apply quietly."},
			{Name: "filterIncorrect", In: InQuery, Type: "boolean", Desc: "Silently skip grantees not in the workspace instead of erroring."},
		},
	},
	{
		Name: "getTreeLayerAccess", Method: "GET", Path: "/access_rules/treeLayer/{objId}",
		Summary: "List the users/groups with access to the actors on a tree layer (objId = layer/root actor id).",
		Params: []Param{
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Tree layer / root actor id."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-200, default 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
		},
	},
	{
		Name: "saveTreeLayerAccess", Method: "POST", Path: "/access_rules/treeLayer/{objId}",
		Summary: "Grant/revoke access to all actors on a tree layer (objId = layer/root actor id). " +
			"Applied asynchronously — returns a taskId.",
		Params: []Param{
			{Name: "objId", In: InPath, Type: "string", Required: true, Desc: "Tree layer / root actor id."},
			{Name: "rules", In: InBodyRoot, Type: "array", Required: true, Desc: ruleDesc},
			{Name: "edgeTypeId", In: InQuery, Type: "number", Desc: "Optional edge type id used when sharing along a relation."},
			{Name: "notify", In: InQuery, Type: "boolean", KeepFalse: true, Desc: "Send access-change notifications (default true). Set false to apply quietly."},
			{Name: "filterIncorrect", In: InQuery, Type: "boolean", Desc: "Silently skip grantees not in the workspace instead of erroring."},
		},
	},
	{
		Name: "bulkSaveAccessRules", Method: "POST", Path: "/access_rules/bulk",
		Summary: "Apply access-rule changes to many objects in one call (up to 50). " +
			"Each item targets one object by (objType, objId) and carries its own rules.",
		Params: []Param{
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (max 50) of " +
				"{objId, objType (one of: " + strings.Join(accessObjTypes, ", ") + "), recursive?:boolean, " +
				"rules:[" + ruleDesc + "]}."},
		},
	},
	{
		Name: "bulkSaveAccountPairsAccessRules", Method: "POST", Path: "/access_rules/bulk/accounts_pairs/{accId}",
		Summary: "Grant/revoke access in bulk to every account whose name id matches a given prefix, " +
			"across all actors in the workspace. Use to share a whole account category at once.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (max 50) of " +
				"{objId (account-name id prefix — matches all accounts named `<objId>_*`), " +
				"recursive?:boolean, rules:[" + ruleDesc + "]}."},
		},
	},
	{
		Name: "requestAccess", Method: "POST", Path: "/request_access",
		Summary: "Request access to an object you currently CAN'T see (no view permission). Use when a read/" +
			"write tool fails with 403 / Access Denied on an actor (or form/account): this raises a request to " +
			"the object's owner(s) — it does NOT grant access itself, the owner approves or declines. Returns the " +
			"created invite + a request-access event id. Idempotent: a pending request for the same object is " +
			"reused, not duplicated. You don't need access to the object to call this.",
		Params: []Param{
			{Name: "objId", In: InBody, Type: "string", Required: true, Desc: "Id of the object to request access to (actor UUID, or numeric form/account id as a string)."},
			{Name: "objType", In: InBody, Type: "string", Required: true, Enum: accessObjTypes, Desc: "Object kind (usually actor)."},
			{Name: "modify", In: InBody, Type: "boolean", Desc: "Also request modify (edit) access, not just view (default false = view only)."},
			{Name: "sendEvent", In: InBody, Type: "boolean", Desc: "Raise a request-access event the owner can approve/decline (default true). Pass false to create the invite without an event."},
		},
	},
}
