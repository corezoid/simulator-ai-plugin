# System Forms

System Forms in the Simulator.Company platform provide predefined form templates for core platform functionality and specialized features.

## Overview

System Forms are built-in form templates that define the structure and behavior of various system components. These forms are automatically created during system initialization and provide the foundation for many platform features, including events, graphs, layers, streams, scripts (Smart Forms/CDU), reactions, accounts, currencies, transactions, and other specialized functionality.

## Properties

System Forms are stored in the `system_forms` table with the following properties:

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the system form |
| type | String | Type of form (typically 'system') |
| order | Integer | Display order (optional) |
| form | JSON | Form definition structure |

## Form Definition Structure

Each system form has a JSON definition with the following common structure:

| Property | Type | Description |
|----------|------|-------------|
| title | String | Display title for the form |
| color | String | Color associated with the form (hex code) |
| tags | Array | Tags for categorization |
| sections | Array | Form sections containing fields |
| settings | Object | Form-specific settings |
| description | String | Description of the form's purpose |

## Core System Forms

### Scripts (Smart Forms/CDU)

Scripts (also known as Smart Forms or CDU) provide customizable form templates that can be used throughout the platform.

```json
{
  "tags": [],
  "color": "#d9d385",
  "title": "Scripts",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "sharedWith",
      "class": "select",
      "title": "Shared with",
      "value": "",
      "options": [
        {"title": "Users from the list", "value": "userList"},
        {"title": "All workspace users", "value": "allWorkspaceUsers"},
        {"title": "All registered users", "value": "allRegisteredUsers"},
        {"title": "Anyone with the link", "value": "anyone"}
      ],
      "required": true,
      "visibility": "visible"
    }]
  }],
  "settings": {},
  "description": ""
}
```

### Graphs

The Graphs system form defines the basic structure for graph visualization.

```json
{
  "tags": [],
  "color": "#9ced8b",
  "title": "Graphs",
  "sections": [{"content": []}],
  "settings": {},
  "description": ""
}
```

### Layers

Layers provide organization and visualization options for actors in graphs.

```json
{
  "tags": [],
  "color": "#d1a5ed",
  "title": "Layers",
  "sections": [{
    "title": "Settings",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Type",
        "value": [{"title": "Graph", "value": "graph"}],
        "options": [
          {"title": "Graph", "value": "graph"},
          {"title": "Tree", "value": "tree"}
        ],
        "visibility": "visible"
      },
      {
        "id": "edgeCurveStyle",
        "class": "select",
        "title": "Edges type",
        "value": [{"title": "Curved", "value": "curved"}],
        "options": [
          {"title": "Curved", "value": "curved"},
          {"title": "Rounded", "value": "rounded"},
          {"title": "Straight", "value": "straight"}
        ],
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": ""
}
```

## Filter Forms

### ActorFilters

ActorFilters define criteria for filtering actors in views and dashboards.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "ActorFilters",
  "sections": [{
    "title": "Settings",
    "content": [
      {
        "id": "filter",
        "type": "text",
        "class": "edit",
        "extra": {"multiline": true},
        "title": "Filter",
        "required": false,
        "visibility": "disabled"
      },
      {
        "id": "fields",
        "type": "text",
        "class": "edit",
        "extra": {"multiline": true},
        "title": "Visible fields",
        "required": false,
        "visibility": "disabled"
      }
    ]
  }],
  "settings": {},
  "description": ""
}
```

### TransactionFilters

TransactionFilters define criteria for filtering financial transactions.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "TransactionFilters",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "filter",
      "type": "text",
      "class": "edit",
      "extra": {"multiline": true},
      "title": "Filter",
      "required": false,
      "visibility": "disabled"
    }]
  }],
  "settings": {},
  "description": ""
}
```

### TransferFilters

TransferFilters define criteria for filtering transfers between accounts.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "TransferFilters",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "filter",
      "type": "text",
      "class": "edit",
      "extra": {"multiline": true},
      "title": "Filter",
      "required": false,
      "visibility": "disabled"
    }]
  }],
  "settings": {},
  "description": ""
}
```

## Financial Forms

### Accounts

The Accounts system form defines the structure for financial accounts associated with actors.

```json
{
  "tags": [],
  "color": "#4a90e2",
  "title": "Accounts",
  "sections": [{
    "title": "Account Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Account Type",
        "value": "",
        "options": [
          {"title": "Operational", "value": "operational"},
          {"title": "Analytical", "value": "analytical"},
          {"title": "Formula", "value": "formula"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "income_type",
        "class": "select",
        "title": "Income Type",
        "value": "",
        "options": [
          {"title": "Debit", "value": "debit"},
          {"title": "Credit", "value": "credit"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "tree_calculation",
        "class": "checkbox",
        "title": "Tree Calculation",
        "value": false,
        "visibility": "visible"
      },
      {
        "id": "formula",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Formula",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines financial accounts that can be associated with actors"
}
```

Accounts in the platform are used to track financial information and balances. The Accounts system form provides the structure for:

- Creating operational accounts for tracking actual balances
- Setting up analytical accounts for reporting
- Defining formula-based accounts that calculate values from other accounts
- Configuring hierarchical balance calculations through tree structures
- Setting limits and constraints on account balances

### Currencies

The Currencies system form defines the structure for currencies used in financial accounts.

```json
{
  "tags": [],
  "color": "#50e3c2",
  "title": "Currencies",
  "sections": [{
    "title": "Currency Properties",
    "content": [
      {
        "id": "name",
        "class": "edit",
        "title": "Currency Name",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "symbol",
        "class": "edit",
        "title": "Symbol",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "precision",
        "class": "edit",
        "type": "int",
        "title": "Decimal Precision",
        "value": 2,
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "type",
        "class": "select",
        "title": "Currency Type",
        "value": "",
        "options": [
          {"title": "Number", "value": "number"},
          {"title": "Percent", "value": "percent"},
          {"title": "Currency", "value": "currency"},
          {"title": "DateTime", "value": "dateTime"},
          {"title": "Seconds", "value": "seconds"}
        ],
        "required": true,
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines currencies used in financial accounts"
}
```

Currencies in the platform define the units of value for financial accounts. The Currencies system form provides the structure for:

- Defining currency names and symbols
- Setting decimal precision for financial calculations
- Specifying currency types (numeric, percentage, time-based)
- Creating both standard and custom currency units

### Transactions

The Transactions system form defines the structure for financial transactions affecting account balances.

```json
{
  "tags": [],
  "color": "#f5a623",
  "title": "Transactions",
  "sections": [{
    "title": "Transaction Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Transaction Type",
        "value": "",
        "options": [
          {"title": "Authorized", "value": "authorized"},
          {"title": "Completed", "value": "completed"},
          {"title": "Declined", "value": "declined"},
          {"title": "Canceled", "value": "canceled"},
          {"title": "Reversed", "value": "reversed"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "amount",
        "class": "edit",
        "type": "decimal",
        "title": "Amount",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "description",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Description",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines financial transactions affecting account balances"
}
```

Transactions in the platform record financial activities within accounts. The Transactions system form provides the structure for:

- Recording financial movements with specific transaction types
- Tracking transaction amounts and descriptions
- Supporting various transaction states (authorized, completed, declined)
- Enabling transaction history and audit trails

### AccountTriggers

AccountTriggers define conditions for triggering actions based on account balances or transaction counts.

```json
{
  "tags": [],
  "color": "#00ff0d",
  "title": "AccountTriggers",
  "sections": [{
    "title": "Trigger settings",
    "content": [
      {
        "id": "valueType",
        "class": "select",
        "title": "Account value type",
        "value": [{"title": "amount", "value": "amount"}],
        "options": [
          {"title": "Account amount", "value": "amount"},
          {"title": "Count of transactions", "value": "count"}
        ],
        "required": true
      },
      {
        "id": "accountIncomeType",
        "class": "multiSelect",
        "title": "Account income type",
        "value": [{"title": "Total", "value": "total"}],
        "options": [
          {"title": "Total", "value": "total"},
          {"title": "Debit", "value": "debit"},
          {"title": "Credit", "value": "credit"}
        ]
      }
    ]
  }],
  "settings": {},
  "description": ""
}
```

### Transfers

The Transfers system form defines the structure for transfer operations between accounts.

```json
{
  "tags": [],
  "color": "#9ced8b",
  "title": "Transfers",
  "sections": [{
    "title": "Transfer Properties",
    "content": [
      {
        "id": "source_account_id",
        "class": "select",
        "title": "Source Account",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "destination_account_id",
        "class": "select",
        "title": "Destination Account",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "amount",
        "class": "edit",
        "type": "decimal",
        "title": "Amount",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "description",
        "class": "edit",
        "type": "text",
        "title": "Description",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines transfers between financial accounts"
}
```

Transfers in the platform move funds between accounts. The Transfers system form provides the structure for:

- Defining source and destination accounts for transfers
- Specifying transfer amounts and descriptions
- Supporting various transfer types and states
- Enabling transfer history and audit trails

## Integration Forms

### CommentsWidgets

CommentsWidgets define the structure for comment functionality.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "CommentsWidgets",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "status",
      "class": "select",
      "title": "Status",
      "value": "active",
      "options": [
        {"color": "#2fb773", "title": "Active", "value": "active"},
        {"color": "#fdab00", "title": "Inactive", "value": "inactive"}
      ]
    }]
  }],
  "settings": {},
  "description": ""
}
```

### Dashboards

Dashboards define the structure for data visualization dashboards.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "Dashboards",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "source",
      "type": "text",
      "class": "edit",
      "extra": {"multiline": true},
      "title": "Source",
      "required": false,
      "visibility": "disabled"
    }]
  }],
  "settings": {},
  "description": ""
}
```

### Snippets

Snippets store reusable code or text fragments organized by category.

```json
{
  "tags": [],
  "color": "#ecaea2",
  "title": "Snippets",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "category",
      "type": "text",
      "required": true,
      "class": "edit",
      "title": "Category"
    }]
  }],
  "settings": {},
  "description": ""
}
```

## Company Integration Forms

### CompanyCategories

CompanyCategories define categories for classifying companies.

```json
{
  "tags": [],
  "color": "#9ced8b",
  "title": "CompanyCategories",
  "sections": [{"content": []}],
  "settings": {},
  "description": ""
}
```

### PublishedCompanies

PublishedCompanies define the structure for companies published in the marketplace.

```json
{
  "tags": [],
  "color": "#e0d8ea",
  "title": "PublishedCompanies",
  "sections": [{
    "title": "General information",
    "content": [
      {
        "id": "site",
        "class": "edit",
        "title": "Site URL",
        "visibility": "visible"
      },
      {
        "id": "categories",
        "class": "select",
        "extra": "{{CompanyCategories}}",
        "title": "Categories",
        "value": "",
        "options": [],
        "required": true
      }
    ]
  }],
  "settings": {},
  "description": ""
}
```

### CompanyCard

CompanyCard defines the structure for company profile cards.

```json
{
  "tags": [],
  "color": "#ff36a3",
  "title": "CompanyCard",
  "sections": [{
    "title": "General information",
    "content": [
      {
        "id": "site",
        "class": "edit",
        "title": "Site URL",
        "visibility": "visible",
        "regexp": "^(https?|ftp):\\/\\/[^\\s/$.?#].[^\\s]*$"
      },
      {
        "id": "categories",
        "type": "text",
        "class": "edit",
        "extra": {"rows": "5", "multiline": true},
        "regexp": "^[a-zA-Z0-9, ]*$",
        "title": "Categories",
        "value": "",
        "required": true
      }
    ]
  }],
  "settings": {},
  "description": ""
}
```

## Specialized Forms

### State

State defines geographical regions using polygon coordinates.

```json
{
  "tags": [],
  "color": "#ecfad7",
  "title": "State",
  "sections": [{
    "title": "Settings",
    "content": [{
      "id": "polygon",
      "type": "text",
      "class": "edit",
      "extra": {"rows": 10, "multiline": true},
      "title": "Polygon points",
      "required": true,
      "visibility": "disabled"
    }]
  }],
  "settings": {},
  "description": ""
}
```

## External Connector Forms

The platform includes several system forms for external integrations that are not explicitly defined in the migration script but are referenced in the codebase:

### AWS Connector

Enables integration with Amazon Web Services for storage, computation, and other services.

### Stripe Connector

Facilitates payment processing through the Stripe payment gateway.

### PayPal Connector

Enables integration with PayPal for payment processing.

### Email Connector

Provides email notification capabilities through SMTP or API-based email services.

### SMS Connector

Enables SMS notification capabilities through various SMS gateway providers.

## API Endpoints

For detailed API documentation on system forms, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[System Forms API Documentation](https://doc.simulator.company/#tag/system-forms)

The API provides endpoints for:

- Getting all system forms
- Retrieving system forms of a specific type
- Accessing system form definitions and structures

All API requests require appropriate OAuth2 scopes (`control.events:forms.readonly` for read operations).

### System

The System system form defines core system settings and configurations.

```json
{
  "tags": [],
  "color": "#000000",
  "title": "System",
  "sections": [{
    "title": "System Properties",
    "content": [
      {
        "id": "key",
        "class": "edit",
        "title": "Configuration Key",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "value",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Configuration Value",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "description",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Description",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines core system settings and configurations"
}
```

The System system form provides the structure for:

- Storing global system configuration settings
- Defining feature flags and toggles
- Managing system-wide parameters
- Configuring default behaviors and limits

## Additional System Forms

### Widgets

The Widgets system form defines the structure for UI widgets used throughout the platform.

```json
{
  "tags": [],
  "color": "#9ced8b",
  "title": "Widgets",
  "sections": [{
    "title": "Widget Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Widget Type",
        "value": "",
        "options": [
          {"title": "Chart", "value": "chart"},
          {"title": "Table", "value": "table"},
          {"title": "Counter", "value": "counter"},
          {"title": "Status", "value": "status"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "source",
        "class": "select",
        "title": "Data Source",
        "value": "",
        "options": [
          {"title": "Actor", "value": "actor"},
          {"title": "Account", "value": "account"},
          {"title": "Transaction", "value": "transaction"},
          {"title": "API", "value": "api"}
        ],
        "required": true,
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines UI widgets for data visualization"
}
```

Widgets in the platform provide visualization and interaction components. The Widgets system form provides the structure for:

- Creating various widget types (charts, tables, counters)
- Configuring data sources for widgets
- Setting up widget appearance and behavior
- Enabling interactive dashboard components

### Locations

The Locations system form defines geographical locations with coordinates.

```json
{
  "tags": [],
  "color": "#50e3c2",
  "title": "Locations",
  "sections": [{
    "title": "Location Properties",
    "content": [
      {
        "id": "coordinates",
        "class": "edit",
        "title": "Coordinates",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "address",
        "class": "edit",
        "title": "Address",
        "required": false,
        "visibility": "visible"
      },
      {
        "id": "type",
        "class": "select",
        "title": "Location Type",
        "value": "",
        "options": [
          {"title": "Point", "value": "point"},
          {"title": "Area", "value": "area"},
          {"title": "Route", "value": "route"}
        ],
        "required": true,
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines geographical locations with coordinates"
}
```

Locations in the platform provide geographical context for actors. The Locations system form provides the structure for:

- Storing geographical coordinates
- Associating addresses with locations
- Defining different location types (points, areas, routes)
- Supporting map-based visualizations

### Tags

The Tags system form defines categorization tags for actors and other entities.

```json
{
  "tags": [],
  "color": "#f5a623",
  "title": "Tags",
  "sections": [{
    "title": "Tag Properties",
    "content": [
      {
        "id": "name",
        "class": "edit",
        "title": "Tag Name",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "color",
        "class": "edit",
        "title": "Color",
        "required": false,
        "visibility": "visible"
      },
      {
        "id": "category",
        "class": "select",
        "title": "Category",
        "value": "",
        "options": [],
        "required": false,
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines categorization tags for actors and other entities"
}
```

Tags in the platform provide categorization and filtering capabilities. The Tags system form provides the structure for:

- Creating named tags with colors
- Organizing tags into categories
- Applying tags to actors and other entities
- Supporting tag-based filtering and searching

### Streams

The Streams system form defines data streams for real-time updates and notifications.

```json
{
  "tags": [],
  "color": "#4a90e2",
  "title": "Streams",
  "sections": [{
    "title": "Stream Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Stream Type",
        "value": "",
        "options": [
          {"title": "Actor", "value": "actor"},
          {"title": "Account", "value": "account"},
          {"title": "System", "value": "system"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "filter",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Filter",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines data streams for real-time updates and notifications"
}
```

Streams in the platform provide real-time data flows and notifications. The Streams system form provides the structure for:

- Defining different stream types (actor, account, system)
- Configuring stream filters and conditions
- Setting up stream processing rules
- Enabling real-time notifications and updates

### Events

The Events system form defines scheduled and triggered events in the platform.

```json
{
  "tags": [],
  "color": "#bd10e0",
  "title": "Events",
  "sections": [{
    "title": "Event Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Event Type",
        "value": "",
        "options": [
          {"title": "Meeting", "value": "meeting"},
          {"title": "Reminder", "value": "reminder"},
          {"title": "Task", "value": "task"},
          {"title": "Deadline", "value": "deadline"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "start_date",
        "class": "edit",
        "type": "date",
        "title": "Start Date",
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "end_date",
        "class": "edit",
        "type": "date",
        "title": "End Date",
        "required": false,
        "visibility": "visible"
      },
      {
        "id": "participants",
        "class": "multiSelect",
        "title": "Participants",
        "value": [],
        "options": [],
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines scheduled and triggered events in the platform"
}
```

Events in the platform provide scheduling and calendar functionality. The Events system form provides the structure for:

- Creating different event types (meetings, reminders, tasks)
- Scheduling events with start and end dates
- Assigning participants to events
- Setting up event notifications and reminders

### Reactions

The Reactions system form defines user interactions with actors, such as comments, ratings, and approvals.

```json
{
  "tags": [],
  "color": "#f8e71c",
  "title": "Reactions",
  "sections": [{
    "title": "Reaction Properties",
    "content": [
      {
        "id": "type",
        "class": "select",
        "title": "Reaction Type",
        "value": "",
        "options": [
          {"title": "View", "value": "view"},
          {"title": "Comment", "value": "comment"},
          {"title": "Rating", "value": "rating"},
          {"title": "Sign", "value": "sign"},
          {"title": "Done", "value": "done"},
          {"title": "Reject", "value": "reject"},
          {"title": "Freeze", "value": "freeze"}
        ],
        "required": true,
        "visibility": "visible"
      },
      {
        "id": "content",
        "class": "edit",
        "type": "text",
        "extra": {"multiline": true},
        "title": "Content",
        "visibility": "visible"
      },
      {
        "id": "rating",
        "class": "edit",
        "type": "int",
        "title": "Rating Value",
        "visibility": "visible"
      }
    ]
  }],
  "settings": {},
  "description": "Defines user interactions with actors, such as comments, ratings, and approvals"
}
```

Reactions in the platform provide user interaction and collaboration capabilities. The Reactions system form provides the structure for:

- Recording user views of actors
- Adding comments and discussion threads
- Providing ratings and feedback
- Approving or rejecting actors (signing)
- Marking tasks as complete (done)
- Freezing actors to prevent further changes
- Building hierarchical reaction trees for threaded discussions

Reactions are stored as specialized actors with a tree structure, enabling threaded comments and nested replies. The platform uses the REACTIONS_EDGE_TYPE to connect reactions to their parent actors or other reactions.

## Usage in the Platform

System Forms are used throughout the platform for various purposes:

- **Scripts/Smart Forms/CDU**: Provide the foundation for customizable form templates
- **Events**: Enable scheduling and calendar functionality
- **Graphs**: Support business process visualization
- **Layers**: Provide visual organization for actors
- **Streams**: Enable real-time data flows and notifications
- **Reactions**: Support user interactions and comments
- **Accounts**: Manage financial information and balances
- **Currencies**: Define units of value for financial accounts
- **Transactions**: Record financial activities within accounts
- **Transfers**: Move funds between accounts
- **Filters**: Enable filtering of actors, transactions, and transfers
- **Dashboards**: Support data visualization and reporting
- **Integration Forms**: Facilitate connections with external systems
- **Company Forms**: Support marketplace and company profile functionality
- **Widgets**: Provide visualization and interaction components
- **Locations**: Store geographical information
- **Tags**: Enable categorization and filtering

These forms are typically not directly editable by users but provide the structure for system functionality that users can configure and extend.
