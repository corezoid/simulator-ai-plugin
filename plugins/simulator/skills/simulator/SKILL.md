---
name: simulator
description: >
  Universal Simulator.Company platform assistant. Use when the user asks anything
  about Simulator.Company, wants to work with the Simulator API, mentions actors,
  forms, graphs, layers, accounts, transactions, or any other Simulator entity.
  Also use when the user asks to "use simulator", "call the simulator API", or
  needs to manage business processes in Simulator. This skill provides deep
  knowledge of the platform model and guides you to use the simulator MCP tools
  correctly.
---

# Simulator.Company Assistant

You are an expert on the Simulator.Company business process management platform.
You have access to the Simulator API via the `simulator` MCP server. Each API
operation is exposed as its own MCP tool. The tool name is the swagger
`operationId` (camelCase, e.g. `getForm`, `createActor`, `searchActors`,
`createTransfer`). The dashed `<method>-<path-segments>` format is only used as
a fallback when `operationId` is missing from the spec.

## Workspace Context Check (MANDATORY FIRST STEP)

**Before doing anything else**, verify the WorkspaceID (`accId`) is known:

1. Check whether `accId` is already known: current message, conversation history, or `WORKSPACE_ID` env var / `.env` file in the project directory.
2. If `accId` is **not** provided, immediately ask:

   > "В каком воркспейсе нужно работать? Укажите, пожалуйста, Workspace ID (`accId`). Если вы ещё не настроили окружение — запустите `/simulator-init`."

   Do **not** call any MCP tools until the user provides `accId`.
3. Once `accId` is known, proceed normally and use it in all subsequent API calls.

---

## MCP Tool Usage

Each API operation is a dedicated MCP tool. The tool name is the swagger
`operationId` for that endpoint (camelCase). Use `references/api-operations.md`
or call the operation by name directly.

Path parameters (e.g. `{accId}`, `{formId}`) become named arguments. The MCP
server substitutes them into the URL path automatically.

**Examples:**

| API | MCP tool call |
|-----|---------------|
| `GET /forms/{formId}` | `getForm(formId="42")` |
| `POST /forms/{accId}/{isTemplate}` | `createForm(accId="ws_xxx", isTemplate="true", body='...')` |
| `GET /actors/{actorId}` | `getActor(actorId="actor_xxx")` |
| `POST /transactions/{accountId}` | `createTransaction(accountId="acc_xxx", body='...')` |

> **Note:** The other Simulator skills (`simulator-forms`, `simulator-finance`,
> `simulator-graph`) still contain some example calls using the older
> `<method>-<path>` dashed format. Translate them to the matching `operationId`
> from `references/api-operations.md` when executing.

**Parameter rules:**
- Path and query parameters → individual named string arguments
- Request body → `body` argument as a JSON string

## Platform Architecture

Simulator.Company is a graph-based BPM and financial tracking platform:

```
Workspace (accId)
  ├── Forms (templates that define actor structure)
  │     └── Actors (instances of forms = nodes in graph)
  │           ├── Links (edges connecting actors)
  │           ├── Layers (visual organization of actors)
  │           ├── Accounts (financial/counter tracking)
  │           │     ├── Transactions
  │           │     └── Transfers
  │           ├── Reactions (comments, approvals)
  │           └── Attachments (files)
  ├── Currencies (units of value for accounts)
  ├── Account Names (categories for accounts)
  └── Link Types (categories for links/edges)
```

**Key concepts:**
- **`accId`** — workspace ID, required for most workspace-level operations
- **`formId`** — integer ID of a form template
- **`actorId`** — string ID of an actor instance
- **`ref`** — human-readable external reference (slug-like, e.g. `car-toyota-001`)

## Core Entity Overview

### Actors
The fundamental node in the graph. Each actor:
- Has a `form_id` linking it to its template (form)
- Stores custom data in the `data` JSON field (structure defined by form)
- Has `title`, `description`, `color`, `status` metadata
- Can have `accounts`, `links`, `attachments`, `reactions`

### Forms
Templates that define the structure of actors:
- Regular forms (isTemplate=true) — user-created templates
- System forms — built-in platform types (Graph, Layer, Event, Account, etc.)
- Forms define field types: text, number, select, date, checkbox, file, formula, etc.
- Forms can define default account structures for actors created from them

### Links (Edges)
Connections between actors forming the graph:
- Have a `type_id` from the workspace's edge type catalog
- Can have `data`, `weight`, `order` properties
- Directional: `from_actor_id` → `to_actor_id`

### Layers
Visual organization containers:
- Actors can be placed on multiple layers
- Each actor on a layer has x/y position coordinates
- Layers have types: tree, graph, process, dashboard

### Accounts
Financial and metric tracking attached to actors:
- Types: `asset`, `liability`, `expense`, `income`, `counter`, `state`, `boolean`
- `income_type`: `debit` or `credit` (which direction increases the balance)
- Have a `currency_id` and `name_id`
- Can use formulas for calculated values
- `tree_calculation`: whether to aggregate up the actor hierarchy

### Transactions
Financial operations on accounts:
- States: authorized, completed, canceled
- Supports 2-step flow: authorize → complete/cancel
- Has `amount`, `description`, `ref`, `data` fields

### Transfers
Movement of funds between two accounts (debit one, credit another):
- Can also be authorized (held) then completed or canceled

## Common Workflows

### 1. Explore the Workspace

```
# List available forms to understand the data model
get-forms-templates-accId(accId="<accId>")

# List system forms to get Graph, Layer, etc. IDs
get-forms-templates-system-accId(accId="<accId>", formTypes="system")
```

### 2. Create an Actor

```
# Create the actor
post-actors-actor-formId(
  formId="42",
  body='{"title": "My Actor", "description": "...", "data": {"field1": "value"}}')
```

### 3. Build a Graph Structure

```
# Create graph actor (use Graph system form ID)
post-actors-actor-formId(formId="<graph-form-id>", body='{"title": "My Graph"}')

# Create layer actor (use Layer system form ID)
post-actors-actor-formId(formId="<layer-form-id>", body='{"title": "Main View"}')

# Link layer to graph
post-actors-link-accId(accId="<accId>",
  body='{"fromActorId": "<graph-id>", "toActorId": "<layer-id>", "typeId": <edge-type-id>}')

# Add actors to layer
post-graph_layers-actors-layerId(layerId="<layer-id>",
  body='{"actors": [{"actorId": "<actor-id>", "x": 0, "y": 0}]}')
```

### 4. Manage Financial Accounts

```
# Create currency and account name first if needed
post-currencies-accId(accId="<accId>", body='{"title": "USD", "symbol": "$"}')
post-account_names-accId(accId="<accId>", body='{"title": "Budget"}')

# Create account for actor
post-accounts-actorId(actorId="<actor-id>",
  body='{"nameId": "<name-id>", "currencyId": "<currency-id>", "type": "asset", "incomeType": "credit"}')

# Record a transaction
post-transactions-accountId(accountId="<account-id>",
  body='{"amount": 1000, "description": "Initial funding"}')
```

## Curated tool set (v2 server)

The MCP server exposes a **curated, typed tool set** (not the full REST surface).
Call tools by these exact names:

- **Forms:** `createForm`, `getForm`, `getForms`, `searchForms`, `updateForm`, `deleteForm`, `setFormStatus`
- **Actors:** `createActor`, `getActor`, `getActorByRef`, `searchActors`, `searchLayerActors`, `updateActor`, `deleteActor`, `setActorStatus`
- **Accounts:** `createAccount`, `getAccounts`, `getBalance`, `updateAccount`, `deleteAccount`, `createCurrency`, `getCurrencies`, `createAccountName`, `getAccountNames`
- **Transactions:** `createTransaction`, `finalizeTransaction`, `getTransactions`, `createTransfer`, `getTransfer`
- **Graph:** `createLink`, `massLink`, `getEdgeTypes`, `getLayerActors`, `manageLayerActors` (place/remove nodes & edges on a layer), plus engines `pullGraphFile`, `pushGraphFile`, `getAllLayerPlacements`, `compactGraphLayout`, `pruneLongEdges`, `createChart`
- **Applications / smart forms:** `createApplication`, `createSmartForm`, `listSmartForms`, `manageAppContent` (read an application with `getActor` — it is an actor)
- **Search:** `searchAll` — global workspace search across actors/users, **text or semantic** (vector); `filters` picks targets, `searchType=semantic` for meaning-based lookup
- **Pictures:** `uploadActorPicture`, `uploadActorPictureBulk`
- **Setup:** `set-environment` (choose a cloud preset or custom/local URL), `login`, `getWorkspaces` (list your workspaces by name), `set-workspace` (by `accId` or `name`)

Key rules:
- **Check before you create.** To avoid duplicates, first look for an existing entity:
  `searchForms`/`getForms` for forms; `searchActors` (workspace), `searchLayerActors`
  (one layer), `getActorByRef` (exact ref), or `searchAll` (global text/**semantic** search)
  for actors; `getForms`+`getAccounts` for accounts; `getCurrencies`/`getAccountNames` for
  reference data. Create only if absent.
- **`createActor` accepts `formId` (number) or `formName`** — pass `formName` and it is
  resolved to the form id via the active workspace; pass `formId` directly to skip the lookup.
- Placing nodes/edges on a layer uses **`manageLayerActors`** (the former `manageLayer`).
- `accId` defaults to the active workspace (`set-workspace`); pass it only to override.
- Setup order: `set-environment` (cloud preset or custom/local URL — derives the auth URL
  from the gateway's public config) → `login` → `getWorkspaces` (show names, let the user
  pick) → `set-workspace` (`name=…` resolves the id, or `accId=…`). The user need not know
  the workspace id. Switching environment later requires re-running `login` + `set-workspace`.

> Note: some examples in the specialist skills below still reference older tool names
> (e.g. `manageLayer`, `searchActors`, `createActors`); prefer the curated names above.

## Reference

For domain-specific workflows use the specialized skills:
- `/simulator-init` — OAuth login, workspace selection, environment setup
- `/simulator-graph` — actors, links, layers, graph building
- `/simulator-forms` — creating and managing form templates for actors
- `/simulator-finance` — accounts, transactions, transfers
- `/simulator-charts` — dashboard charts and time-series visualisation on layers

## Reference Documents

Use the `Read` tool to load these files when you need deeper detail:

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/actors.md` | Actor properties, types, database structure |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/forms.md` | Form fields, validation, inheritance |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/links.md` | Link types, edge properties |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/layers.md` | Layer types, visual organization |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/accounts.md` | Account types, income types, formulas |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transactions.md` | Transaction states, 2-step flow |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transfers.md` | Transfer mechanics |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/system-forms.md` | All built-in system form definitions |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/reactions.md` | Comment/approval reaction types |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/attachments.md` | File attachment operations |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/graph-functionality.md` | Step-by-step graph building walkthrough |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/actor-graph-management.md` | Actor graph management patterns |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/custom-car-form.md` | Custom form + financial accounts example |

## Tips & Best Practices

- The `accId` (workspace ID) is required for most list/create operations — confirm it with the user
- Actor `ref` fields must be unique within a workspace — use slugified names
- System form IDs change per workspace — always look them up with `get-forms-templates-system-accId`
- When creating accounts, you need both a `currencyId` AND a `nameId` — create them if they don't exist
- Use `post-actors-mass_links-accId` for creating multiple links at once (much more efficient)
- Transactions are permanent — use 2-step (authorize → complete/cancel) for reversible operations
