# Simulator.Company Entity Relationships

This document provides an overview of the entity relationships in the Simulator.Company platform, illustrating how different components interact to form the complete system architecture.

## System Architecture Overview

The Simulator.Company platform is built around a core set of entities that work together to provide a comprehensive business process management and financial tracking system:

1. **Workspaces** - Multi-tenant containers for organizations
2. **Actors** - Core entities representing nodes in business process graphs
3. **Forms** - Reusable data structure templates for actors
4. **Links** - Connections between actors forming graph structures
5. **Accounts** - Financial tracking for actors
6. **Transactions & Transfers** - Financial operations between accounts
7. **Access Control** - Permission system for object-level access
8. **System Forms** - Predefined templates for system functionality

The platform uses a combination of PostgreSQL and ScyllaDB for data storage, with PostgreSQL handling transactional data and entity relationships, while ScyllaDB manages high-performance counters, metrics, and real-time data.

## Core Entity Relationships

### Workspace Relationships

Workspaces (Accounts) serve as the top-level container for all other entities:

- Workspaces → Forms: One-to-many relationship
- Workspaces → AccessRules: One-to-many relationship
- Workspaces → Webhooks: One-to-many relationship
- Workspaces → AccountCurrencies: One-to-many relationship
- Workspaces → AccountNames: One-to-many relationship
- Workspaces → EdgesTypes: One-to-many relationship
- Workspaces → AsyncTasks: One-to-many relationship
- Workspaces → RecentObjs: One-to-many relationship
- Workspaces → UserSettings: One-to-many relationship
- Workspaces → Invites: One-to-many relationship

### Actor Relationships

Actors are the central entity in the system, representing nodes in the business process graph:

- Actors → Forms: Many-to-one relationship (each actor uses a form template)
- Actors → ActorsEdges: One-to-many as source and target (graph connections)
- Actors → ActorsAccounts: One-to-many (financial accounts)
- Actors → LayerToActors: One-to-many (placement on visualization layers)
- Actors → ActorToAttachments: One-to-many (file attachments)
- Actors → ActorsTreeEdges: Multiple relationships for hierarchical structures
- Actors → AccountToActors: One-to-many (account associations)
- Actors → ActorToForms: One-to-many (form associations)
- Actors → AppEnvs: One-to-many (application environments)
- Actors → AuthorizedActors: One-to-one (authentication)
- Actors → ConnectorsToAccounts: One-to-many (external integrations)

### Form Relationships

Forms define the structure and behavior of actors:

- Forms → Actors: One-to-many (actors based on form templates)
- Forms → FormAccounts: One-to-many (account definitions for forms)
- Forms → ActorToForms: One-to-many (form associations)
- Forms → Forms: One-to-many (parent-child relationships)

### Financial Relationships

The financial system consists of accounts, transactions, and transfers:

- AccountNames → ActorsAccounts: One-to-many (account categorization)
- AccountCurrencies → ActorsAccounts: One-to-many (currency definition)
- ActorsAccounts → TransactionsUniqueRef: One-to-many (transaction tracking)
- ActorsAccounts → ConnectorsToAccounts: One-to-many (external integrations)
- ActorsAccounts → AccountFormulas: Multiple relationships for formula calculations

### Graph Structure Relationships

The graph structure is defined by actors, edges, and layers:

- EdgesTypes → ActorsEdges: One-to-many (edge type definition)
- ActorsEdges → LayerToEdges: One-to-many (edge placement on layers)
- ActorsEdges → ActorsEdgesOrder: One-to-many (edge ordering)
- LayerToActors → LayerToEdges: Multiple relationships for layer connections

### Application Relationships

The application system manages environments, folders, and files:

- Actors → AppEnvs: One-to-many (application environments)
- AppEnvs → AppFolders: One-to-many (folder structure)
- AppFolders → AppFolders: One-to-many (nested folders)
- AppFolders → AppFiles: One-to-many (file storage)

## Entity Relationship Diagram

```
Workspaces
  ├── Forms
  │     ├── Actors
  │     │     ├── ActorsEdges
  │     │     ├── ActorsAccounts
  │     │     │     ├── Transactions
  │     │     │     ├── Transfers
  │     │     │     └── AccountFormulas
  │     │     ├── LayerToActors
  │     │     ├── Attachments
  │     │     ├── ActorsTreeEdges
  │     │     └── AppEnvs
  │     └── FormAccounts
  ├── AccessRules
  ├── AccountNames
  ├── AccountCurrencies
  ├── EdgesTypes
  ├── Webhooks
  ├── AsyncTasks
  ├── RecentObjs
  └── UserSettings
```

## Cross-References to Entity Documentation

### Core Entities

- [Actors](./actors.md) - Core entity representing nodes in business process graph
- [Forms](./forms.md) - Reusable data structure templates for actors
- [Links](./links.md) - Connections between actors forming graph structures
- [Layers](./layers.md) - Visual organization of actors and edges

### Financial Entities

- [Accounts](./accounts.md) - Financial tracking for actors
- [Transactions](./transactions.md) - Financial operations within accounts
- [Transfers](./transfers.md) - Movement of funds between accounts
- [Balances](./balances.md) - Account balance tracking

### System Entities

- [System Forms](./system-forms.md) - Predefined templates for system functionality
- [Counters](./counters.md) - High-performance metrics tracking
- [System Data](./system-data.md) - Supporting data structures for platform functionality
- [Search](./search.md) - Full-text search capabilities
- [History](./history.md) - Audit trail of changes to objects

### Additional Entities

- [Reactions](./reactions.md) - User interactions on actors
- [Attachments](./attachments.md) - File storage system for actors

## Database Technologies

The Simulator.Company platform uses multiple database technologies:

### PostgreSQL

PostgreSQL is used for transactional data and entity relationships:

- Entity definitions and relationships
- Transactional data (actors, forms, accounts)
- Full-text search capabilities
- Access control rules

### ScyllaDB

ScyllaDB is used for high-performance, distributed data:

- Counters and metrics
- Task callbacks
- Push tokens
- Recent objects
- Actor coordinates

### Redis

Redis is used for caching and real-time features:

- Pub/Sub for real-time updates
- Caching of frequently accessed data
- Session management
- Rate limiting

## Integration Points

The platform provides several integration points:

- **Webhooks** - Event notifications to external systems
- **Connectors** - Integration with external financial systems
- **API** - Comprehensive REST API for all operations
- **Task Callbacks** - Scheduled operations and notifications

## System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      Client Applications                         │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────┐
│                           API Layer                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────┐ │
│  │  Graph API  │  │ Account API │  │  Forms API  │  │ Auth API │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └──────────┘ │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────┐
│                        Business Logic                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────┐ │
│  │Graph System │  │Financial Sys│  │Form System  │  │Access Ctrl│ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └──────────┘ │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────┐
│                        Data Access Layer                         │
└───────────────┬─────────────────────────────┬──────────────────┘
                │                             │
    ┌───────────▼────────────┐    ┌───────────▼────────────┐
    │      PostgreSQL        │    │        ScyllaDB        │
    │  ┌─────────────────┐   │    │  ┌─────────────────┐   │
    │  │ Entity Tables   │   │    │  │ Counters        │   │
    │  └─────────────────┘   │    │  └─────────────────┘   │
    │  ┌─────────────────┐   │    │  ┌─────────────────┐   │
    │  │ Relationship    │   │    │  │ Task Callbacks  │   │
    │  │ Tables          │   │    │  └─────────────────┘   │
    │  └─────────────────┘   │    │  ┌─────────────────┐   │
    │  ┌─────────────────┐   │    │  │ Recent Objects  │   │
    │  │ Financial       │   │    │  └─────────────────┘   │
    │  │ Tables          │   │    │  ┌─────────────────┐   │
    │  └─────────────────┘   │    │  │ Push Tokens     │   │
    └────────────────────────┘    └────────────────────────┘
```

## Conclusion

The Simulator.Company platform is built on a comprehensive set of interrelated entities that work together to provide a powerful business process management and financial tracking system. The entity relationships documented here illustrate how these components interact to form the complete system architecture.
