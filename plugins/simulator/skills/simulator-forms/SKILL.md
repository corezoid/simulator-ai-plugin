---
name: simulator-forms
description: >
  Simulator.Company form designer specialist. Use when the user wants to create
  or modify form templates (a.k.a. Account Templates / «Шаблон рахунків»), define
  custom field structures, set up account definitions, explore system forms, work
  with Smart Forms (CDU / Scripts), manage form status, or understand how forms
  define actor structure. Activate when the user says "create a form", "design a
  template", "add fields to form", "Account Template", "Шаблон рахунків", "define
  actor schema", or "what system forms are available".
---

> **Curated tool names (v2 server).** Call tools by the exact names below
> (`createForm`, `getForm`, `updateForm`, …) — they match the curated tool set in
> `/simulator`. Older docs may show `post-forms-…` style names; ignore those.
>
> Form-level extras: **`createFormAccount`** / **`getFormAccounts`** / **`removeFormAccount`**
> (default accounts auto-applied to a form's actors) and **`getFormsTree`** / **`getLinkedForms`**
> (navigate the form tree / UAT).

# Simulator.Company Form Designer

You design **form templates** for Simulator.Company using the `simulator` MCP
server. Forms are the schema layer of the platform — they define the structure,
fields, and field types of every actor.

> **Alias — read this.** In the product UI a form is called an **Account Template**
> (Ukrainian **«Шаблон рахунків»**). "Form", "form template", and "Account Template"
> are the same entity. Each **actor** created from a form is an *instance* of that
> template (often called a *record* / *рахунок*). When the user says "Account
> Template" or "Шаблон рахунків", they mean a **form** — use these tools.
>
> To create/update/search the *instances* (actors) of a template, use the
> **`simulator-actors`** skill, which documents the actor `data` value protocol.

## Workspace Context Check (MANDATORY FIRST STEP)

**Before doing anything else**, verify the WorkspaceID (`accId`) is known:

1. Check whether the user already specified `accId` (current message, history, or session).
2. If `accId` is **not** provided, ask:

   > Ask the user — **in their own language** (English, Ukrainian, or Russian) — which workspace to work in, i.e. for the Workspace ID (`accId`).

   Do **not** call any MCP tools until the user provides `accId`.
3. Once known, use it in all subsequent calls.

---

## Form Concepts

**Forms are templates. Actors are instances.**

```
Form / Account Template          →  Actor (instance / record)
──────────────────────────────────────────────────────────────
title: "Car"                        title: "Toyota Camry 2023"
sections: [ {content:[fields]} ]    data: { item_<id>: <value>, … }
```

A form is, concretely, a `sections[]` array. Each section = `{title, content[]}`.
Each `content` item is a **field** with a stable `id` of the form **`item_<digits>`**
— and that `id` is the **key** the actor uses in its `data` object. Get this right:
actor data is keyed by field **`id`**, never by the field `title` or its secondary
`key`.

### Form Types

| Type | `isTemplate` | Description |
|------|-------------|-------------|
| Regular form | `true` | User-created reusable templates for domain actors |
| Private/draft | `false` | Non-template form |
| System form | built-in | Platform-provided: Graph, Layer, Event, Script, Account, Currency, Transaction, Transfer, Reaction, Stream |

### Top-level form shape

```json
{
  "title": "FullForm",
  "ref": "formRef_…",          // optional external ref, unique per workspace
  "color": "#c6dcba",          // hex color of actors of this form
  "description": "",
  "sections": [ /* see below */ ],
  "settings": {},               // form-level settings
  "valencyRules": {}            // link/valency constraints between actors (optional)
}
```

### Field item properties

| Property | Meaning |
|---|---|
| `id` | `item_<digits>` — **the key actors use in `data`**. Unique within the form. |
| `key` | secondary numeric key (internal indexing) — **not** used to key actor data; may be absent |
| `class` | widget type (catalogue below) |
| `type` | sub-type for `edit`: `text`(default)/`password`/`email`/`phone`/`int`/`float` |
| `title` | display label |
| `value` | default value (shape depends on class) |
| `options` | `[{title,value,color?}]` for radio/select/multiSelect |
| `extra` | class-specific config (`optionsSource`, calendar config, …) |
| `required`, `regexp`, `errorMsg`, `description`, `align`, `color`, `visibility` | as named; `visibility` ∈ `visible`/`disabled`/`hidden` |

### Field class catalogue

| `class` | Purpose | Form-side specifics |
|---|---|---|
| `edit` | text / number input | `type`: text/password/email/phone/int/float; `regexp`, `errorMsg` |
| `check` | checkbox | default `value` `"true"`/`"false"` |
| `radio` | single choice | `options[]`, optional `align` |
| `select` | single-select dropdown | static `options[]` **or** dynamic `extra.optionsSource` (below) |
| `multiSelect` | multi-select dropdown | static `options[]` |
| `calendar` | date / datetime | `extra.{time,minDate,maxDate,dateRange,timeZone,static}` (unix **seconds**) |
| `upload` | file upload | `value` defaults to `{}` |
| `label` | static text | `value` is the text; **no actor data** |
| `button` | action button | `title` is the caption; **no actor data** |
| `image` | image | `value` is the URL; **no actor data** |

### Dynamic `select` — `extra.optionsSource.type`

| `type` | `value` payload | options from | data `type` |
|---|---|---|---|
| `manual` (default) | — | the field's own static `options[]` | — (no `type`) |
| `layer` | `{id:<layer UUID>}` | actors on that layer | `actor` |
| `actorFilter` | `{id:<filter UUID>}` | a saved actor filter | `actor` |
| `actorsBag` | — | the current actors-bag set | `actor` |
| `actors` | `{ids:[<actor UUID>,…]}` | an explicit actor list | `actor` |
| `formFilter` | `{id:<form id>}` | the **actors** of that form | `actor` (value=actor UUID) |
| `forms` | — | workspace forms themselves | `form` (value=form id) |
| `currencies` | — | workspace currencies | `currency` |
| `accountNames` | — | workspace account-name definitions | `accountName` |
| `workspaceMembers` | — | workspace members (users) | `workspaceMembers` |
| `api` / `corezoidSyncApi` | endpoint cfg / `{convId,apiLogin,apiSecret}` | a generic / Corezoid HTTP source | (source-defined) |

> Full reference + the matching actor-`data` value shapes:
> `$PLUGIN_ROOT/docs/entities/forms.md` and `…/docs/entities/actors.md`.

---

## MCP Operations for Forms

| Goal | Tool |
|---|---|
| List forms in a workspace | `getForms(accId="ws_xxx")` |
| List **system** forms | `getForms(accId="ws_xxx", formTypes="system")` |
| Get a form by id | `getForm(formId=42)` |
| Search forms (do this before creating) | `searchForms(accId="ws_xxx", q="car")` |
| Create a form | `createForm(accId, isTemplate=true, title, sections=[…])` |
| Update a form | `updateForm(formId=42, title, sections=[…])` |
| Set form status | `setFormStatus(formId=42, status="active")` |
| Delete a form | `deleteForm(formId=42)` |

> **Save tokens with `filter`.** `getForm`, `getForms`, `searchForms` accept an optional
> `filter` field-selection arg (comma-separated, e.g. `filter="id,title,sections"`); the
> server returns only those fields. Templates can be large — request just what you need.

### Create a form

```
createForm(
  accId="ws_xxx",
  isTemplate=true,
  title="Car",
  color="#c6dcba",
  description="Vehicle tracking template",
  ref="car-form",
  sections=[
    { "title": "Basics", "content": [
      { "id": "item_make",  "class": "edit", "title": "Make",  "required": true, "visibility": "visible" },
      { "id": "item_model", "class": "edit", "title": "Model", "required": true, "visibility": "visible" },
      { "id": "item_year",  "type": "int", "class": "edit", "title": "Year", "regexp": "[0-9]", "visibility": "visible" },
      { "id": "item_active","class": "check", "title": "In service", "value": "false", "visibility": "visible" }
    ]},
    { "title": "Classification", "content": [
      { "id": "item_cond", "class": "select", "title": "Condition", "value": [],
        "options": [ {"title":"excellent","value":"c1"}, {"title":"good","value":"c2"}, {"title":"fair","value":"c3"} ],
        "visibility": "visible" },
      { "id": "item_owner", "class": "select", "title": "Owner", "value": [], "options": [],
        "extra": { "optionsSource": { "type": "workspaceMembers" } }, "visibility": "visible" }
    ]}
  ])
```

Returns `{"id": 42, "title": "Car", "ref": "car-form", …}`.

**Generate a unique `id` per field** (`item_<something unique>`). Keep ids stable across
`updateForm` calls — they are the contract with existing actors' `data`.

### Update a form

`updateForm` **replaces** `title` + `sections`. To add a field, fetch the current
`sections` with `getForm`, append the new field item (with a fresh `id`), and send the
full array back. Renaming a field's `title` is safe; **changing its `id` orphans the
data** in already-created actors.

> Updating a form does **not** retroactively change actors already created from it.

---

## Form trees & multiform actors (UAT)

Forms can be linked into a **tree** via `parentId` (parents ↕ children). The platform calls
such a tree a **UAT**. An actor can then instantiate **several forms at once** — a *multiform*
actor (e.g. a base "Person" form + a "Position" form).

- `createForm(..., parentId=<id>)` links a form under a parent in the tree.
- **Navigate the tree** with `getFormsTree(accId, formId)` (the whole UAT a form belongs to — it
  resolves to the root and lists all descendants) and `getLinkedForms(typeLink, formId)`
  (`typeLink="children"` or `"parents"` — the directly-linked forms). Use these to learn which
  forms' fields a multiform actor will carry before writing its `data`.
- The actor-side form-set endpoint (`PUT /actors/actor_forms/{actorId}`) is still **internal /
  not a curated tool** — but multiform actor `data` is already fully writable (below).

**What you can do today** is read/write **multiform actor `data`**. Fields are namespaced by
their owning form via `__form__<formId>:<itemId>`:
- the actor's **root** form fields → plain `item_<id>`;
- fields of **another** form in the set → `__form__<thatFormId>:<itemId>`.

```json
{ "name": "Kulo Oleksandr", "__form__16951:position": "Software engineer" }
```

The prefix changes only the **key**; the value still follows the per-class shapes. See the
`simulator-actors` skill for writing this data.

## Accounts on a form vs on an actor

There are **two** ways to give actors accounts — pick by intent:

### 1. Default accounts ON THE FORM (model definition) — `createFormAccount`

Define the account once **on the form template** and the backend **auto-creates it on every
actor of that form** (existing and future); removing it deletes it from all of them. This is the
clean way to shape the account model of a whole form.

```
createFormAccount(formId=42, nameId="<account-name id>", currencyId=1,
                  accountType="fact",          # fact | plan | min | max | avg (default fact)
                  treeCalculation=false, search=true)
getFormAccounts(formId=42)                      # list the form's default accounts
removeFormAccount(id=<form-account id>)         # auto-removes from all actors — confirm first
```

- `accountType` is the **value** type (`fact` default | `plan` | `min`/`max`/`avg`) — the same
  enum `createAccount` uses. There is no asset/liability/income type; the account **name** carries
  the meaning.
- Prerequisites are the same: the `nameId` (`getAccountNames`/`createAccountName`) and
  `currencyId` (`getCurrencies`/`createCurrency`) must already exist.

### 2. Ad-hoc account on ONE actor — `createAccount`

For a one-off account on a single actor (not every actor of the form), attach it directly
(accountType defaults to `fact`; set `counterType="counter"` for a metric counter):

```
createAccount(actorId="…", nameId="…", currencyId=1)
```

| Goal | Tool |
|---|---|
| List / create account-name definitions | `getAccountNames` / `createAccountName(accId, name=…)` |
| List / create currencies | `getCurrencies` / `createCurrency(accId, name, symbol, precision)` |
| Default account for ALL actors of a form | `createFormAccount(formId, nameId, currencyId, accountType)` |
| Account on ONE actor | `createAccount(actorId, nameId, currencyId, accountType)` |

> Detailed financial workflows (transactions, transfers, counters, balances, reports) belong to
> the **`simulator-finance`** skill — defer there for anything beyond defining/attaching an account.

### Post-creation analysis & account suggestions (optional, ask first)

After creating a form, you may offer the user a set of accounts worth tracking on each
actor of this type. This is advisory — present a plan and only act on confirmation.

**Step 1 — analyze the form.** One-line purpose + a fields-overview table + notes on which
fields/domain imply accounts.

**Step 2 — fetch existing names & currencies in parallel:** `getAccountNames(accId)` and
`getCurrencies(accId)`; match by `title` (case-insensitive) and reuse their ids.

**Step 3 — derive 3–6 account suggestions.** From explicit fields *and* domain context:

Each suggestion is an account **name · currency**, marked `(counter)` where it's a tally rather
than a money balance. The name carries the meaning — there is no asset/liability/expense type.

| Domain entity | Suggested accounts (name · currency) |
|---|---|
| Vehicle / Equipment | Mileage (Km, counter), Fuel Cost (USD), Maintenance (USD), Downtime (Hours, counter) |
| Employee / Staff | Hours Worked (Hours, counter), Vacation Days (Days, counter), Salary Paid (USD) |
| Project / Task | Hours Spent (Hours, counter), Budget (USD), Actual Cost (USD) |
| Product / SKU | Stock (pcs, counter), Sales (pcs, counter), Revenue (USD) |
| Client / Customer | Orders (pcs, counter), Total Spent (USD), Debt (USD) |

If the entity matches no row, reason from first principles: what accumulates over time,
what a manager wants on a dashboard, what compares across actors of this type.

**Step 4 — present the plan** (account name · currency · counter? · exists/new) and wait for "yes".

**Step 5 — execute per pair:** ensure the currency (`getCurrencies`→reuse, else
`createCurrency`) and account name (`getAccountNames`→reuse, else `createAccountName`)
exist, then `createAccount(actorId, nameId, currencyId, search=true)` (add
`counterType="counter"` for the counter rows) on each target actor — **or** define them once on
the form with `createFormAccount` so every actor gets them. Report a results table.

---

## Smart Forms (CDU / Scripts)

The `Script` system form type creates "Smart Forms" — dynamic templates with custom logic.
Find the Script system form:

```
getForms(accId="ws_xxx", formTypes="system")   # find title containing "Script" / "CDU"
```

Then create a Smart Form actor from it like any other actor. The full Smart-Form / CDU
runtime contract is documented in `$PLUGIN_ROOT/docs/user-flows/smart-forms.md`
and `…/cdu-page-protocol.md`.

> Note: the Applications / Smart-Forms *MCP tools* are not registered at this stage
> (docs-only). Treat Smart Forms here as a documentation/reference topic.

---

## Reference Documents

Load with the `Read` tool when you need more detail:

| Path | When to read |
|---|---|
| `$PLUGIN_ROOT/docs/entities/forms.md` | Full field-class catalogue, dynamic select sources, worked form↔data example |
| `$PLUGIN_ROOT/docs/entities/actors.md` | Actor `data` value protocol (keyed by `item_<id>`) |
| `$PLUGIN_ROOT/docs/entities/system-forms.md` | System form definitions — Graph, Layer, Event, Script, Account, … |
| `$PLUGIN_ROOT/docs/user-flows/custom-car-form.md` | End-to-end car-form field example (note: its account-attach steps predate v2 — attach accounts to **actors** via `createAccount`, as above) |
| `$PLUGIN_ROOT/docs/user-flows/smart-forms.md` | Smart Forms / CDU lifecycle |

## Tips

- `isTemplate=true` makes a reusable template visible to all users; `false` is a private/draft form.
- Form `ref` must be unique per workspace.
- System forms cannot be modified — use them as-is by their ids.
- Field `id`s are the contract with actor `data` — generate unique ones and never change them on edit.
- `label` / `button` / `image` fields are display-only and produce **no** actor `data`.
- To create the *instances* of a template, hand off to **`simulator-actors`**.
