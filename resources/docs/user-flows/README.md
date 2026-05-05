# Simulator.Company User Flows

This document provides an overview of common user flows in the Simulator.Company platform, illustrating how different API endpoints are used together to accomplish specific tasks.

## User Flow Overview

The Simulator.Company platform provides a comprehensive API that enables various user flows for managing applications, accounts, pages, content, forms, and graph structures:

1. **Application Management** - Creating and managing applications
2. **Account Management** - Managing financial accounts and transactions
3. **Page Management** - Creating and configuring application pages
4. **Content Management** - Managing application content and files
5. **Form Management** - Creating custom forms and form-based actors
6. **Graph Management** - Creating and managing actors, links, and layers in graph structures

These user flows demonstrate how to use the platform's API endpoints to accomplish common tasks and implement business processes.

## API Documentation

For detailed API documentation, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Simulator.Company API Documentation](https://doc.simulator.company)

## User Flow Documentation

### Application Management

- [Application Management](./application-management.md) - Creating, updating, and managing applications through the public API

### Account Management

- [Account Management](./account-management.md) - Creating, updating, and managing accounts through the public API

### Page Management

- [Page Management](./page-management.md) - Creating, updating, and managing application pages through the public API

### Content Management

- [Content Management](./content-management.md) - Managing application content, file sources, and folder structures through the public API

### Form Management

- [Custom Car Form](./custom-car-form.md) - Creating custom forms for cars and managing car actors with financial accounts through the public API

### Graph Management

- [Graph Functionality](./graph-functionality.md) - Creating and managing actors, links, and layers in graph structures through the public API
- [Actor Graph Management](./actor-graph-management.md) - Managing actors on graphs, including creating links and organizing on layers

## Related System Forms

The user flows described in this documentation utilize various system forms that define the structure and behavior of entities in the platform. For detailed information about these system forms, please refer to:

- [System Forms](../entities/system-forms.md) - Documentation of predefined form templates for system functionality

Key system forms used in these user flows include:

- **Scripts/Smart Forms/CDU** - Used in application management for defining custom forms
- **Events** - Used for scheduling and calendar functionality
- **Graphs** - Used for business process visualization
- **Layers** - Used for visual organization of actors
- **Streams** - Used for real-time data flows and notifications
- **Reactions** - Used for user interactions and comments
- **Accounts** - Used in account management for financial tracking
- **Currencies** - Used in account management for defining units of value
- **Transactions** - Used in account management for recording financial activities
- **Transfers** - Used in account management for moving funds between accounts

## Related Entity Documentation

For detailed information about the entities used in these user flows, please refer to:

- [Actors](../entities/actors.md) - Core entity representing nodes in business process graph
- [Forms](../entities/forms.md) - Reusable data structure templates for actors
- [Links](../entities/links.md) - Connections between actors forming graph structures
- [Layers](../entities/layers.md) - Visual organization of actors and edges
- [Accounts](../entities/accounts.md) - Financial tracking for actors
- [Transactions](../entities/transactions.md) - Financial operations within accounts
- [Transfers](../entities/transfers.md) - Movement of funds between accounts
- [Attachments](../entities/attachments.md) - File storage system for actors

## Authentication and Authorization

All API requests require OAuth2 authentication. The specific scopes required for each endpoint are documented in the official API documentation.

Common scopes used in these user flows include:

- `control.events:actors.readonly` - Read-only access to actors
- `control.events:actors.management` - Create, update, and delete actors
- `control.events:forms.readonly` - Read-only access to forms
- `control.events:forms.management` - Create, update, and delete forms
- `control.events:accounts.readonly` - Read-only access to accounts
- `control.events:accounts.management` - Create, update, and delete accounts
- `control.events:attachments.readonly` - Read-only access to attachments
- `control.events:attachments.management` - Create, update, and delete attachments

## Conclusion

The user flows documented here provide a practical guide to using the Simulator.Company platform's API to implement common business processes. By following these flows, developers can quickly understand how to leverage the platform's capabilities to build powerful applications.
