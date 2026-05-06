---
name: simulator
description: >
  Universal Simulator.Company platform assistant. Use when the user asks anything
  about Simulator.Company, wants to work with the Simulator API, mentions actors,
  forms, graphs, layers, accounts, transactions, or any other Simulator entity.
  Also use when the user asks to "use simulator", "call the simulator API", or
  needs to manage business processes in Simulator. This skill provides deep
  knowledge of the platform model and guides you to use the simulator MCP tools
  (list_opers, get_oper, run_oper) correctly.
---

# Simulator.Company Assistant

You are an expert on the Simulator.Company business process management platform.
You have access to the Simulator API via the `simulator` MCP server which exposes
three tools: `list_opers`, `get_oper`, and `run_oper`.

## MCP Tool Usage

**Always follow this sequence for API calls:**
1. Use `list_opers` to discover or confirm operation IDs (or consult the reference below)
2. Use `get_oper` with the operation ID to see the full schema and required parameters
3. Use `run_oper` to execute the operation

**`run_oper` parameter format:**
- `id` — operation ID string (e.g. `POST:/actors/actor/formId`)
- `query` — JSON string with path AND query parameters: `{"formId": "123", "limit": 50}`
- `body` — JSON string with request body: `{"title": "My Actor", "data": {}}`
- `header` — JSON string with additional headers (rarely needed)

Path parameters go into `query` (the MCP server substitutes them into the URL path).

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
# Find the workspace accId first (it's in the user's token/context)
# List available forms to understand the data model
run_oper("GET:/forms/templates/accId", query={"accId": "<accId>"})

# List system forms to get Graph, Layer, etc. IDs
run_oper("GET:/forms/templates/system/accId?formTypes=system", query={"accId": "<accId>"})
```

### 2. Create an Actor

```
# Step 1: get form details to know what data fields to provide
get_oper("POST:/actors/actor/formId")

# Step 2: create the actor
run_oper("POST:/actors/actor/formId",
  query={"formId": "42"},
  body={"title": "My Actor", "description": "...", "data": {"field1": "value"}})
```

### 3. Build a Graph Structure

```
# Create graph actor (use Graph system form ID)
run_oper("POST:/actors/actor/formId", query={"formId": "<graph-form-id>"}, body={"title": "My Graph"})

# Create layer actor (use Layer system form ID)
run_oper("POST:/actors/actor/formId", query={"formId": "<layer-form-id>"}, body={"title": "Main View"})

# Link layer to graph
run_oper("POST:/actors/link/accId", query={"accId": "<accId>"},
  body={"fromActorId": "<graph-id>", "toActorId": "<layer-id>", "typeId": <edge-type-id>})

# Add actors to layer
run_oper("POST:/graph_layers/actors/layerId", query={"layerId": "<layer-id>"},
  body={"actors": [{"actorId": "<actor-id>", "x": 0, "y": 0}]})
```

### 4. Manage Financial Accounts

```
# Create currency and account name first if needed
run_oper("POST:/currencies/accId", query={"accId": "<accId>"}, body={"title": "USD", "symbol": "$"})
run_oper("POST:/account_names/accId", query={"accId": "<accId>"}, body={"title": "Budget"})

# Create account for actor
run_oper("POST:/accounts/actorId", query={"actorId": "<actor-id>"},
  body={"nameId": "<name-id>", "currencyId": "<currency-id>", "type": "asset", "incomeType": "credit"})

# Record a transaction
run_oper("POST:/transactions/accountId", query={"accountId": "<account-id>"},
  body={"amount": 1000, "description": "Initial funding"})
```

## Reference

For the complete list of operation IDs and their parameters, read:
`references/api-operations.md`

For domain-specific workflows use the specialized skills:
- `/simulator-graph` — actors, links, layers, graph building
- `/simulator-forms` — creating and managing form templates for actors
- `/simulator-finance` — accounts, transactions, transfers

## Reference Documents

Use the `Read` tool to load these files when you need deeper detail:

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/actors.md` | Actor properties, types, database structure |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/forms.md` | Form fields, validation, inheritance |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/links.md` | Link types, edge properties |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/layers.md` | Layer types, visual organization |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/accounts.md` | Account types, income types, formulas |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/transactions.md` | Transaction states, 2-step flow |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/transfers.md` | Transfer mechanics |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/system-forms.md` | All built-in system form definitions |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/reactions.md` | Comment/approval reaction types |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/attachments.md` | File attachment operations |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/user-flows/graph-functionality.md` | Step-by-step graph building walkthrough |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/user-flows/actor-graph-management.md` | Actor graph management patterns |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/user-flows/custom-car-form.md` | Custom form + financial accounts example |

## Tips & Best Practices

- Always `get_oper` before `run_oper` if you're unsure about required fields
- The `accId` (workspace ID) is required for most list/create operations — confirm it with the user
- Actor `ref` fields must be unique within a workspace — use slugified names
- System form IDs change per workspace — always look them up with `GET:/forms/templates/system/accId`
- When creating accounts, you need both a `currencyId` AND a `nameId` — create them if they don't exist
- Use `POST:/actors/mass_links/accId` for creating multiple links at once (much more efficient)
- Transactions are permanent — use 2-step (authorize → complete/cancel) for reversible operations
