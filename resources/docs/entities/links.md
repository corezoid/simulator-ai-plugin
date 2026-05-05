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
