---
name: simulator-graph
description: >
  Simulator.Company graph structure specialist. Use when the user wants to
  build, edit, analyze, inspect, or ask questions about business process
  graphs, flowcharts, algorithms, actors (nodes), links (edges), or layers
  (visual views) in Simulator.Company.

  Trigger on any of these intents:
  — Creating: "create graph", "build flowchart", "new diagram", "add actor",
    "add block to graph", "create algorithm", "draw flowchart", "digital twin",
    "create process on graph", "build process diagram", "FlowchartBlock",
    "startStop", "predefinedProcess".
  — Editing: "edit graph", "update actor", "rename node", "change color",
    "move actor", "add step to flowchart", "modify diagram", "restructure
    process", "add edge", "remove link", "reorder steps", "update layer".
  — Syncing: "push graph", "pull graph", "sync graph", "push changes",
    "apply edits to layer".
  — Querying / analysis: "what actors are on this layer", "show me the graph",
    "who is connected to", "find actor", "list nodes", "describe the process",
    "analyze the flowchart", "how many steps", "what links exist",
    "search layer", "inspect actor", "get actor", "explore graph".
  — Layer / connection operations: "add to layer", "connect actors",
    "link nodes", "organize on layer", "move actors between layers",
    "explore graph connections".

  Covers the full actor lifecycle (create, update, delete, search), all graph
  traversal operations, layer management, and FlowchartBlock diagram creation.
---

> **Curated tool names (v2 server).** Place/remove nodes & edges on a layer with `manageLayerActors`; read layer membership with `getLayerActors` / `getAllLayerPlacements`; create nodes with `createActor` (one call each — there is no `createActors`); links with `createLink` / `massLink`; edge CRUD with `getEdge` / `updateEdge` / `deleteEdge` / `existLink` / `deleteEdgesByNodes`; edge types with `getEdgeTypes`. Traverse from an actor with `getRelatedActors` (type = linked | parents | children; hierarchy link type by default; paginated/filterable/sortable), `getLinkedActors` (directly-linked actors across edge types, with `edgeTypes`/`withSystem`/`pinned` filters), and `getActorLinks` (every edge of an actor). Layer ops: `layerStats` (node/edge counts), `existLayerElement` (is a node/edge on a layer — dedup before placing), `moveActors` (move ≤10 actors between layers), `cleanGraphLayer` (wipe a layer — destructive). See `/simulator` for the full list.

# Simulator.Company Graph Builder

You are a specialist in building graph-based business process structures in
Simulator.Company using the `simulator` MCP server.

---

## Core Concepts & Glossary

| Term            | Description                                                                                          |
|-----------------|------------------------------------------------------------------------------------------------------|
| **Actor**       | Graph node. Created from a Form template. Has id, title, color, data fields.                         |
| **Form**        | Actor template/type. Defines shape and behavior. All system form names are in the catalog below.     |
| **Graph actor** | An actor with `formName="Graphs"` — the logical container for a diagram.                             |
| **Layer actor** | An actor with `formName="Layers"` — the visual canvas where nodes are placed at (x, y).              |
| **Graph file**  | A YAML file named `<layerId>.yaml` in the current working directory describing the full layer state. |
| **laId**        | Layer Actor ID. Assigned by `manageLayerActors` when an actor is placed on a layer.                        |

---

## Primary Workflow — File-Based Graph Building

> **This is the preferred approach for all graph creation and editing.**
> Use the low-level MCP tools (createActor, manageLayerActors, etc.) only for
> one-off queries or when specifically requested.

### Step 1 — Create Graph + Layer actors

Every diagram needs two actors linked together. Do this once per new diagram.

```
// 1. Create the graph container
createActor(formName="Graphs", body='{"title":"<diagram name>"}')
→ save returned id as graphId

// 2. Create the visual canvas (layer)
createActor(formName="Layers", body='{"title":"<diagram name>"}')
→ save returned id as layerId

// 3. Link graph → layer
createLink(body='{"source":"<graphId>","target":"<layerId>"}')
```

If `layerId` is already known (from user message or context) — skip this step entirely.

---

### Step 2 — Prepare the Graph File

**Option A — New empty layer:** write `<layerId>.yaml` from scratch:

```yaml
layerId: "<layerId>"
actors:
  - id: start
    title: "Start"
    formName: "Start / Stop"
    color: "#17B26A"
    position:
      x: 0
      y: 0
  - id: process1
    title: "Process Step"
    formName: "Process"
    color: "#539fdf"
    description: "Does something important"
    position:
      x: 0
      y: 130
  - id: end
    title: "End"
    formName: "Start / Stop"
    color: "#F04438"
    position:
      x: 0
      y: 260
edges:
  - source: start
    target: process1
  - source: process1
    target: end
```

**Option B — Existing layer:** pull current state into a file, then edit it:

```
pullGraphFile(layerId="<layerId>")
→ creates <layerId>.yaml with all current actors and edges
→ open and edit the file as needed
```

---

### Step 3 — Push the File to Server

```
pushGraphFile(layerId="<layerId>")
```

The server will:

- **Create** actors whose `id` is a local name (e.g. `start`, `process1`) → replaces local ids with server UUIDs in the
  file
- **Update** actors whose `id` is already a UUID — if title/color/description changed
- **Delete from layer** actors present on server but missing from file
- **Create** missing edges (links + layer placement)
- **Delete from layer** edges present on server but missing from file
- **Update positions** for actors whose `position` changed

After push the file is updated in place with all server UUIDs.

---

### Editing an Existing Graph

```
// 1. Pull current state
pullGraphFile(layerId="<layerId>")

// 2. Edit <layerId>.yaml — add/remove/modify actors and edges

// 3. Push changes
pushGraphFile(layerId="<layerId>")
```

---

## Custom Form Data — Populating `actors.data`

When the user specifies a **custom `formId`** (or a non-system `formName`) for one or more actors, you **must** fetch the form schema before writing the YAML file or pushing to the server.

### When to trigger

- User explicitly provides a `formId` UUID for an actor.
- User names a form that is **not** in the system Form Catalog above (i.e. it is a user-created form).
- User says "use form X for this actor" where X is an ID or an unfamiliar name.

### Step-by-step

```
// 1. Fetch form schema
getForm(formId="<formId>")
→ returns form object with fields list

// 2. Inspect the returned fields array — each field has:
//    { id, name, title, type, defaultValue, ... }

// 3. Build actors.data from those fields:
//    - Include every field that the user provided a value for.
//    - For fields the user did NOT mention: include the field with its
//      defaultValue if one exists; omit the field entirely otherwise.
//    - Use the field's `name` (not `title`) as the key in data.

// 4. Write the actor entry in the YAML with the populated data block.
```

### YAML example with custom form data

```yaml
actors:
  - id: my_actor
    title: "My Custom Actor"
    formId: "abc-123-uuid"   # custom form — data was populated from getForm
    color: "#539fdf"
    position:
      x: 0
      y: 130
    data:
      status: "active"        # field name from form schema, value from user
      priority: 1             # field name from form schema, value from user
      category: ""            # field present in schema, no default, user left blank
```

### Rules

- **Always call `getForm` before writing `data`** when a custom formId is involved — never guess field names.
- If the form has no fields, omit `data` entirely.
- Do **not** add `data` for system forms (those in the Form Catalog) — the server auto-injects their shape/view/blockId.
- If the user specifies only some field values, fill the rest from `defaultValue`; omit fields with no default and no user value.
- After `getForm`, confirm the field list with the user if the form has many fields and the user has not specified values — ask which fields matter.

---

## Graph File Format Reference

```yaml
layerId: "<uuid>"           # layer actor UUID

actors:
  - id: local_name          # local id for new actors; UUID for existing ones
    title: "My Block"
    formName: "Process"     # use formName from the catalog (preferred over formId)
    color: "#539fdf"        # hex color string — always set for flowchart blocks
    description: "..."      # optional
    picture: ""             # optional
    position:
      x: 0                  # horizontal position on layer
      y: 130                # vertical position on layer

edges:
  - source: local_name_or_uuid   # references actor id field
    target: local_name_or_uuid
    source_title: "My Block"     # informational only, not sent to server
    target_title: "Other Block"
```

**Rules:**

- `id` values are local references used only within the file for edge wiring.
  After `pushGraphFile` all local ids are replaced with server UUIDs.
- Do **not** include `data` for system forms — the server auto-injects shape/view.
- `formName` takes priority over `formId` when both are present.
- `edges` reference actor `id` fields (local names work, they are resolved at push time).

---

## Layout Algorithm — Coordinate Calculation

**Never hardcode coordinates.** Calculate using dagre/Sugiyama layout.

### Node Sizes

```
SIZES = {
  "Start / Stop":         { w: 200, h: 50  },
  "Process":              { w: 200, h: 50  },
  "Predefined Process":   { w: 200, h: 100 },
  "Decision":             { w: 200, h: 100 },
  "Data":                 { w: 200, h: 60  },
  "Document":             { w: 200, h: 70  }
}
default:                  { w: 200, h: 50  }
```

### Rank Assignment (BFS from start node)

```
rank[start] = 0
for each edge source → target:
  rank[target] = max(rank[target] ?? 0, rank[source] + 1)
```

### Gap Calculation

```
nodeSep(rank) = max(max_w_in_rank * 0.3, 60)    // horizontal gap between centers
rankSep(r)    = max(max_h_in_rank(r) * 1.2, 80) // vertical gap between rows
```

### Y Coordinates (top → down)

```
y[rank_0] = 0
y[rank_n] = y[rank_n-1] + max_h(rank_n-1)/2 + rankSep(rank_n-1) + max_h(rank_n)/2
```

### X Coordinates (center each row)

```
for each rank with N nodes:
  center_to_center = max_w_in_rank + nodeSep
  total_span       = (N - 1) * center_to_center
  x[node_i]        = -total_span / 2 + i * center_to_center
```

### Examples

- **3 Start/Stop blocks in a row** (w=200, nodeSep=60, c2c=260) → x = [-260, 0, 260]
- **Start/Stop(h=50) → Process(h=50)** → y[rank_1] = 0+25+80+25 = 130
- **Process(h=50) → Decision(h=100)** → y[rank_2] = 130+25+80+50 = 285

---

## Complete Example: Build a Flowchart

```
// Step 1 — Create Graph + Layer (skip if layerId already known)
createActor(formName="Graphs", body='{"title":"Order Processing"}')  → graphId
createActor(formName="Layers", body='{"title":"Order Processing"}')  → layerId
createLink(body='{"source":"<graphId>","target":"<layerId>"}')

// Step 2 — Write <layerId>.yaml
```

```yaml
layerId: "<layerId>"
actors:
  - id: start
    title: "Start"
    formName: "Start / Stop"
    color: "#17B26A"
    position: { x: 0, y: 0 }
  - id: validate
    title: "Validate Order"
    formName: "Process"
    color: "#539fdf"
    position: { x: 0, y: 130 }
  - id: check
    title: "Valid?"
    formName: "Decision"
    color: "#F79009"
    position: { x: 0, y: 285 }
  - id: process
    title: "Process Payment"
    formName: "Process"
    color: "#539fdf"
    position: { x: 260, y: 440 }
  - id: error
    title: "Return Error"
    formName: "Process"
    color: "#F04438"
    position: { x: -260, y: 440 }
  - id: end
    title: "End"
    formName: "Start / Stop"
    color: "#17B26A"
    position: { x: 0, y: 595 }
edges:
  - source: start
    target: validate
  - source: validate
    target: check
  - source: check
    target: process
  - source: check
    target: error
  - source: process
    target: end
  - source: error
    target: end
```

```
// Step 3 — Push to server
pushGraphFile(layerId="<layerId>")
// → all actors created, edges drawn, file updated with server UUIDs
```

---

## Form Catalog — All Available Form Names

Pass `formName` to `createActor` or in the graph YAML file.
The server resolves the name to ID automatically.

### Graph & Layer

| formName   | Usage                       |
|------------|-----------------------------|
| `"Graphs"` | Graph container actor       |
| `"Layers"` | Layer (visual canvas) actor |

### Flowchart Blocks

> Always set `color` (hex string) for flowchart blocks.

| formName               | Description                     |
|------------------------|---------------------------------|
| `"Start / Stop"`       | Start or end node               |
| `"Process"`            | Process step                    |
| `"Decision"`           | Decision diamond                |
| `"Predefined Process"` | Predefined / subroutine process |
| `"Document"`           | Document shape                  |
| `"Data"`               | Data parallelogram              |
| `"Documents"`          | Multi-document stack            |
| `"Stored Data"`        | Stored data shape               |
| `"Off-page Reference"` | Off-page connector              |
| `"Preparation"`        | Preparation / initialization    |
| `"API Call"`           | API call block                  |
| `"Manual Input"`       | Manual input                    |
| `"Delay"`              | Delay block                     |
| `"Database"`           | Database cylinder               |
| `"Manual Operation"`   | Manual operation                |
| `"Terminator"`         | Oval terminator                 |
| `"Root Node"`          | Root node                       |
| `"Null"`               | Universal generic node          |
| `"Flowchart"`          | Nested flowchart reference      |

### Diagram Types

| formName             | Description              |
|----------------------|--------------------------|
| `"Petri Net"`        | Petri net diagram        |
| `"Sequence Diagram"` | Sequence diagram         |
| `"Actor Graph"`      | Actor graph              |
| `"Mind Map"`         | Mind map                 |
| `"Corezoid"`         | Corezoid process diagram |

### Corezoid Nodes

| formName                          | Description         |
|-----------------------------------|---------------------|
| `"Corezoid Start"`                | Entry point         |
| `"Corezoid API Call"`             | External API call   |
| `"Corezoid Condition"`            | Condition / routing |
| `"Corezoid Code"`                 | Code execution      |
| `"Corezoid Copy Task"`            | Copy task           |
| `"Corezoid Modify Task"`          | Modify task         |
| `"Corezoid Sum"`                  | Aggregation / sum   |
| `"Corezoid Delay"`                | Time delay          |
| `"Corezoid Database Call"`        | DB call             |
| `"Corezoid Waiting for Callback"` | Async wait          |
| `"Corezoid Call a Process"`       | Sub-process call    |
| `"Corezoid Reply to Process"`     | Reply to caller     |
| `"Corezoid Queue"`                | Queue node          |
| `"Corezoid Get from Queue"`       | Dequeue             |
| `"Corezoid Set Parameters"`       | Set task parameters |
| `"Corezoid End: Success"`         | Success terminal    |
| `"Corezoid End: Error"`           | Error terminal      |
| `"Corezoid GIT Call"`             | GIT call            |

### AWS Services

| formName                               | formName                   | formName                       |
|----------------------------------------|----------------------------|--------------------------------|
| `"EC2"`                                | `"Lambda"`                 | `"RDS"`                        |
| `"App Runner"`                         | `"EKS Cloud"`              | `"EKS Distro"`                 |
| `"EFS"`                                | `"Client VPN"`             | `"Elastic Container Registry"` |
| `"Certificate Manager"`                | `"Simple Queue Service"`   | `"Inspector"`                  |
| `"GuardDuty"`                          | `"Data Pipeline"`          | `"Kinesis"`                    |
| `"Key Management Service"`             | `"DynamoDB"`               | `"Elastic Kubernetes Service"` |
| `"Secrets Manager"`                    | `"Elastic Load Balancing"` | `"Route 53"`                   |
| `"ElastiCache"`                        | `"CloudWatch"`             | `"Transfer Family"`            |
| `"Managed Streaming for Apache Kafka"` | `"Amplify"`                | `"Budgets"`                    |
| `"Simple Storage Service"`             | `"Transit Gateway"`        | `"Network Firewall"`           |
| `"OpenSearch Service"`                 | `"WAF"`                    | `"Virtual Private Cloud"`      |
| `"Simple Notification Service"`        | `"API Gateway"`            | `"Transcribe"`                 |
| `"Athena"`                             | `"QuickSight"`             | `"CloudFront"`                 |
| `"Global Accelerator"`                 | `"Backup"`                 | `"DocumentDB"`                 |
| `"Glue"`                               | `"RDS on VMware"`          |                                |

### AI / ML

| formName              | Description               |
|-----------------------|---------------------------|
| `"GPT"`               | GPT model node            |
| `"Anthropic"`         | Anthropic model node      |
| `"Grok"`              | Grok model node           |
| `"SLM"`               | Small language model      |
| `"Devin"`             | Devin AI agent            |
| `"Black box (Mind)"`  | Opaque mind component     |
| `"Black box (LLM)"`   | Opaque LLM component      |
| `"Black box (Actor)"` | Opaque actor component    |
| `"Black box (Seq)"`   | Opaque sequence component |
| `"Grey box (Actor)"`  | Semi-transparent actor    |
| `"Grey box (Mind)"`   | Semi-transparent mind     |
| `"Grey box (LLM)"`    | Semi-transparent LLM      |
| `"Grey box (Seq)"`    | Semi-transparent sequence |

### UML / OOP

| formName      | Description   |
|---------------|---------------|
| `"Class"`     | UML class     |
| `"Interface"` | UML interface |
| `"Component"` | UML component |
| `"Package"`   | UML package   |
| `"Artifact"`  | UML artifact  |

### Other

| formName             | Description                 |
|----------------------|-----------------------------|
| `"Human"`            | Human participant           |
| `"Node"`             | Generic network node        |
| `"Branch Node"`      | Branch point                |
| `"White box"`        | Transparent white container |
| `"Place"`            | Physical location           |
| `"Token"`            | Token / badge               |
| `"Flag"`             | Flag marker                 |
| `"Program"`          | Program block               |
| `"Current Position"` | Current position marker     |
| `"Transition"`       | State transition            |
| `"Stubnet"`          | Stub network                |
| `"Marketplace"`      | Marketplace node            |

---

## Low-Level MCP Tools (one-off operations)

Use these for targeted queries, not for building graphs (use the file workflow instead).

### Actor Operations

```
createActor(formName="Process", body='{"title":"Step","color":"#539fdf"}')
createActors(formName="Process", actors='[{"title":"A","color":"#539fdf"},{"title":"B","color":"#539fdf"}]')
getActor(actorId="<actorId>")
updateActor(formId=<formId>, actorId="<actorId>", body='{"title":"Updated"}')
deleteActor(actorId="<actorId>")
deleteBulk(body='{"actorIds":["<a1>","<a2>"]}')
```

### Link Operations

```
// Preferred — creates links AND places them on the layer automatically
massLink(layerId="<layerId>", body='[{"source":"<a>","target":"<b>"}]')

// Single link — two calls required (logical + visual)
createLink(body='{"source":"<a>","target":"<b>"}')   → edgeId
manageLayerActors(layerId="<layerId>", body='[{"action":"create","data":{"id":"<edgeId>","type":"edge","laIdSource":<laA>,"laIdTarget":<laB>}}]')

existLink(body='{"source":"<a>","target":"<b>"}')
updateLink(edgeId="<edgeId>", body='{"data":{"weight":5}}')
deleteLink(edgeId="<edgeId>")
bulkDeleteLinks(body='{"edgeIds":["<e1>","<e2>"]}')
```

### Layer Operations

```
getLayer(layerId="<layerId>")            // read full layer state with laIds
searchLayerActors(layerId="<layerId>", query="...")
getLayerActorsByFormId(layerId="<layerId>", formId=<formId>)
cleanLayer(layerId="<layerId>")          // remove all from view (actors remain)
moveElements(sourceLayerId="<la>", targetLayerId="<lb>", body='{"actorIds":["<a1>"]}')
```

### Graph Traversal

```
getRelatedActors(type="linked", actorId="<actorId>")  // type: "linked"|"parents"|"children"; defaults to the hierarchy link type
getActorLinks(actorId="<actorId>")
getLinkedActors(actorId="<actorId>")
actorGlobalLayers(actorId="<actorId>")

// Related actors filtered/ranked by an account balance, in one query:
// "the actors related to X whose account N balance is > / < a value".
filterActors(formId=<formId>, linkedToActorId="<anchorActorId>",
             accountNameId="<nameId>", currencyId=<id>,
             amountFrom=<min>, amountTo=<max>,        // amountFrom = balance >=, amountTo = balance <=
             orderBy="balance", orderValue="DESC")    // omit linkedToActorId for a form-wide ranking

// Save tokens: every read/traversal tool above (getRelatedActors, getActor,
// getLayerActors, searchLayerActors, searchActors, filterActors, ...) accepts an
// optional `filter` field-selection arg — comma-separated fields to return, e.g.
// filter="id,title,formId" (dotted paths like data.status pick nested data fields).
// The server returns only those fields. NOT a row filter (that's search / q).
getRelatedActors(type="children", actorId="<actorId>", filter="id,title,formId")
```

---

## Financial Operations

### Accounts

```
getAccounts(actorId="<actor>")
getAccount(accountId="<acc>")
createAccounts(actorId="<actor>", body='{"nameId":"<name>","currencyId":"<cur>","accountType":"default"}')
postAccounts(body='{"accountName":"Balance","currencyName":"USD"}')
setAmount(accountId="<acc>", body='{"amount":1000}')
delAccounts(actorId="<actor>", currencyId="<cur>", nameId="<name>", accountType="default")
```

### Currencies & Account Names

```
getCurrencies()
createCurrency(body='{"name":"USD"}')
getAccountNames()
createAccountName(body='{"name":"Balance","abbreviation":"BAL"}')
```

---

## Key Rules

- **File-based workflow is preferred** for building and editing graphs — write a YAML file, then `pushGraphFile`.
- **Create Graph + Layer first**, then work with the layer's YAML file. Never skip the graph→layer link.
- **`formName` takes priority over `formId`** in the YAML file and in `createActor` calls.
- **Do NOT include `data`** in the YAML for system forms — the server auto-injects shape/view/blockId.
- **For custom forms** (formId specified by user or formName not in the catalog): call `getForm(formId)` first, then build `actors.data` from the returned field schema using field `name` as keys.
- **Always set `color`** (hex string) for flowchart blocks.
- **Local `id` values** in the YAML are replaced with server UUIDs after `pushGraphFile`. Use short readable names (
  `start`, `validate`, `end`) for new actors.
- **`pullGraphFile` → edit → `pushGraphFile`** is the standard edit cycle for existing layers.
- `laId` ≠ `actorId`. The file workflow handles laId management automatically.
- Space actors ~200–300 px apart; use the layout algorithm for coordinates.

---

## Reference Documents

| Path                                                            | When to read                                   |
|-----------------------------------------------------------------|------------------------------------------------|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/actors.md`                   | Full actor property list and types             |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/links.md`                    | Link/edge properties and type system           |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/layers.md`                   | Layer types and behavior                       |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/graph-functionality.md`    | Graph building walkthrough with test scenarios |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/actor-graph-management.md` | Managing actors on graphs — practical patterns |
