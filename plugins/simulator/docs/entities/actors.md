# Actors

Actors in the Simulator.Company platform represent the core entities in business process graphs, serving as nodes that can be connected through links to model complex workflows.

## Overview

Actors are the fundamental building blocks of the platform, representing various business entities such as tasks, documents, users, or any other object in a business process. Each actor is based on a form template that defines its structure and behavior.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the actor |
| acc_id | String | Workspace ID the actor belongs to |
| user_id | Integer | ID of the user who created the actor |
| form_id | Integer | ID of the form template used by this actor |
| title | Text | Display title of the actor |
| description | Text | Detailed description of the actor |
| ref | String | External reference identifier |
| data | JSON | Custom data associated with the actor, structured according to its form template |
| color | String | Color associated with the actor (hex code) |
| picture | Text | URL or path to the actor's image |
| pictureObject | JSON | Custom image rendered **as the node body** (the backend's "napkin" element) instead of a standard form node — a divider line, shape, icon, logo or small diagram. Shape: `{"img": "data:image/png;base64,…", "width": 800, "height": 8, "type": "napkin"}`; anchored at its centre, keeps the source aspect ratio (set `width`; `height` follows). Settable on createActor / updateActor. |
| hole | Boolean | Marks the actor as a **hole** — an empty placeholder slot on the graph (rendered as a hollow node) that becomes a normal actor once it is filled. Settable on createActor / updateActor. |
| status | Enum | Actor status (active, removed, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |
| app_id | String | ID of the application this actor belongs to (if applicable) |

## Actor Types

The platform supports various actor types based on their purpose:

- **Standard Actors** - Regular business entities with custom data
- **Layer Actors** - Special actors that represent visualization layers
- **System Actors** - Built-in actors for system functionality
- **Reaction Actors** - Actors representing user interactions (comments, approvals, etc.)
- **Application Actors** - Actors representing applications or integrations

## Actor Data Protocol

An actor is an **instance of a form** (a.k.a. Account Template — see [forms.md](forms.md)).
Its `data` object holds the field values, and the contract is strict:

- **Keys are the form field `id`s** — `item_<digits>` — **not** the field `title` and **not** the field's `key`. Always read the form with `getForm` first to learn the ids.
- **Each value's shape is dictated by that field's `class`** (and, for `edit`, its `type`).
- **Display-only fields** (`label`, `button`, `image`) have **no** entry in `data`.
- Fields the user did not fill (and `disabled` fields) may simply be omitted.

| Form field class / source | Value in actor `data` | Example |
|---|---|---|
| `edit` text / password / email / phone | string | `"dsdf@fsdf.com"` |
| `edit` int / float | number | `1` |
| `check` | boolean | `true` |
| `radio` | the selected option's `value` (string) | `"1780650831765_266361868"` |
| `select` (static) | array of chosen option objects | `[{"color":"#863434","title":"one","value":"…"}]` |
| `select` → `layer`/`actorFilter`/`actorsBag`/`actors`/`formFilter` | `[{type:"actor", title, value:<actor UUID>}]` (formFilter = the actors of a given form) | `[{"type":"actor","title":"Simple","value":"e948822e-…"}]` |
| `select` → `forms` | `[{type:"form", title, value:<form id number>}]` | `[{"type":"form","title":"Car","value":42}]` |
| `select` → `currencies` | `[{type:"currency", title, value:<currency id number>}]` | `[{"type":"currency","title":"done","value":9}]` |
| `select` → `accountNames` | `[{type:"accountName", title, value:<account-name UUID>}]` | `[{"type":"accountName","title":"Child Actors","value":"d67e90e0-…"}]` |
| `select` → `workspaceMembers` | `[{type:"workspaceMembers", title, value:<user id number>}]` | `[{"type":"workspaceMembers","title":"Alex Kulo","value":1}]` |
| `select` → `api`/`corezoidSyncApi` | array of synced option objects (`[]` until synced) | `[]` |
| `multiSelect` | array `[{title,value}, …]` | `[{"title":"one","value":"…"},{"title":"two","value":"…"}]` |
| `calendar` | `{startDate,endDate,timeZoneOffset,sendInvite}` (unix **seconds**) | `{"startDate":1780736400,"endDate":1780736400,"timeZoneOffset":-180,"sendInvite":false}` |
| `upload` | array of file refs (`[]` when empty) | `[]` |

> The `type` discriminator (`actor` / `currency` / `accountName` / `workspaceMembers` / `form`) only appears on **dynamic** `select` values, so the platform can resolve the referenced entity. Static `select`/`multiSelect` values carry no `type`.

See [forms.md → Worked example](forms.md#worked-example--every-field-class) for a side-by-side form `sections` ↔ actor `data` pairing.

### Multiform actors (`__form__<formId>:<itemId>`)

An actor can instantiate **more than one form at once** (a *multiform* actor — see
[forms.md → Form trees and multiform actors](forms.md#form-trees-and-multiform-actors-uat)).
Its `data` then mixes fields from several forms, disambiguated by a key prefix:

- Fields of the actor's own **root** `form_id` → the **plain** field `id` (e.g. `"name"`).
  (The namespaced form `__form__<rootFormId>:<itemId>` is also accepted and aliased.)
- Fields of **any other** form → **`__form__<thatFormId>:<itemId>`**.

```json
{
  "name": "Kulo Oleksandr",                 // root-form field, plain id
  "__form__16951:position": "Software engineer"  // field "position" of form 16951
}
```

The prefix changes only the **key** — the value still follows the per-class shapes in the
table above. When you write multiform data, use the plain id for the form you are creating
the actor under and the `__form__<id>:` form for fields owned by the other attached forms.

> **Create under the UAT root, not a leaf.** In a UAT workspace the `form_id` you create an
> actor under must be the **root** of the tree. If the form you want has a non-empty
> `parent_id` (it's a leaf/child), creating directly under it fails with
> `400: Form <id> is not UAT` — walk up `parent_id` to the root, create under the root, and
> put the leaf form's fields under `__form__<leafFormId>:<itemId>`. See
> [forms.md → Creating actors under a UAT tree](forms.md#creating-actors-under-a-uat-tree-root-vs-leaf).

## API Endpoints

For detailed API documentation on actors, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Actors API Documentation](https://doc.simulator.company/#tag/actors)

The API provides endpoints for:

- Getting actor details
- Creating new actors
- Updating existing actors
- Deleting actors
- Retrieving actors by form
- Getting and updating actor data
- Managing actor relationships

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for read operations and `control.events:actors.management` for write operations).

## Relationships

Actors have relationships with various other entities in the system:

- **Forms** - Each actor is based on a form template that defines its structure
- **Links** - Actors can be connected to other actors through links
- **Accounts** - Actors can have associated financial accounts
- **Attachments** - Files can be attached to actors
- **Layers** - Actors can be placed on visualization layers
- **Reactions** - Users can interact with actors through reactions

## Database Structure

Actors are stored in the `actors` table with the following structure:

- Primary key on `id`
- Foreign key relationships to forms, users, and workspaces
- Indexed for efficient querying by form_id, ref, and other properties
- Full-text search capabilities through the ActorsSearch model

## Example

### Actor JSON

```json
{
  "id": "actor_123456",
  "title": "Customer Onboarding",
  "description": "Process for onboarding new customers",
  "form_id": 42,
  "data": {
    "customer_name": "Acme Corp",
    "contact_email": "contact@acme.com",
    "onboarding_stage": "documentation",
    "priority": "high"
  },
  "color": "#3498db",
  "status": "active",
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Actor with Relationships

```json
{
  "id": "actor_123456",
  "title": "Customer Onboarding",
  "form_id": 42,
  "data": { ... },
  "links": [
    {
      "id": "link_789012",
      "target_id": "actor_345678",
      "type_id": 1,
      "type_name": "Process Flow"
    }
  ],
  "accounts": [
    {
      "id": "account_901234",
      "name_id": "account_name_567",
      "name": "Project Budget",
      "amount": 5000.00,
      "currency_id": 1,
      "currency_symbol": "$"
    }
  ],
  "attachments": [
    {
      "id": "attachment_234567",
      "filename": "contract.pdf",
      "size": 1024000,
      "mime_type": "application/pdf"
    }
  ]
}
```

## Usage in the Platform

Actors are used throughout the platform for various purposes:

- **Business Process Modeling** - Representing entities in process flows
- **Data Collection** - Structured data entry through form templates
- **Financial Tracking** - Association with accounts for financial operations
- **Collaboration** - User interactions through reactions and comments
- **Visualization** - Placement on layers for visual organization

Actors form the foundation of the platform's graph-based approach to business process management, enabling flexible and powerful modeling of complex workflows.
