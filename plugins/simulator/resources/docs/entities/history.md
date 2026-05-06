# History

History in the Simulator.Company platform tracks changes to objects, providing an audit trail of modifications across the system.

## Overview

The History system records all changes made to objects within the platform, creating a comprehensive audit trail. It tracks modifications to actors, form templates, accounts, and other entities, storing both the previous and new values for each change.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | BigInt | Unique identifier for the history record (auto-incrementing) |
| obj_type | Enum | Type of object being tracked (actor, formTemplate, templateActors, account) |
| obj_id | String | ID of the object being tracked |
| field | String | Name of the field that was changed |
| field_title | String | Display title for the field |
| form_title | String | Title of the form containing the field |
| transfer_id | String | ID of the associated transfer (if applicable) |
| user_id | Integer | ID of the user who made the change |
| prev_value | Text | Previous value before the change |
| new_value | Text | New value after the change |
| data | JSON | Additional data about the change |
| created_at | BigInt | Unix timestamp of when the change occurred |

## Object Types

The platform tracks history for several types of objects:

- **actor** - Changes to actor properties and data
- **formTemplate** - Changes to form templates
- **templateActors** - Changes to template actors
- **account** - Changes to account properties

## API Endpoints

For detailed API documentation on history, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[History API Documentation](https://doc.simulator.company/#tag/history)

The API provides endpoints for:

- Getting the change history for a specific object
- Retrieving the change history for a specific field of an object
- Getting all history records associated with a specific transfer
- Filtering history records by date range and user

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for actor history and `control.events:accounts.readonly` for transfer history).

## Database Structure

History records are stored in a partitioned table for performance optimization:

- Partitioned by `created_at` for efficient time-based queries
- Indexed by object type, object ID, and field for quick lookups
- Foreign key relationship to the transfers table

## Relationships

History has relationships with:

- **Actors** - Tracks changes to actor properties and data
- **Forms** - Tracks changes to form templates
- **Accounts** - Tracks changes to account properties
- **Transfers** - Links history records to specific transfers

## Example

```json
{
  "id": 123456,
  "obj_type": "actor",
  "obj_id": "actor_789012",
  "field": "title",
  "field_title": "Title",
  "form_title": "Customer Form",
  "user_id": 42,
  "prev_value": "Old Customer Name",
  "new_value": "New Customer Name",
  "data": {
    "change_reason": "Customer requested name update",
    "change_source": "web_interface"
  },
  "created_at": 1621459200
}
```

## Audit Trail

The history system provides a complete audit trail for compliance and tracking purposes:

- Records who made each change (user_id)
- Records when each change was made (created_at)
- Stores both the previous and new values for comparison
- Links changes to business processes through transfer_id
- Supports filtering and searching by object type, field, and time period
