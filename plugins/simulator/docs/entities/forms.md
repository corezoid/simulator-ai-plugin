# Forms

Forms in the Simulator.Company platform define the structure and behavior of actors, providing reusable templates for data collection and validation.

> **Alias.** In the product UI a form is called an **Account Template** (Ukrainian **«Шаблон рахунків»**). "Form", "form template", and "Account Template" all refer to this same entity. Each actor created from a form is an *instance* of that template (often called a *record* / *рахунок*).

## Overview

Forms (also known as Smart Forms or CDU) serve as templates that define the structure, fields, and validation rules for actors. They enable consistent data collection and provide a foundation for creating structured business processes.

A form is, concretely, a `sections[]` array. Each section has a `title` and an ordered `content[]` of **field items**. Every field item carries a stable `id` of the form `item_<digits>` — **this id is the key actors use in their `data` object** (see [actors.md](actors.md)). The field's `class` decides the widget and the value shape.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the form |
| acc_id | String | Workspace ID the form belongs to |
| user_id | Integer | ID of the user who created the form |
| title | Text | Display title of the form |
| description | Text | Detailed description of the form's purpose |
| sections | JSON | Form sections containing fields and their properties |
| color | String | Color associated with the form (hex code) |
| picture | Text | URL or path to the form's image |
| tags | JSON Array | Tags for categorization |
| settings | JSON | Form-specific settings |
| parent_id | Integer | ID of the parent form (for form inheritance) |
| status | Enum | Form status (active, removed, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Form Sections and Fields

Forms are organized into sections, each containing fields with specific properties:

### Section Structure

| Property | Type | Description |
|----------|------|-------------|
| title | String | Section title |
| content | Array | Array of field definitions |

### Field Properties

Each field item in a section's `content[]` may carry these properties:

| Property | Type | Description |
|----------|------|-------------|
| `id` | String | **Field identifier — `item_<digits>`. This is the key used in the actor `data` object.** Must be unique within the form. |
| `key` | String | Secondary numeric key used internally (search indexing / Smart-Form binding). NOT used to key actor data. May be absent on some fields. |
| `class` | String | Widget type — see the catalogue below (`edit`, `check`, `radio`, `select`, `multiSelect`, `calendar`, `upload`, `label`, `button`, `image`). |
| `type` | String | Sub-type for `edit` fields: `text` (default), `password`, `email`, `phone`, `int`, `float`. |
| `title` | String | Display label for the field. |
| `value` | Mixed | Default value (shape depends on the class). |
| `options` | Array | Available options for `radio`/`select`/`multiSelect` — each `{title, value, color?}`. |
| `extra` | Object | Class-specific config — `optionsSource` for dynamic `select`, calendar config for `calendar`, `{multiline, rows}` for multiline `edit`. |
| `required` | Boolean | Whether the field is required. |
| `visibility` | String | `visible`, `disabled`, or `hidden`. |
| `regexp` | String | Validation regular expression (`edit`). |
| `errorMsg` | String | Custom error message shown on validation failure. |
| `description` | String | Helper text under the field. |
| `align` | String | Layout alignment (`horizontal`, `center`, …). |
| `color` | String | Field/option color (hex). |
| `idNotChanged` | Boolean | Internal flag: the `id` is stable and not regenerated on edit. |

### Field Class Catalogue

The `class` property selects the widget. The right-hand column is the value shape the
matching **actor `data`** entry takes (keyed by the field `id`); see [actors.md](actors.md).

| `class` | Purpose | Form-side specifics | Actor `data` value |
|---|---|---|---|
| `edit` | Text/number input | `type`: text / `password` / `email` / `phone` / `int` / `float`; `regexp`, `errorMsg` | `string` (text/password/email/phone), `number` (int/float) |
| `check` | Checkbox | default `value` is `"true"`/`"false"` | `boolean` |
| `radio` | Single choice (radios) | `options[]` of `{title,value}`, optional `align` | the selected option's `value` (string) |
| `select` | Single-select dropdown | static `options[]` **or** dynamic `extra.optionsSource` (see below) | array of selected option object(s) — see below |
| `multiSelect` | Multi-select dropdown | static `options[]` of `{title,value}` | array `[{title,value}, …]` |
| `calendar` | Date / datetime | `extra.{time,minDate,maxDate,dateRange,timeZone,static}` (unix **seconds**) | `{startDate,endDate,timeZoneOffset,sendInvite}` (unix seconds) |
| `upload` | File upload | `value` defaults to `{}` | array of file refs (`[]` when empty) |
| `label` | Static text | `value` is the text shown, optional `align` | **none** (display-only) |
| `button` | Action button | `title` is the caption | **none** (display-only) |
| `image` | Image display | `value` is the image URL | **none** (display-only; URL lives in the form) |

> **Display-only classes** (`label`, `button`, `image`) never appear in actor `data`. Fields with `visibility: "disabled"` that the user does not fill are also typically omitted from `data`.

### Dynamic `select` — `extra.optionsSource`

A `select` field can pull its options from the platform instead of a static `options[]`.
`extra.optionsSource.type` (constant `FORM_OPT_SOURCE`) is one of:

| `type` | `value` payload | Options come from | Actor `data` entry shape |
|---|---|---|---|
| `manual` | — | the field's own static `options[]` (this is the default when there is no `optionsSource`) | `[{title, value, color?}]` (no `type`) |
| `layer` | `{id: <layer actor UUID>}` | actors placed on that layer | `[{type:"actor", title, value:<actor UUID>}]` |
| `actorFilter` | `{id: <filter UUID>}` | a saved actor filter | `[{type:"actor", title, value:<actor UUID>}]` |
| `actorsBag` | — | the current "actors bag" context set | `[{type:"actor", title, value:<actor UUID>}]` |
| `actors` | `{ids: [<actor UUID>, …]}` | an explicit list of actors | `[{type:"actor", title, value:<actor UUID>}]` |
| `formFilter` | `{id: <form id>}` | the **actors** of that form (an actor source scoped by form) | `[{type:"actor", title, value:<actor UUID>}]` |
| `forms` | — | workspace forms (Account Templates) themselves | `[{type:"form", title, value:<form id number>}]` |
| `currencies` | — | workspace currencies | `[{type:"currency", title, value:<currency id number>}]` |
| `accountNames` | — | workspace account-name definitions | `[{type:"accountName", title, value:<account-name UUID>}]` |
| `workspaceMembers` | — | workspace members (users) | `[{type:"workspaceMembers", title, value:<user id number>}]` |
| `api` | `{…endpoint config}` | a generic external HTTP source | `[{…option objects from the API}]` |
| `corezoidSyncApi` | `{convId, apiLogin, apiSecret}` | options synced from a Corezoid process | `[{…synced option objects}]` (`[]` until synced) |

The `type` discriminator (`FORM_ITEM_TYPES`) is one of `actor`, `currency`, `accountName`,
`workspaceMembers`, or `form`. **`manual`/static** `select`/`multiSelect` values carry
**no** `type` — they are just the chosen `{title, value, color?}` option object(s). Dynamic
`select` values always carry a `type` so the platform can resolve the referenced entity.

## API Endpoints

For detailed API documentation on forms, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Forms API Documentation](https://doc.simulator.company/#tag/forms)

The API provides endpoints for:

- Getting all forms in a workspace
- Retrieving specific form details
- Creating new forms
- Updating existing forms
- Deleting forms
- Managing form inheritance and relationships

All API requests require appropriate OAuth2 scopes (`control.events:forms.readonly` for read operations and `control.events:forms.management` for write operations).

### Clone Form

```
POST /papi/1.0/forms/{formId}/clone
```

Scopes: `control.events:forms.management`

Creates a copy of an existing form.

## Form Inheritance

Forms support inheritance through the `parent_id` property:

- Child forms inherit sections and fields from their parent
- Child forms can override inherited fields
- Changes to parent forms can be propagated to child forms
- Multiple levels of inheritance are supported

## Form trees and multiform actors (UAT)

Forms can be linked into a **tree** via `parent_id` (parents ↕ children). The platform
calls such a tree a **UAT**. An actor can then be an instance of **several forms at once**
— a *multiform* actor — typically a base form plus other forms from the same UAT.

- **Form tree (UAT).** `parent_id` chains forms into a hierarchy. Internally the backend
  exposes the tree via the `forms_graph` routes (`/:typeLink/:formId` with
  `typeLink = parents | children`, and `/tree/{accId}/{formId}` which walks to the topmost
  parent and returns all of its descendants). These are **internal** control-plane routes —
  they are **not** part of the curated public MCP tool set.
- **Multiform actor.** An actor has one **root** `form_id` and may carry data for additional
  forms. The set of forms attached to an actor is managed by `PUT /actors/actor_forms/{actorId}`
  (body `{forms}`) — also not exposed as a curated MCP tool yet.

### Multiform data keys — `__form__<formId>:<itemId>`

This is the part that affects actor `data` and so the create/update actor tools. The
constant `KEY_FORM_PREFIX = "__form__"` namespaces fields by their owning form:

- Fields of the actor's **root form** are keyed by the **plain** field `id` (e.g. `"name"`,
  `"item_1780…"`). The backend also accepts/echoes the namespaced form (`__form__<rootFormId>:<itemId>`)
  and aliases the two, so either works for the root form.
- Fields belonging to **another** form in the multiform set are keyed
  **`__form__<thatFormId>:<itemId>`**.

```json
// a multiform actor's data: a root-form field + a field from form 16951
{
  "name": "Kulo Oleksandr",
  "__form__16951:position": "Software engineer"
}
```

The value on the right of the key follows the **same per-class rules** as any other field
(string / number / option arrays / `{type,title,value}` references / calendar object) — the
prefix only changes the *key*, never the value shape. Scanning an actor's data keys for the
`__form__<id>:` prefix is how the platform discovers every form the actor instantiates
(`getActorsAllForms`).

### Creating actors under a UAT tree (root vs leaf)

In a UAT workspace an actor can be created **only under the root form** of the tree, never
under an arbitrary leaf. So when you want an actor "of form X" and X has a non-empty
`parent_id` (X is a child/leaf), creating it directly under X is rejected:

```
400: Form <id> is not UAT
```

The correct flow:

1. Walk up `parent_id` from X to the **root** form (read forms until `parent_id` is empty).
2. Create the actor with `form_id` = the **root**.
3. Put the **root's own fields** under their plain `id`, and the **leaf form X's fields**
   under the `__form__<X>:<itemId>` prefix.

```json
// "create an actor for form Employee (16951)", where Employee.parent_id = People (16950):
// create under the ROOT People (16950), not under Employee.
{
  "form_id": 16950,
  "title": "Olena Kovalenko",
  "data": {
    "name": "Olena Kovalenko",                            // People (root) field
    "__form__16951:position": "Senior Backend Engineer"   // Employee (leaf) field
  }
}
```

> Do **not** flip the leaf form to `uat` status to bypass the error — that mutates the shared
> template. Use the root form's id as the `form_id` instead.

## Accounts and forms

Financial accounts are **attached to actors**, not to the form itself: in the curated
MCP tool set there is no form-level account-attach operation. The form's `sections`
describe field structure only. To give actors of a form a set of accounts, create the
account building blocks once per workspace and attach an account to each actor:

- `createAccountName(accId, name)` / `getAccountNames(accId)` — account-name categories
- `createCurrency(accId, name, symbol, precision)` / `getCurrencies(accId)` — currencies/units
- `createAccount(actorId, nameId, currencyId, accountType)` — attach an account to an actor

A `select` field in the form can still *reference* account names or currencies via
`extra.optionsSource.type = accountNames | currencies` (see the field-class catalogue
above), but that only populates a dropdown — it does not create accounts. See the
`simulator-finance` skill / [accounts.md](accounts.md) for the full financial workflow.

## Database Structure

Forms are stored in the `forms` table with the following structure:

- Primary key on `id`
- Foreign key relationships to workspaces and users
- Indexed for efficient querying by acc_id and parent_id
- JSON storage for sections, fields, and settings

## Example

### Basic Form

```json
{
  "id": 42,
  "title": "Customer",
  "description": "Customer information form",
  "color": "#3498db",
  "sections": [
    {
      "title": "General Information",
      "content": [
        {
          "id": "name",
          "class": "edit",
          "title": "Customer Name",
          "required": true,
          "visibility": "visible"
        },
        {
          "id": "email",
          "class": "edit",
          "title": "Email Address",
          "regexp": "^[\\w-\\.]+@([\\w-]+\\.)+[\\w-]{2,4}$",
          "errorMsg": "Please enter a valid email address",
          "required": true,
          "visibility": "visible"
        }
      ]
    },
    {
      "title": "Additional Information",
      "content": [
        {
          "id": "type",
          "class": "select",
          "title": "Customer Type",
          "value": "",
          "options": [
            {"title": "Individual", "value": "individual"},
            {"title": "Business", "value": "business"}
          ],
          "required": true,
          "visibility": "visible"
        },
        {
          "id": "notes",
          "class": "edit",
          "type": "text",
          "extra": {"multiline": true, "rows": 5},
          "title": "Notes",
          "required": false,
          "visibility": "visible"
        }
      ]
    }
  ],
  "settings": {},
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Worked example — every field class

A trimmed `sections` payload exercising each class, and the matching actor `data`
keyed by the field `id`s. Note how display-only classes (`label`/`button`/`image`)
have no `data` entry, and how each `select` source produces a different value shape.

```json
// form.sections (abridged)
[
  { "title": "Basics", "content": [
    { "id": "item_text",  "key": "1001", "class": "edit", "title": "Name", "required": true, "visibility": "visible" },
    { "id": "item_pass",  "key": "1002", "type": "password", "class": "edit", "title": "Secret", "value": "", "errorMsg": "err msg", "visibility": "visible" },
    { "id": "item_int",   "key": "1003", "type": "int",   "class": "edit", "title": "Count", "regexp": "[0-9]", "visibility": "visible" },
    { "id": "item_chk",   "key": "1004", "class": "check", "title": "Active", "value": "false", "visibility": "visible" },
    { "id": "item_radio", "key": "1005", "class": "radio", "title": "Pick", "value": "",
      "options": [ {"title":"one","value":"o1"}, {"title":"two","value":"o2"} ], "visibility": "visible" }
  ]},
  { "title": "Selects", "content": [
    { "id": "item_sel_static", "key": "1006", "class": "select", "title": "Static", "value": [],
      "options": [ {"color":"#863434","title":"one","value":"s1"}, {"title":"two","value":"s2"} ], "visibility": "visible" },
    { "id": "item_sel_layer", "key": "1007", "class": "select", "title": "From layer", "value": [], "options": [],
      "extra": { "optionsSource": { "type": "layer", "value": { "id": "14b284e0-5f77-4b40-bdde-f7058b05939f" } } }, "visibility": "visible" },
    { "id": "item_sel_cur", "key": "1008", "class": "select", "title": "Currency", "value": [], "options": [],
      "extra": { "optionsSource": { "type": "currencies" } }, "visibility": "visible" },
    { "id": "item_sel_mem", "class": "select", "title": "Member", "value": [], "options": [],
      "extra": { "optionsSource": { "type": "workspaceMembers" } }, "visibility": "visible" }
  ]},
  { "title": "Other", "content": [
    { "id": "item_cal",   "key": "1009", "class": "calendar", "title": "When", "value": {},
      "extra": { "time": true, "dateRange": true, "minDate": 1780606800, "maxDate": 1782766800 }, "visibility": "visible" },
    { "id": "item_label", "key": "1010", "class": "label", "value": "simple text", "align": "center", "visibility": "visible" },
    { "id": "item_img",   "key": "1011", "class": "image", "title": "pic", "value": "https://example.com/a.png", "visibility": "visible" }
  ]}
]
```

```json
// actor.data created from that form — keyed by field id, no entries for label/image
{
  "item_text":  "Acme",
  "item_pass":  "s3cr3t",
  "item_int":   1,
  "item_chk":   true,
  "item_radio": "o2",
  "item_sel_static": [ {"color":"#863434","title":"one","value":"s1"} ],
  "item_sel_layer":  [ {"type":"actor","title":"FlowchartBlock","value":"14b284e0-5f77-4b40-bdde-f7058b05939f"} ],
  "item_sel_cur":    [ {"type":"currency","title":"done","value":9} ],
  "item_sel_mem":    [ {"type":"workspaceMembers","title":"Alex Kulo","value":1} ],
  "item_cal":   { "startDate": 1780736400, "endDate": 1780736400, "timeZoneOffset": -180, "sendInvite": false }
}
```

## Usage in the Platform

Forms are used throughout the platform for various purposes:

- **Data Structure Definition** - Defining the structure and validation rules for actors
- **User Interface Generation** - Automatically generating UI components based on form definitions
- **Validation** - Enforcing data integrity through field validation rules
- **Default Values** - Providing initial values for actor fields
- **Conditional Logic** - Implementing dynamic behavior based on field values

Forms (Smart Forms/CDU) are a central component of the platform, enabling structured data collection and consistent business processes.
