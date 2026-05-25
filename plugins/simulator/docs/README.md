# Simulator.Company Documentation for AI Agents

This repository contains comprehensive documentation for the Simulator.Company platform, designed specifically for AI agents to understand the system architecture, entities, and user flows.

## Overview

The Simulator.Company platform is a comprehensive business process management and workflow automation platform that enables organizations to model complex business processes as interactive graphs, manage financial accounts and transactions, and collaborate in real-time.

## Documentation Structure

The documentation is organized into the following sections:

### Entities

Documentation of core system entities and their relationships:

- [Actors](docs/entities/actors.md) - Core entity representing nodes in business process graph
- [Forms](docs/entities/forms.md) - Reusable data structure templates for actors
- [Links](docs/entities/links.md) - Connections between actors forming graph structures
- [Layers](docs/entities/layers.md) - Visual organization of actors and edges
- [Accounts](docs/entities/accounts.md) - Financial tracking for actors
- [Transactions](docs/entities/transactions.md) - Financial operations within accounts
- [Transfers](docs/entities/transfers.md) - Movement of funds between accounts
- [Attachments](docs/entities/attachments.md) - File storage system for actors
- [System Forms](docs/entities/system-forms.md) - Predefined form templates for system functionality

### User Flows

Documentation of common user flows through the public API:

- [Application Management](docs/user-flows/application-management.md) - Creating and managing applications
- [Account Management](docs/user-flows/account-management.md) - Managing financial accounts and transactions
- [Page Management](docs/user-flows/page-management.md) - Creating and configuring application pages
- [Content Management](docs/user-flows/content-management.md) - Managing application content and files

## System Forms

The platform includes predefined form templates for core system functionality:

- **Scripts/Smart Forms/CDU** - Used for defining custom forms and data structures
- **Events** - Used for scheduling and calendar functionality
- **Graphs** - Used for business process visualization
- **Layers** - Used for visual organization of actors
- **Streams** - Used for real-time data flows and notifications
- **Reactions** - Used for user interactions and comments
- **Accounts** - Used for financial tracking
- **Currencies** - Used for defining units of value
- **Transactions** - Used for recording financial activities
- **Transfers** - Used for moving funds between accounts

For detailed information about these system forms, please refer to the [System Forms](docs/entities/system-forms.md) documentation.

## API Documentation

For detailed API documentation, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Simulator.Company API Documentation](https://doc.simulator.company)

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

## Contributing

To contribute to this documentation:

1. Clone the repository
2. Create a new branch for your changes
3. Make your changes and commit them
4. Create a merge request for review

Please ensure that all documentation follows the project's formatting settings for consistency.
