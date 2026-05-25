# Actors

Actors in the Simulator.Company platform represent the core entities in business process graphs, serving as nodes that can be connected through links to model complex workflows.

## Overview

Actors are the fundamental building blocks of the platform, representing various business entities such as tasks, documents, users, or any other object in a business process. Each actor is based on a form template that defines its structure and behavior.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the actor |
| acc_id | String | Workspace ID the actor belongs to |
| user_id | Integer | ID of the user who created the actor |
| form_id | Integer | ID of the form template used by this actor |
| title | Text | Display title of the actor |
| description | Text | Detailed description of the actor |
| ref | String | External reference identifier |
| data | JSON | Custom data associated with the actor, structured according to its form template |
| color | String | Color associated with the actor (hex code) |
| picture | Text | URL or path to the actor's image |
| status | Enum | Actor status (active, removed, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |
| app_id | String | ID of the application this actor belongs to (if applicable) |

## Actor Types

The platform supports various actor types based on their purpose:

- **Standard Actors** - Regular business entities with custom data
- **Layer Actors** - Special actors that represent visualization layers
- **System Actors** - Built-in actors for system functionality
- **Reaction Actors** - Actors representing user interactions (comments, approvals, etc.)
- **Application Actors** - Actors representing applications or integrations

## API Endpoints

For detailed API documentation on actors, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Actors API Documentation](https://doc.simulator.company/#tag/actors)

The API provides endpoints for:

- Getting actor details
- Creating new actors
- Updating existing actors
- Deleting actors
- Retrieving actors by form
- Getting and updating actor data
- Managing actor relationships

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for read operations and `control.events:actors.management` for write operations).

## Relationships

Actors have relationships with various other entities in the system:

- **Forms** - Each actor is based on a form template that defines its structure
- **Links** - Actors can be connected to other actors through links
- **Accounts** - Actors can have associated financial accounts
- **Attachments** - Files can be attached to actors
- **Layers** - Actors can be placed on visualization layers
- **Reactions** - Users can interact with actors through reactions

## Database Structure

Actors are stored in the `actors` table with the following structure:

- Primary key on `id`
- Foreign key relationships to forms, users, and workspaces
- Indexed for efficient querying by form_id, ref, and other properties
- Full-text search capabilities through the ActorsSearch model

## Example

### Actor JSON

```json
{
  "id": "actor_123456",
  "title": "Customer Onboarding",
  "description": "Process for onboarding new customers",
  "form_id": 42,
  "data": {
    "customer_name": "Acme Corp",
    "contact_email": "contact@acme.com",
    "onboarding_stage": "documentation",
    "priority": "high"
  },
  "color": "#3498db",
  "status": "active",
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Actor with Relationships

```json
{
  "id": "actor_123456",
  "title": "Customer Onboarding",
  "form_id": 42,
  "data": { ... },
  "links": [
    {
      "id": "link_789012",
      "target_id": "actor_345678",
      "type_id": 1,
      "type_name": "Process Flow"
    }
  ],
  "accounts": [
    {
      "id": "account_901234",
      "name_id": "account_name_567",
      "name": "Project Budget",
      "amount": 5000.00,
      "currency_id": 1,
      "currency_symbol": "$"
    }
  ],
  "attachments": [
    {
      "id": "attachment_234567",
      "filename": "contract.pdf",
      "size": 1024000,
      "mime_type": "application/pdf"
    }
  ]
}
```

## Usage in the Platform

Actors are used throughout the platform for various purposes:

- **Business Process Modeling** - Representing entities in process flows
- **Data Collection** - Structured data entry through form templates
- **Financial Tracking** - Association with accounts for financial operations
- **Collaboration** - User interactions through reactions and comments
- **Visualization** - Placement on layers for visual organization

Actors form the foundation of the platform's graph-based approach to business process management, enabling flexible and powerful modeling of complex workflows.
