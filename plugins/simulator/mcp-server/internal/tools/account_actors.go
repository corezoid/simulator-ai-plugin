package tools

// accountActorOps — actors linked to accounts through the account_to_actors
// mechanism. This single backend table powers three features:
//
//   - Account TAGS: actors of the system form "Tags" linked to a workspace-level
//     (nameId, currencyId) pair — every account of that pair carries the tag.
//   - Account TRIGGERS: actors of the system form "AccountTriggers" linked to a
//     pair (all its accounts) or to ONE account (accountId) — fired on every
//     transaction that changes the account (amount / count value types).
//   - DATA triggers: AccountTriggers actors with valueType=data bound to a
//     specific actor's (or a whole form's) data field — fired when that field
//     changes on an actor update.
//
// Tag/trigger actors are ordinary actors: create them with createActor on the
// workspace's "Tags" / "AccountTriggers" system form (find the form ids via
// getForms(formTypes="system")), then link them with the ops below. Read a pair's
// current tags/triggers per account via getAccounts(withTags/withTriggers).
var accountActorOps = []Operation{
	{
		Name: "saveAccountActors", Method: "POST", Path: "/accounts/actors/{nameId}/{currencyId}",
		Summary: "REPLACE the actors linked to a workspace account pair (nameId + currencyId) — the write behind account TAGS and account TRIGGERS. " +
			"Existing links in scope are removed first, then `actors` are inserted: ALWAYS pass `formId` (the Tags or AccountTriggers system form id) to scope the replace to that kind only — without it every pair-level link (tags AND pair-level triggers alike) is wiped before inserting (only per-account bindings made with accountId survive). " +
			"Pair-level links apply to ALL accounts of the pair on every actor; pass `accountId` to bind triggers to ONE specific account instead. " +
			"To ADD a tag, first read the current set (getAccounts withTags=true) and resend it plus the new actor; `actors: []` unlinks everything in scope. Requires modify access on the pair.",
		Params: []Param{
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id of the pair."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id of the pair."},
			{Name: "formId", In: InQuery, Type: "number", Desc: "Form id of the LINKED actors — the Tags system form id when saving tags, the AccountTriggers form id when saving triggers (getForms(formTypes=\"system\") lists both). Strongly recommended: scopes the replace to links of that form; omitting it clears ALL of the pair's links first."},
			{Name: "accountId", In: InQuery, Type: "string", Desc: "Bind to ONE account (per-account triggers) instead of the whole pair. The replace then scopes to that account's links — still pass formId, or ALL of the account's linked actors are removed first."},
			{Name: "actors", In: InBody, Type: "array", Required: true, Desc: "Actor UUIDs to link — Tags-form actors for tags, AccountTriggers-form actors for triggers. The FULL new set (this is a replace, not an append); [] unlinks all in scope."},
		},
	},
	{
		Name: "getDataFieldActorsByActor", Method: "GET", Path: "/accounts/actors/actor/{actorId}",
		Summary: "List the trigger actors bound to one actor's data field (DATA triggers — AccountTriggers actors with valueType=data that fire when that field changes on the actor).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Target actor UUID whose data field the triggers watch."},
			{Name: "dataField", In: InQuery, Type: "string", Required: true, Desc: "The watched data field id (a field id from the actor's form)."},
			{Name: "triggerFormId", In: InQuery, Type: "number", Wire: "formId", Desc: "Filter the linked actors by their form id (usually the AccountTriggers system form)."},
		},
	},
	{
		Name: "saveDataFieldActorsByActor", Method: "POST", Path: "/accounts/actors/actor/{actorId}",
		Summary: "REPLACE the trigger actors bound to one actor's data field (DATA triggers). The triggers (AccountTriggers actors with valueType=data, an operator and a comparisonValue) fire whenever the field changes on that actor. " +
			"Existing (dataField, actor) bindings are removed first, then `actors` are linked; `actors: []` unbinds all.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Target actor UUID whose data field to watch."},
			{Name: "dataField", In: InQuery, Type: "string", Required: true, Desc: "The watched data field id (a field id from the actor's form)."},
			{Name: "triggerFormId", In: InQuery, Type: "number", Wire: "formId", Desc: "Scope the replace to linked actors of this form id (usually the AccountTriggers system form)."},
			{Name: "actors", In: InBody, Type: "array", Required: true, Desc: "AccountTriggers actor UUIDs to bind — the FULL new set (replace, not append); [] unbinds all."},
		},
	},
	{
		Name: "getDataFieldActorsByForm", Method: "GET", Path: "/accounts/actors/form/{formId}",
		Summary: "List the trigger actors bound to a form's data field (DATA triggers that watch the field on EVERY actor of the form).",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Target form id whose actors' data field the triggers watch."},
			{Name: "dataField", In: InQuery, Type: "string", Required: true, Desc: "The watched data field id (a field id from that form)."},
			{Name: "triggerFormId", In: InQuery, Type: "number", Wire: "formId", Desc: "Filter the linked actors by their form id (usually the AccountTriggers system form)."},
		},
	},
	{
		Name: "saveDataFieldActorsByForm", Method: "POST", Path: "/accounts/actors/form/{formId}",
		Summary: "REPLACE the trigger actors bound to a form's data field — the DATA triggers then fire when the field changes on ANY actor of the form. " +
			"Existing (dataField, form) bindings are removed first, then `actors` are linked; `actors: []` unbinds all.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Target form id whose actors' data field to watch."},
			{Name: "dataField", In: InQuery, Type: "string", Required: true, Desc: "The watched data field id (a field id from that form)."},
			{Name: "actors", In: InBody, Type: "array", Required: true, Desc: "AccountTriggers actor UUIDs to bind — the FULL new set (replace, not append); [] unbinds all."},
		},
	},
}
