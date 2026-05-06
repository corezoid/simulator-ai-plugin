# Layers

Layers in the Simulator.Company platform provide a way to organize and visualize actors and their connections in different contexts or views.

## Overview

Layers serve as visual containers for actors and edges, allowing users to create multiple views of the same underlying data. Each layer can show a different subset of actors and connections, enabling focused views for specific business processes or organizational units.

## Properties

### Layer Actor

Layers are represented as special actors with the following properties:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the layer actor |
| acc_id | String | Workspace ID the layer belongs to |
| user_id | Integer | ID of the user who created the layer |
| form_id | Integer | ID of the layer form template (system form) |
| title | Text | Display title of the layer |
| description | Text | Detailed description of the layer's purpose |
| data | JSON | Layer configuration data |
| color | String | Color associated with the layer (hex code) |
| picture | Text | URL or path to the layer's image |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Layer to Actors

The LayerToActors relationship tracks which actors appear on which layers:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the layer-actor relationship |
| layer_id | String | ID of the layer |
| actor_id | String | ID of the actor placed on the layer |
| la_id | String | Layer-specific actor ID |
| x | Float | X-coordinate position of the actor on the layer |
| y | Float | Y-coordinate position of the actor on the layer |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Layer to Edges

The LayerToEdges relationship tracks which edges (connections between actors) appear on which layers:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the layer-edge relationship |
| layer_id | String | ID of the layer |
| edge_id | String | ID of the edge placed on the layer |
| la_id_source | String | Layer-specific ID of the source actor |
| la_id_target | String | Layer-specific ID of the target actor |
| le_id | String | Layer-specific edge ID |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Layer Types

The platform supports different layer types:

- **Graph Layers** - Standard visualization with free positioning of actors
- **Tree Layers** - Hierarchical visualization with automatic layout
- **Process Layers** - Specialized for business process visualization
- **Dashboard Layers** - For data visualization and reporting

## API Endpoints

For detailed API documentation on layers, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Layers API Documentation](https://doc.simulator.company/#tag/layers)

The API provides endpoints for:

- Getting all layers in a workspace
- Retrieving specific layer details
- Creating new layers
- Updating existing layers
- Deleting layers
- Managing actors on layers (adding, positioning, removing)
- Retrieving all actors on a specific layer

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for read operations and `control.events:actors.management` for write operations).

## Database Structure

Layers use multiple tables to track relationships:

- Layers are stored as special actors in the `actors` table
- Actor placements are stored in the `layer_to_actors` table
- Edge placements are stored in the `layer_to_edges` table
- Coordinates are tracked in ScyllaDB for real-time collaboration

## Example

### Layer Definition

```json
{
  "id": "layer_123456",
  "title": "Sales Process",
  "description": "Visualization of the sales pipeline",
  "data": {
    "type": "graph",
    "edgeCurveStyle": "curved",
    "initX": 0,
    "initY": 0,
    "defForm": 42
  },
  "color": "#3498db",
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Layer with Actors and Edges

```json
{
  "id": "layer_123456",
  "title": "Sales Process",
  "data": { ... },
  "actors": [
    {
      "id": "actor_789012",
      "la_id": "la_345678",
      "title": "Lead Generation",
      "x": 100,
      "y": 200
    },
    {
      "id": "actor_901234",
      "la_id": "la_456789",
      "title": "Qualification",
      "x": 300,
      "y": 200
    }
  ],
  "edges": [
    {
      "id": "edge_567890",
      "le_id": "le_123456",
      "source_la_id": "la_345678",
      "target_la_id": "la_456789",
      "type_id": 1,
      "type_name": "Process Flow"
    }
  ]
}
```

## Real-time Collaboration

Layers support real-time collaboration through:

- Coordinate tracking in ScyllaDB
- Real-time updates via WebSocket
- Conflict resolution for simultaneous edits
- User presence indicators

## Usage in the Platform

Layers are used throughout the platform for various purposes:

- **Process Visualization** - Visualizing business processes and workflows
- **Organizational Structure** - Representing hierarchical relationships
- **Project Management** - Tracking tasks and dependencies
- **Financial Flows** - Visualizing financial relationships and transfers
- **Custom Views** - Creating specialized views for different user roles

Layers provide a flexible way to visualize and interact with the underlying graph structure, enabling users to create focused views for specific business needs.
