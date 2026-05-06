# Transfers

Transfers in the Simulator.Company platform represent movements of funds between accounts, providing a record of financial transactions across the system.

## Overview

Transfers track the movement of funds between accounts within the platform. Each transfer can involve multiple transactions across different accounts and provides a comprehensive record of financial activities.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the transfer |
| acc_id | String | Workspace ID the transfer belongs to |
| ref | String | External reference identifier |
| user_id | Integer | ID of the user who created the transfer |
| type | Enum | Transfer type (completed, authorized, declined, canceled, reversed) |
| comment | Text | Description or comment about the transfer |
| details | JSON | Detailed information about the transfer |
| decline_reason | JSON | Reason for decline if the transfer was declined |
| data | JSON | Additional transfer data |
| created_at | BigInt | Unix timestamp of creation time |
| updated_at | BigInt | Unix timestamp of last update |

## Transfer Types

The platform supports several transfer types:

- **completed** - Successfully processed transfers
- **authorized** - Transfers that are approved but not yet settled
- **declined** - Transfers that were rejected
- **canceled** - Transfers that were canceled before completion
- **reversed** - Transfers that were reversed after completion

## API Endpoints

For detailed API documentation on transfers, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Transfers API Documentation](https://doc.simulator.company/#tag/transfers)

The API provides endpoints for:

- Getting all transfers in a workspace
- Retrieving details for a specific transfer
- Creating new transfers between accounts
- Updating existing transfers (limited to certain fields and states)
- Searching and filtering transfers

All API requests require appropriate OAuth2 scopes (`control.events:accounts.readonly` for read operations and `control.events:accounts.management` for write operations).

## Database Structure

Transfers are stored in a partitioned table for performance optimization:

- Partitioned by `created_at` for efficient time-based queries
- Indexed by `ref` and `acc_id` for quick lookups
- Full-text search capability via `title_search` column
- Foreign key relationship to `accounts` table

## Relationships

Transfers have relationships with:

- **Transactions** - Each transfer can generate multiple transactions across different accounts
- **History** - Changes to transfers are recorded in the history table
- **Accounts** - Transfers are associated with a specific workspace

## Example

```json
{
  "id": "transfer_123456",
  "acc_id": "workspace_789",
  "ref": "payment_ref_345",
  "user_id": 42,
  "type": "completed",
  "comment": "Monthly payment transfer",
  "details": {
    "source_account": "account_123",
    "destination_account": "account_456",
    "amount": 500.75,
    "currency": "USD"
  },
  "data": {
    "payment_method": "bank_transfer",
    "processor_id": "bank_transfer_789"
  },
  "created_at": 1621459200,
  "updated_at": 1621459200
}
```
