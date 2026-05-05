# Actor Graph Management

This document describes the user flows for managing actors on graphs in the Simulator.Company platform using the public API. It covers creating actors, linking them together, organizing them on layers, and searching/filtering them.

## Overview

The Simulator.Company platform uses a graph-based approach to represent business processes and relationships between entities. Actors are the nodes in these graphs, and they can be connected by links to form complex structures. Layers provide visual organization for actors, allowing users to create meaningful representations of their business processes.

This document focuses on the user flows for managing actors on graphs, including:

- Creating actors with different system forms
- Linking actors together to form graph structures
- Adding actors to layers for visual organization
- Searching and filtering actors on layers
- Managing complex graph structures

## Prerequisites

Before using the actor graph management API endpoints, you need:

1. Authentication token with appropriate scopes:
   - `control.events:actors.readonly` for read operations
   - `control.events:actors.management` for write operations
   - `control.events:forms.readonly` for form operations

2. Knowledge of the system forms used for actors:
   - Events
   - Graphs
   - Layers
   - Streams
   - Scripts/Smart Forms/CDU
   - Reactions
   - And others as needed for your specific use case

## Actor Management

The platform provides endpoints for creating, retrieving, updating, and deleting actors. For detailed API documentation, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

Key operations include:
- Creating actors with specific form types
- Retrieving actors by ID or reference
- Updating actor properties
- Deleting actors

For detailed information about actors, see [Actors](../entities/actors.md).

## Link Management

Links connect actors together to form graph structures. The platform provides endpoints for creating, retrieving, updating, and deleting links between actors.

Key operations include:
- Creating links between actors
- Creating multiple links at once
- Updating link properties
- Deleting links
- Retrieving an actor's global links

For detailed information about links, see [Links](../entities/links.md).

## Layer Management

Layers provide visual organization for actors, allowing users to create meaningful representations of their business processes.

Key operations include:
- Adding actors to layers
- Searching actors on layers by title
- Searching actors on layers by form ID

For detailed information about layers, see [Layers](../entities/layers.md).

## Complete User Flows

The following user flows demonstrate how to use the actor graph management functionality in real-world situations.

### Creating a Business Process Graph

This flow demonstrates how to create a business process graph with multiple actors and links:

1. Create a Graph Actor
2. Create a Layer Actor
3. Link the Layer to the Graph
4. Create Process Step Actors
5. Link the Process Steps
6. Add the Process Steps to the Layer

These operations use the standard actor, link, and layer endpoints documented in the [Simulator.Company API Documentation](https://doc.simulator.company).

### Managing Financial Tracking in a Graph

This flow demonstrates how to create a financial tracking system using actors, accounts, and transactions:

1. Create an Account Actor
2. Create Transaction Actors
3. Link Transactions to the Account

For detailed information about accounts and transactions, see:
- [Accounts](../entities/accounts.md)
- [Transactions](../entities/transactions.md)

### Building Interactive Forms with Scripts/Smart Forms/CDU

This flow demonstrates how to create and manage interactive forms using the Scripts/Smart Forms/CDU system:

1. Create a Script/Smart Form Actor
2. Create a Customer Actor using the Form
3. Link the Customer to the Form

For detailed information about forms, see [Forms](../entities/forms.md).

### Managing User Collaboration with Reactions

This flow demonstrates how to create and manage user reactions to actors:

1. Create a Reaction Actor
2. Link the Reaction to an Actor

For detailed information about reactions, see [Reactions](../entities/reactions.md).

## API Reference

For detailed API documentation, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Simulator.Company API Documentation](https://doc.simulator.company)

## Related Documentation

- [Actors](../entities/actors.md) - Core entity representing nodes in business process graph
- [Forms](../entities/forms.md) - Reusable data structure templates for actors
- [Links](../entities/links.md) - Connections between actors forming graph structures
- [Layers](../entities/layers.md) - Visual organization of actors and edges
- [Accounts](../entities/accounts.md) - Financial tracking for actors
- [Transactions](../entities/transactions.md) - Financial operations within accounts
- [Reactions](../entities/reactions.md) - User interactions and comments
- [System Forms](../entities/system-forms.md) - Predefined form templates for system functionality

## Authentication and Authorization

All API requests require OAuth2 authentication. The specific scopes required for each endpoint are documented in the official API documentation.

Common scopes used in these user flows include:

- `control.events:actors.readonly` - Read-only access to actors
- `control.events:actors.management` - Create, update, and delete actors
- `control.events:forms.readonly` - Read-only access to forms
- `control.events:forms.management` - Create, update, and delete forms

## Conclusion

The actor graph management functionality in the Simulator.Company platform provides a powerful way to create and manage complex business processes. By using actors, links, and layers, you can create rich, interactive representations of your business processes and data.
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

The actor graph management API provides a powerful way to create and manage complex business processes in the Simulator.Company platform. By using actors, links, and layers, you can create rich, interactive representations of your business processes and data.
