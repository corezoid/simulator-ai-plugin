# Links

Links in the Simulator.Company platform represent connections between actors, forming a graph structure that models relationships between entities.

## Overview

Links connect actors to each other, creating a graph-like structure. Each link has a specific type that defines the nature of the relationship between the connected actors.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the link |
| acc_id | String | Workspace ID the link belongs to |
| source_id | String | ID of the source actor |
| target_id | String | ID of the target actor |
| type_id | Integer | ID of the link type |
| data | JSON | Custom data associated with the link |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Link Types

Link types define the nature of relationships between actors. Each link type has its own properties:

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the link type |
| acc_id | String | Workspace ID the link type belongs to |
| name | Text | Name of the link type |
| user_id | Integer | ID of the user who created the link type |
| is_system | Boolean | Whether this is a system-defined link type |
| is_tree | Boolean | Whether the link represents a hierarchical relationship |
| created_at | Integer | Unix timestamp of creation time |

## System Link Types

The platform includes several system-defined link types:

- **Hierarchy** - Parent-child relationships between actors
- **Reference** - Simple references between actors
- **Transfer** - Financial transfers between actors
- **Process** - Process flow connections
- **Dependency** - Dependency relationships

## API Endpoints

For detailed API documentation on links, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Links API Documentation](https://doc.simulator.company/#tag/links)
[Link Types API Documentation](https://doc.simulator.company/#tag/link-types)
[Graph API Documentation](https://doc.simulator.company/#tag/graph)

The API provides endpoints for:

- Getting linked actors with their link information
- Retrieving all links for a specific actor
- Checking if links exist between actors
- Creating new links between actors
- Updating existing links
- Deleting links
- Managing link types (creating, retrieving, updating, deleting)
- Graph traversal operations

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for read operations and `control.events:actors.management` for write operations).

### MCP tools (curated)

The link (edge) lifecycle is fully covered by curated MCP tools:

| Operation | Tool | Notes |
|---|---|---|
| Create one | `createLink(accId, source, target, edgeTypeId, name?, weight?, curveStyle?, linkedActorId?, pinned?, hole?, forceDirection?)` | directed edge between two actors; `hole:true` creates a placeholder link |
| Create many | `massLink(accId, links[], forceDirection?)` | up to 50 edge objects in one call |
| Read one | `getEdge(edgeId, linkedActor?)` | a single edge by UUID, with source/target + privileges |
| Update | `updateEdge(edgeId, name?, linkedActorId?, curveStyle?, pinned?)` | partial — only provided fields change |
| Delete by id | `deleteEdge(edgeId)` | irreversible; permanent/system edge types (e.g. the hierarchy link) and form-field edges can't be deleted |
| Delete by endpoints | `deleteEdgesByNodes(links[], force?)` | bulk (1-200) delete by `(source, target, edgeTypeId)`; per-item `bidirected` also removes the reverse edge |
| Exists / lookup | `existLink(source, target, edgeTypeId, bidirected?)` | returns matching edge(s) — use to dedupe before `createLink` or to find an edge id by endpoints |
| List an actor's links | `getRelatedActors(type, actorId, …)` | `type` = linked / parents / children; traverses the hierarchy edge type by default |
| Edge types | `getEdgeTypes(accId, isSystem?)` | including the hierarchy type used by `getRelatedActors` |

> The hierarchy edge type cannot be created or deleted via these tools — it is permanent and
> managed by the platform (use it through `getRelatedActors` / `createActor` parent-child flows).

#### Hole (placeholder) links

An edge can be a **hole** — a placeholder link (`hole: true` on `createLink`, stored as
`actors_edges.hole`). It renders as a dashed placeholder on a layer. Closing a hole is a
**layer-level placement swap** (the exact edge analogue of closing an actor hole): the hole edge's
`layer_to_edges` placement is replaced by a real "closer" edge's placement on that layer, recorded
in `closed_edge_holes`. The hole edge itself is **never modified or deleted** (its `hole` flag stays
`true`) — both edges persist, only the layer view changes. A hole is closed either:

- **automatically** when a financial **transfer** runs between the two actors a hole connects — the
  transfers-amount edge is created, and a pre-drawn **hierarchy** hole is closed by a real closer
  hierarchy edge, so the two actors end up with two links on the layer; or
- **manually** via `POST /papi/1.0/actors/merge_edge_hole/:accId` (body: `holeEdgeId`,
  `targetEdgeId`, `layerId`) — the user-initiated analogue of `merge_hole` for actors.

`POST /papi/1.0/actors/revert_edge_hole/:accId` (body: `targetEdgeId`, optional `layerId` /
`holeEdgeId`) re-opens the holes a closer edge closed, restoring the hole's layer placement. (These
merge/revert routes have no operationId and are not curated MCP tools — same as the actor-hole
`merge_hole`/`revert_hole` routes.)

## Database Structure

Links are stored in the `actors_edges` table with the following structure:

- Unique index on edge_type_id, source, target, and linked_actor_id
- Foreign key relationships to actors and edge types
- Optimized for graph traversal queries

Link types are stored in the `edges_types` table with:
- Unique index on acc_id and name
- Support for system-defined and user-defined types
- Special flags for hierarchical relationships

## Example

### Link

```json
{
  "id": "link_123456",
  "source_id": "actor_123",
  "target_id": "actor_456",
  "type_id": 42,
  "data": {
    "weight": 0.75,
    "notes": "Primary connection"
  },
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Link Type

```json
{
  "id": 42,
  "name": "Hierarchy",
  "acc_id": "workspace_789",
  "user_id": 123,
  "is_system": true,
  "is_tree": true,
  "created_at": 1621459200
}
```

## Graph Traversal

The link system supports efficient graph traversal operations:

- **Depth-First Search** - Traverse the graph depth-first
- **Breadth-First Search** - Traverse the graph breadth-first
- **Path Finding** - Find paths between actors
- **Cycle Detection** - Detect cycles in the graph

These operations are essential for process flow analysis, dependency checking, and hierarchical data visualization.
