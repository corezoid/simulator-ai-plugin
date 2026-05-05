# Transactions

Transactions in the Simulator.Company platform represent financial activities that occur within accounts, tracking monetary movements with detailed history and state.

## Overview

Transactions record all financial activities within the platform, including deposits, withdrawals, transfers, authorizations, and limit changes. Each transaction affects an account's balance and can be linked to transfers for cross-account operations.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| id | BigInt | Unique identifier for the transaction (auto-incrementing) |
| account_id | String | ID of the account the transaction belongs to |
| ref | String | External reference identifier |
| transfer_id | String | ID of the associated transfer (if applicable) |
| user_id | Integer | ID of the user who created the transaction |
| amount | Numeric | Transaction amount with precision (100,8) |
| parent_amount | Numeric | Amount in parent account currency (for hierarchical accounts) |
| account_amount | Numeric | Current account balance after transaction |
| account_hold_amount | Numeric | Current hold amount after transaction |
| account_min_limit | Numeric | Minimum limit after transaction (if changed) |
| account_max_limit | Numeric | Maximum limit after transaction (if changed) |
| parent_id | BigInt | ID of the parent transaction (for related transactions) |
| type | Enum | Transaction type (authorized, declined, completed, canceled, reversed, minLimitChanged, maxLimitChanged) |
| expiration | Integer | Expiration timestamp for authorized transactions |
| commission | Numeric | Commission amount with precision (100,8) |
| comment | Text | Transaction description or comment |
| data | JSON | Additional transaction data |
| is_done | Boolean | Whether the transaction is completed |
| created_at | BigInt | Unix timestamp of creation time |
| original_date | BigInt | Original transaction date (for backdated transactions) |
| actor_id | String | ID of the actor associated with the account |
| name_id | String | ID of the account name |
| currency_id | Integer | ID of the currency |
| income_type | String | Income classification (credit/debit) |
| account_type | String | Type of account |

## Transaction Types

The platform supports several transaction types:

- **authorized** - Reserved funds that are not yet settled
- **completed** - Finalized transactions that have affected the balance
- **declined** - Rejected transactions
- **canceled** - Transactions that were canceled before completion
- **reversed** - Transactions that were reversed after completion
- **minLimitChanged** - Transactions that record changes to minimum limits
- **maxLimitChanged** - Transactions that record changes to maximum limits

## API Endpoints

For detailed API documentation on transactions, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Transactions API Documentation](https://doc.simulator.company/#tag/transactions)

The API provides endpoints for:

- Getting transactions for a specific account
- Retrieving details for a specific transaction
- Creating new transactions for an account
- Updating existing transactions (limited to certain fields and states)
- Searching and filtering transactions

All API requests require appropriate OAuth2 scopes (`control.events:accounts.readonly` for read operations and `control.events:accounts.management` for write operations).

## Database Structure

Transactions are stored in a partitioned table for performance optimization:

- Partitioned by `created_at` for efficient time-based queries
- Multiple indexes for optimized access patterns
- Foreign key relationship to `actors_accounts`

## Example

```json
{
  "id": 123456,
  "account_id": "account_789012",
  "ref": "payment_ref_345",
  "transfer_id": "transfer_567",
  "user_id": 42,
  "amount": 100.50,
  "parent_amount": 100.50,
  "account_amount": 1500.75,
  "account_hold_amount": 50.25,
  "type": "completed",
  "comment": "Monthly subscription payment",
  "data": {
    "payment_method": "credit_card",
    "processor_id": "stripe_charge_123"
  },
  "is_done": true,
  "created_at": 1621459200,
  "original_date": 1621459200,
  "actor_id": "actor_123456",
  "name_id": "account_name_789",
  "currency_id": 1,
  "income_type": "debit",
  "account_type": "operational"
}
```
