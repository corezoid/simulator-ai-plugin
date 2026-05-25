# Graph Functionality

This document describes how to use the graph functionality in the Simulator.Company platform based on test scenarios from the control-api-qa repository. It focuses on practical examples of using the public API to manage actors, links, and layers in graph structures.

## Overview

The Simulator.Company platform uses a graph-based approach to represent business processes and relationships between entities. The platform provides API endpoints for:

- Managing actors (nodes in the graph)
- Creating and managing links between actors
- Organizing actors on layers
- Searching and filtering actors on layers

All examples in this document are based on actual test scenarios from the control-api-qa repository, ensuring they represent real-world usage patterns.

## Prerequisites

Before using the graph functionality API endpoints, you need:

1. Authentication token with appropriate scopes:
   - `control.events:actors.readonly` for read operations
   - `control.events:actors.management` for write operations
   - `control.events:forms.readonly` for form operations

2. Knowledge of the system forms used for actors, particularly:
   - Layers
   - Graphs
   - Events

## Key User Flows

The following user flows demonstrate how to use the graph functionality in real-world situations, based on test scenarios from the control-api-qa repository.

### Actor Management

The platform provides endpoints for creating, retrieving, updating, and deleting actors. For detailed API documentation, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

Key operations include:
- Creating actors with specific form types
- Retrieving actors by ID or reference
- Updating actor properties
- Deleting actors

For detailed information about actors, see [Actors](../entities/actors.md).

### Link Management

Links connect actors to form graph structures. The platform provides endpoints for creating, retrieving, updating, and deleting links between actors.

Key operations include:
- Creating links between actors
- Creating multiple links at once
- Updating link properties
- Deleting links
- Retrieving an actor's global links

For detailed information about links, see [Links](../entities/links.md).

### Layer Management

Layers provide visual organization for actors in graphs. The platform provides endpoints for managing actors on layers and searching/filtering them.

Key operations include:
- Adding actors to layers
- Searching actors on layers by title
- Searching actors on layers by form ID

For detailed information about layers, see [Layers](../entities/layers.md).

## Test Scenarios

The following test scenarios from the control-api-qa repository demonstrate how to use the graph functionality in practical applications.

### Creating a Business Process Graph

This scenario demonstrates how to create a complete business process graph with multiple actors and links:

1. Create a graph actor
2. Create a layer actor
3. Link the layer to the graph
4. Create process step actors
5. Link the process steps
6. Add the process steps to the layer

These operations use the standard actor, link, and layer endpoints documented in the [Simulator.Company API Documentation](https://doc.simulator.company).

## API Reference

For detailed API documentation, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Simulator.Company API Documentation](https://doc.simulator.company)

## Related Documentation

- [Actors](../entities/actors.md) - Core entity representing nodes in business process graph
- [Forms](../entities/forms.md) - Reusable data structure templates for actors
- [Links](../entities/links.md) - Connections between actors forming graph structures
- [Layers](../entities/layers.md) - Visual organization of actors and edges
- [System Forms](../entities/system-forms.md) - Predefined form templates for system functionality

## Authentication and Authorization

All API requests require OAuth2 authentication. The specific scopes required for each endpoint are documented in the official API documentation.

Common scopes used in these user flows include:

- `control.events:actors.readonly` - Read-only access to actors
- `control.events:actors.management` - Create, update, and delete actors
- `control.events:forms.readonly` - Read-only access to forms
- `control.events:forms.management` - Create, update, and delete forms

## Conclusion

The graph functionality in the Simulator.Company platform provides a powerful way to create and manage complex business processes. By using actors, links, and layers, you can create rich, interactive representations of your business processes and data.

The test scenarios in this document demonstrate how to use the public API to manage graph structures in real-world situations, based on actual test cases from the control-api-qa repository.
