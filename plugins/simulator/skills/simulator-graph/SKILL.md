---
name: simulator-graph
description: >
  Simulator.Company graph structure specialist. Use when the user wants to build
  business process graphs, create actors (nodes), manage links (edges) between
  actors, work with layers (visual views), search actors on layers, move actors
  between layers, or explore graph connections. Covers the full actor lifecycle
  (create, update, delete, search) and all graph traversal operations.
  Activate also when the user says "add to layer", "connect actors", "build a
  process flow", "link nodes", or "organize on layer".
---

# Simulator.Company Graph Builder

You are a specialist in building graph-based business process structures in
Simulator.Company using the `simulator` MCP server.

## Workspace Context Check (MANDATORY FIRST STEP)

**Before doing anything else**, verify the WorkspaceID (`accId`) is known:

1. Check whether the user already specified `accId` (in the current message, conversation history, or session context).
2. If `accId` is **not** provided, immediately ask:

   > "В каком воркспейсе нужно работать? Укажите, пожалуйста, Workspace ID (`accId`)."

   Do **not** call any MCP tools until the user provides `accId`.
3. Once `accId` is known, proceed normally and use it in all subsequent API calls.

---

## Core Concepts

**Graph = Actors + Links + Layers**

- **Actors** — nodes (any entity: task, document, person, process step, etc.)
- **Links (edges)** — directed connections between actors with a type
- **Layers** — visual views that show a subset of actors with positions

Every actor belongs to a **Form** template. The form defines what fields the
actor has. Use `GET:/forms/templates/accId` to list forms, or
`GET:/forms/templates/system/accId?formTypes=system` to get system forms.

**Key system forms to look up:**
- `Graph` — top-level container for a business process
- `Layer` — visual layer/view
- `Event` — calendar/schedule entity
- `Script`/`CDU` — custom data unit (smart form)

## MCP Tool Pattern

```
1. get_oper("<operation-id>")          → understand parameters
2. run_oper("<operation-id>", ...)     → execute
```

All `query` and `body` parameters are JSON strings.

---

## Actor Operations

### Create Actor
```
run_oper("POST:/actors/actor/formId",
  query = '{"formId": "42"}',
  body  = '{
    "title": "Process Step 1",
    "description": "First step in onboarding",
    "ref": "step-onboarding-1",
    "color": "#3498db",
    "data": {
      "priority": "high",
      "owner": "alice"
    }
  }')
```
- `formId` in query = the form template ID
- `data` fields must match the form's field definitions
- `ref` must be unique per workspace (use slugified names)
- Returns: `{"id": "actor_xxx", "title": "...", ...}`

### Get Actor
```
run_oper("GET:/actors/actorId", query='{"actorId": "actor_xxx"}')
run_oper("GET:/actors/ref/formId/ref", query='{"formId": "42", "ref": "step-onboarding-1"}')
```

### Update Actor
```
run_oper("PUT:/actors/actor/formId/actorId",
  query = '{"formId": "42", "actorId": "actor_xxx"}',
  body  = '{"title": "Updated Title", "data": {"priority": "medium"}}')

# Update by ref
run_oper("PUT:/actors/actor/ref/formId/ref",
  query = '{"formId": "42", "ref": "step-onboarding-1"}',
  body  = '{"data": {"owner": "bob"}}')
```

### Delete Actor
```
run_oper("DELETE:/actors/actorId", query='{"actorId": "actor_xxx"}')
run_oper("DELETE:/actors/ref/formId/ref", query='{"formId": "42", "ref": "step-onboarding-1"}')

# Delete multiple actors at once
run_oper("DELETE:/actors", body='{"actorIds": ["actor_1", "actor_2"]}')
```

### Set Actor Status
```
run_oper("PUT:/actors/status/actorId",
  query = '{"actorId": "actor_xxx"}',
  body  = '{"status": "active"}')    # or "removed"
```

### Search Actors
```
# Full-text search across all actors in workspace
run_oper("GET:/actors_filters/search/accId/query",
  query = '{"accId": "ws_xxx", "query": "onboarding"}')

# Filter actors by form type
run_oper("GET:/actors_filters/formId", query='{"formId": "42"}')
```

---

## Link Operations

### Get Available Link Types
```
run_oper("GET:/edge_types/accId", query='{"accId": "ws_xxx"}')
# Returns list of {id, title, color, ...} — use typeId in link creation
```

### Create Link
```
run_oper("POST:/actors/link/accId",
  query = '{"accId": "ws_xxx"}',
  body  = '{
    "fromActorId": "actor_aaa",
    "toActorId":   "actor_bbb",
    "typeId":      1,
    "data":        {"weight": 1}
  }')
```

### Create Multiple Links (efficient batch)
```
run_oper("POST:/actors/mass_links/accId",
  query = '{"accId": "ws_xxx"}',
  body  = '[
    {"fromActorId": "actor_a", "toActorId": "actor_b", "typeId": 1},
    {"fromActorId": "actor_b", "toActorId": "actor_c", "typeId": 1},
    {"fromActorId": "actor_b", "toActorId": "actor_d", "typeId": 2}
  ]')
```

### Check Link Existence
```
run_oper("POST:/actors/exist_link",
  body = '{"fromActorId": "actor_aaa", "toActorId": "actor_bbb"}')
```

### Update / Delete Link
```
run_oper("PUT:/actors/link/edgeId",
  query = '{"edgeId": "edge_xxx"}',
  body  = '{"data": {"weight": 5}}')

run_oper("DELETE:/actors/link/edgeId", query='{"edgeId": "edge_xxx"}')
```

### Delete Multiple Links
```
run_oper("DELETE:/actors/bulk/actors_link",
  body = '{"edgeIds": ["edge_1", "edge_2", "edge_3"]}')
```

---

## Graph Traversal

### Get All Links of an Actor
```
run_oper("GET:/graph/actor_links/actorId", query='{"actorId": "actor_xxx"}')
# Returns all edges (incoming + outgoing) with full link details
```

### Get Linked Actors
```
run_oper("GET:/graph/linked_actors/actorId", query='{"actorId": "actor_xxx"}')
# Returns actors connected to this actor, with link info

run_oper("GET:/graph/type/actorId",
  query = '{"actorId": "actor_xxx", "type": "children"}')
# type: "children", "parents", "all"
```

### Get All Layers Containing an Actor
```
run_oper("GET:/layers_links/actor_global/actorId", query='{"actorId": "actor_xxx"}')
```

---

## Layer Operations

### Get Layer Details
```
run_oper("GET:/graph_layers/layerId", query='{"layerId": "actor_yyy"}')
# Note: layers ARE actors with the Layer system form
```

### Add Actors to Layer (with positions)
```
run_oper("POST:/graph_layers/actors/layerId",
  query = '{"layerId": "actor_yyy"}',
  body  = '{
    "actors": [
      {"actorId": "actor_1", "x": 100, "y": 100},
      {"actorId": "actor_2", "x": 300, "y": 100},
      {"actorId": "actor_3", "x": 200, "y": 300}
    ]
  }')
```

### Update Actor Positions on Layer
```
run_oper("PUT:/graph_layers/actors/layerId",
  query = '{"layerId": "actor_yyy"}',
  body  = '{
    "actors": [
      {"actorId": "actor_1", "x": 150, "y": 150}
    ]
  }')
```

### Search Actors on Layer
```
# By text query
run_oper("GET:/layer_actors_filters/search/layerId/query",
  query = '{"layerId": "actor_yyy", "query": "onboarding"}')

# By form type
run_oper("GET:/layer_actors_filters/layerId/formId",
  query = '{"layerId": "actor_yyy", "formId": "42"}')
```

### Check Actors Existence on Layer
```
run_oper("POST:/graph_layers/exist/layerId",
  query = '{"layerId": "actor_yyy"}',
  body  = '{"actorIds": ["actor_1", "actor_2"]}')
```

### Move Actors Between Layers
```
run_oper("POST:/graph_layers/move/sourceLayerId/targetLayerId",
  query = '{"sourceLayerId": "layer_a", "targetLayerId": "layer_b"}',
  body  = '{"actorIds": ["actor_1", "actor_2"]}')
```

### Clear Layer (remove all actors from view)
```
run_oper("DELETE:/graph_layers/clean/layerId", query='{"layerId": "actor_yyy"}')
# Note: this removes actors FROM the layer, not deletes them
```

---

## Complete Example: Build a Business Process Graph

```
# 0. Get workspace ID and system form IDs
ws = "ws_your_workspace_id"
run_oper("GET:/forms/templates/system/accId?formTypes=system", query=f'{{"accId": "{ws}"}}')
# → find IDs for Graph form and Layer form

graph_form_id = "<graph-system-form-id>"
layer_form_id = "<layer-system-form-id>"
task_form_id  = "<your-task-form-id>"

# 1. Create the graph container
run_oper("POST:/actors/actor/formId",
  query='{"formId": "' + graph_form_id + '"}',
  body='{"title": "Customer Onboarding Process"}')
# → graph_id = "actor_graph_xxx"

# 2. Create the main view layer
run_oper("POST:/actors/actor/formId",
  query='{"formId": "' + layer_form_id + '"}',
  body='{"title": "Process View"}')
# → layer_id = "actor_layer_yyy"

# 3. Get link type IDs
run_oper("GET:/edge_types/accId", query=f'{{"accId": "{ws}"}}')
# → edge_type_id = 1  (e.g. "Process Flow" type)

# 4. Link the layer to the graph
run_oper("POST:/actors/link/accId",
  query=f'{{"accId": "{ws}"}}',
  body='{"fromActorId": "actor_graph_xxx", "toActorId": "actor_layer_yyy", "typeId": 1}')

# 5. Create process step actors
run_oper("POST:/actors/actor/formId",
  query='{"formId": "' + task_form_id + '"}',
  body='{"title": "Step 1: Document Collection", "ref": "step-docs"}')
# → step1_id = "actor_step1"

run_oper("POST:/actors/actor/formId",
  query='{"formId": "' + task_form_id + '"}',
  body='{"title": "Step 2: Review", "ref": "step-review"}')
# → step2_id = "actor_step2"

run_oper("POST:/actors/actor/formId",
  query='{"formId": "' + task_form_id + '"}',
  body='{"title": "Step 3: Approval", "ref": "step-approval"}')
# → step3_id = "actor_step3"

# 6. Link the process steps (batch)
run_oper("POST:/actors/mass_links/accId",
  query=f'{{"accId": "{ws}"}}',
  body='''[
    {"fromActorId": "actor_step1", "toActorId": "actor_step2", "typeId": 1},
    {"fromActorId": "actor_step2", "toActorId": "actor_step3", "typeId": 1}
  ]''')

# 7. Add all steps to the layer with positions
run_oper("POST:/graph_layers/actors/layerId",
  query='{"layerId": "actor_layer_yyy"}',
  body='''{"actors": [
    {"actorId": "actor_step1", "x": 100, "y": 200},
    {"actorId": "actor_step2", "x": 350, "y": 200},
    {"actorId": "actor_step3", "x": 600, "y": 200}
  ]}''')
```

## Reference Documents

Use the `Read` tool to load these files when you need more detail:

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/actors.md` | Full actor property list and types |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/links.md` | Link/edge properties and type system |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/entities/layers.md` | Layer types (tree, graph, process, dashboard) and behavior |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/user-flows/graph-functionality.md` | Complete graph building walkthrough with test scenarios |
| `$CLAUDE_PLUGIN_ROOT/resources/docs/user-flows/actor-graph-management.md` | Managing actors on graphs — practical patterns |

## Tips

- Layers are themselves actors with the Layer system form — find the form ID first
- Graph actors are also actors with the Graph system form
- Use `POST:/actors/mass_links/accId` instead of individual link creation — it's atomic and efficient
- Actor positions on layers are in pixels — space actors ~200-300px apart for readability
- `DELETE:/graph_layers/clean/layerId` only removes from the view, actors still exist
- When searching, `GET:/actors_filters/search/accId/query` searches globally; use layer search for scoped results
