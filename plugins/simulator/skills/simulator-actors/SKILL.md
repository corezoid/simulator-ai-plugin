---
name: simulator-actors
description: >
  Simulator.Company actor (record) specialist. Use when the user wants to create,
  update, read, search, or filter the *instances* of a form template (a.k.a.
  Account Template records) — i.e. data-bearing actors — and needs the `data`
  value protocol right. Activate when the user says "create an actor", "add a
  record", "create a record from this template", "fill in a form", "update actor
  data", "find actors where field = …", "filter actors by balance", "создай актор",
  "заполни шаблон", or "добавь запись". ALSO use this skill when the user asks what
  **processes / functions / Corezoid processes** an actor has or can call, or about an
  actor's **shared API keys / available functions** — call `getCorezoidProcesses`:
  "what processes/functions can this actor call", "actor's connected processes",
  "actor's API keys", "available functions of the actor", "які процеси/функції доступні
  актору", "процеси актора", "функції актора", "ключі доступу актора", "какие
  процессы/функции у актора", "процессы актора", "доступные функции актора". For
  graph/flowchart STRUCTURE (links, layers, FlowchartBlock diagrams) use `simulator-graph`;
  for designing the template itself use `simulator-forms`.
---

> **Curated tool names (v2 server):** `createActor`, `getActor`, `getActorByRef`,
> `updateActor`, `setActorStatus`, `deleteActor`, `searchActors`, `filterActors`,
> `searchLayerActors`, `getSystemActor`, `getCorezoidProcesses`. Call them by these exact names.
>
> **`getCorezoidProcesses(actorId)`** is THE tool for any question about an actor's
> **processes / functions / available API integrations** — it returns the Corezoid processes
> shared to the actor via its access API keys (what that actor can call). If the user asks
> "what functions/processes does this actor have", "what can this actor call", or about the
> actor's shared API keys, call `getCorezoidProcesses` — do not guess or use a graph-traversal tool.
>
> **`getSystemActor`** resolves the system "twin" actor of a workspace entity — pass
> `objType="user"`, `objId="<userId>"` to get the actor representing a user (so you can attach
> accounts to it or move value between users). Find the `userId` first with `searchUsers` /
> `getUsers`. Money movement between users then goes through their twin actors' accounts via
> **transfers** — see `simulator-finance`.

# Simulator.Company Actor (Record) Specialist

You create and manage **actors** — the *instances* of a form template (Account
Template). An actor's field values live in its `data` object. Getting the `data`
shape right is the whole job, so this skill exists to make that mechanical.

> **Relationship to the other skills**
> - **`simulator-forms`** designs the *template* (`sections`/fields). Read it / a form
>   first to learn the field `id`s.
> - **`simulator-actors`** (this skill) creates/edits the *records* of that template.
> - **`simulator-graph`** handles graph *structure* — links, layers, FlowchartBlock
>   diagrams. If the user wants to wire actors together or place them on a layer, defer there.
> - **`simulator-finance`** handles accounts/transactions on actors.

## Workspace Context Check (MANDATORY FIRST STEP)

Verify `accId` is known before any tool call. If not provided, ask:

> Ask the user — **in their own language** (English, Ukrainian, or Russian) — which workspace to work in, i.e. for the Workspace ID (`accId`).

`createActor` can also resolve a `formName` to its id, but only with a workspace set.

---

## The golden rule: `data` is keyed by field `id`

An actor's `data` is keyed by each form field's **`id`** (`item_<digits>`) — **not** by
the field `title` and **not** by its secondary `key`. So the first step of *every* actor
create/update is:

1. **Resolve the form.** `getForm(formId)` (or `searchForms` then `getForm`). Pass
   `filter="id,title,sections"` to keep it small.
2. **Map each field you want to set** → its `id` and `class`.
3. **Build `data`** using the per-class value shapes below.

### Value shape by field class

| Form field class / source | Value in `data` |
|---|---|
| `edit` text / password / email / phone | string — `"dsdf@fsdf.com"` |
| `edit` int / float | number — `1` |
| `check` | boolean — `true` |
| `radio` | the selected option's `value` (string) — `"o2"` |
| `select` (static `options`) | array of chosen option object(s) — `[{"color":"#863434","title":"one","value":"s1"}]` |
| `select` → `layer`/`actorFilter`/`actorsBag`/`actors`/`formFilter` | `[{"type":"actor","title":…,"value":"<actor UUID>"}]` (formFilter = the actors of a given form) |
| `select` → `forms` | `[{"type":"form","title":…,"value":<form id number>}]` |
| `select` → `currencies` | `[{"type":"currency","title":…,"value":<currency id number>}]` |
| `select` → `accountNames` | `[{"type":"accountName","title":…,"value":"<account-name UUID>"}]` |
| `select` → `workspaceMembers` | `[{"type":"workspaceMembers","title":…,"value":<user id number>}]` |
| `select` → `api`/`corezoidSyncApi` | array of synced options (`[]` until synced) |
| `multiSelect` | array — `[{"title":"one","value":"…"},{"title":"two","value":"…"}]` |
| `calendar` | `{"startDate":…,"endDate":…,"timeZoneOffset":-180,"sendInvite":false}` (unix **seconds**) |
| `upload` | array of file refs (`[]` when empty) |
| `label` / `button` / `image` | **no entry** — display-only |

> The `type` discriminator only appears on **dynamic** `select` values. Static
> `select`/`multiSelect` values carry just `{title,value,color?}`. For a dynamic
> `select`, the `value` must be a real referenced id (actor UUID / currency id /
> account-name UUID / user id) — resolve it first (e.g. `getCurrencies`,
> `getAccountNames`, `searchActors`) rather than guessing.

Full reference: `$CLAUDE_PLUGIN_ROOT/docs/entities/actors.md` and `…/forms.md`.

### Multiform actors (`__form__<formId>:<itemId>`)

An actor can instantiate **several forms at once** (a *multiform* actor; the linked form
tree is called a **UAT**). Its `data` then carries fields from more than one form, namespaced
by a key prefix:

- Fields of the actor's **root** `formId` (the one you pass to `createActor`/`updateActor`) →
  the **plain** field `id`, e.g. `"name"`.
- Fields owned by **another** form → **`__form__<thatFormId>:<itemId>`**.

```json
// data for an actor created under its root form, with one field from form 16951
{
  "name": "Kulo Oleksandr",
  "__form__16951:position": "Software engineer"
}
```

The prefix changes only the **key** — the value still follows the per-class shapes above.
Notes:
- To learn `<thatFormId>`'s field ids, `getForm(<thatFormId>)` just like the root form.
- **Discover the tree** with `getFormsTree(accId, formId)` / `getLinkedForms(typeLink, formId)`
  (see `simulator-forms`) to find which forms a multiform actor spans before writing its `data`.
- Attaching the *set* of forms to an actor (`PUT /actors/actor_forms`) is still **not** a curated
  tool — but you can already write/read multiform `data` via `createActor`/`updateActor`/`getActor`.

#### Creating under the tree when asked for a CHILD (leaf) form

In a UAT workspace, an actor can only be created under the **root** form of the tree — not
under an arbitrary leaf. So when the user says *"create an actor for form X"* and
`getForm(X)` returns a **non-empty `parentId`** (X is a nested/child form), do **not** call
`createActor(formId=X, …)` — the backend rejects it with **`400: Form <id> is not UAT`**.

Instead:

1. **Walk up `parentId`** from X to the root form of the UAT tree (`getForm` repeatedly until
   `parentId` is empty).
2. **Call `createActor` with `formId` = the root.**
3. Put the **root's own fields** under their plain id (`"name"`), and the **requested child
   form's fields** under the `__form__<childFormId>:<itemId>` prefix.

```json
// Request: "create an actor for form Employee (16951)", where Employee.parentId = People (16950)
// → create under the ROOT People (16950), not under Employee:
createActor(formId=16950, title="Olena Kovalenko", data={
  "name": "Olena Kovalenko",                            // People (root) field
  "__form__16951:position": "Senior Backend Engineer"   // Employee (child) field
})
```

> **Do NOT** flip the child form to `uat` status via `setFormStatus` to "work around" the
> error — that mutates the shared template for everyone. Use the **root** form as the
> `formId` instead.

---

## Create an actor

```
createActor(
  formId=42,                       # or formName="Car" (resolved via the workspace)
  title="Toyota Camry 2023",
  color="#409547",                 # optional
  ref="car-toyota-camry-2023",     # optional, unique per form
  contextLayerId="<layer UUID>",   # optional — place it on a layer
  data={
    "item_make":  "Toyota",
    "item_model": "Camry",
    "item_year":  2023,
    "item_active": true,
    "item_cond":  [ {"title":"excellent","value":"c1"} ],
    "item_owner": [ {"type":"workspaceMembers","title":"Alex Kulo","value":1} ]
  })
```

- Provide **`formId`** (number) or **`formName`** (string, resolved to its id).
- Omit fields you are not setting; never invent `item_*` keys not present in the form.
- Display-only fields (`label`/`button`/`image`) get no `data` entry.

## Read an actor

```
getActor(actorId="<UUID>", filter="id,title,data")           # by UUID
getActorByRef(formId=42, ref="car-…", filter="id,title,data") # by (form, ref)
```

Use `filter` to fetch only the fields you need — actor `data` can be large.

## Update an actor

```
updateActor(
  formId=42, actorId="<UUID>",
  data={ "item_year": 2024, "item_active": false })   # only these keys change
```

`updateActor` is a **partial** update of `data` — keys you include are replaced, the rest
are untouched. You can also set `title`/`description`/`color`/`status`. To clear a
multi-value field, send `[]`.

## Status & delete

```
setActorStatus(actorId="<UUID>", status="verified")
deleteActor(actorId="<UUID>")     # irreversible — confirm with the user first
```

---

## Search & filter

### `searchActors` — free-text across the workspace
```
searchActors(accId="ws_xxx", query="camry", limit=20, filter="id,title,formId")
```
Best for "find the actor named …". Run before `createActor` to avoid duplicates.

### `filterActors` — list/rank a form's actors, optionally by an account balance
```
# All actors of a form, newest first
filterActors(formId=42, orderBy="updated_at", withStats=true)

# Data-field filter on the actor data
filterActors(formId=42, q="status=active", status="verified,pending")

# Rank by an account's CURRENT balance, scoped to one anchor actor's neighbours
filterActors(
  formId=42,
  linkedToActorId="<anchor UUID>",   # only actors linked to this one (both directions)
  accountNameId="<account-name UUID>",
  currencyId=9,
  amountFrom=1000,                    # balance >= 1000
  orderBy="balance", orderValue="DESC",
  withStats=true)
```

- `q` filters on actor **data** fields; `search` does full-text on the title; `status`
  filters by status.
- Balance filtering is **current balance only**. For turnover over a period, read each
  actor's accounts with `getAccounts(actorId, from, to)` (a `simulator-finance` task).
- Returned balances are real decimal values — do **not** divide by `10^precision`.

### `searchLayerActors` — search within one layer
```
searchLayerActors(actorId="<layer UUID>", query="camry")
```

---

## End-to-end example

```
# 1. Resolve the template and its field ids
searchForms(accId="ws_xxx", q="Car")          → formId 42
getForm(formId=42, filter="id,title,sections") → field ids: item_make, item_year, …

# 2. Avoid a duplicate
searchActors(accId="ws_xxx", query="Camry 2023")

# 3. Create the record
createActor(formId=42, title="Toyota Camry 2023",
  data={ "item_make":"Toyota", "item_model":"Camry", "item_year":2023 })

# 4. (optional) attach an account — see simulator-finance
createAccount(actorId="<new UUID>", nameId="<aname>", currencyId=1, accountType="fact")
```

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/actors.md` | Actor `data` protocol + per-class value shapes |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/forms.md` | Field-class catalogue / dynamic select sources |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/search.md` | Search & filter internals |

## Tips

- Always `getForm` first — `data` keys are the form's field `id`s, not titles.
- If a form has a `parentId`, it's a **leaf** of a UAT tree — create the actor under the **root** form and put the leaf's fields under `__form__<childId>:<field>`. Symptom of using the wrong root: `400: Form <id> is not UAT`.
- Dynamic-`select` values reference real ids; resolve them, don't guess.
- `createActor` accepts `formName` (resolved in the active workspace) when you don't have the id.
- `updateActor` is partial; `deleteActor` is irreversible — confirm first.
- For wiring actors into a graph (links/layers/flowcharts) use `simulator-graph`.
- `getCorezoidProcesses(actorId)` lists the Corezoid processes shared to an actor via its access API keys — i.e. the functions/processes that actor can call.
