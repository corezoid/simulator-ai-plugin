# Balances

Balances in the Simulator.Company platform track the financial state of accounts, including current amounts, holds, and limits.

## Overview

Balances provide a detailed record of an account's financial state at the time of each transaction. They track credit and debit amounts, holds, and limits, enabling accurate financial reporting and analysis.

> **Note:** balance-history timestamps follow the transaction time convention — see [transactions.md](transactions.md) for units (created on each transaction), and the "Timestamps" rule in `/simulator` for how to present them.

## Properties

| Property | Type | Description |
|----------|------|-------------|
| transaction_id | BigInt | ID of the transaction that created this balance record |
| actor_id | String | ID of the actor associated with the account |
| name_id | String | ID of the account name |
| currency_id | Integer | ID of the currency |
| account_type | String | Type of account |
| credit | Numeric | Credit balance with precision (100,8) |
| credit_hold | Numeric | Credit amount on hold with precision (100,8) |
| credit_min_limit | Numeric | Minimum credit limit |
| credit_max_limit | Numeric | Maximum credit limit |
| debit | Numeric | Debit balance with precision (100,8) |
| debit_hold | Numeric | Debit amount on hold with precision (100,8) |
| debit_min_limit | Numeric | Minimum debit limit |
| debit_max_limit | Numeric | Maximum debit limit |
| created_at | BigInt | Unix timestamp of creation time |

## Balance Calculation

The platform calculates available balances using the following formulas:

- **Available Credit** = credit - credit_hold
- **Available Debit** = debit - debit_hold

Limits are applied to ensure balances stay within defined boundaries:

- credit_min_limit ≤ Available Credit ≤ credit_max_limit
- debit_min_limit ≤ Available Debit ≤ debit_max_limit

## API Endpoints

For detailed API documentation on balances, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Balances API Documentation](https://doc.simulator.company/#tag/balances)

The API provides endpoints for:

- Getting the current balance for a specific account
- Retrieving balance history for a specific account
- Filtering balance records by date range
- Exporting balance data

All API requests require appropriate OAuth2 scopes (`control.events:accounts.readonly` for read operations).

## Database Structure

Balances are stored in a partitioned table for performance optimization:

- Partitioned by `created_at` for efficient time-based queries
- Indexed by actor_id, name_id, currency_id, and account_type for quick lookups
- Foreign key relationship to the transactions table

## Relationships

Balances have relationships with:

- **Transactions** - Each balance record is linked to a specific transaction
- **Actors** - Balances are associated with specific actors
- **Account Names** - Balances are categorized by account names
- **Currencies** - Balances are denominated in specific currencies

## Example

```json
{
  "transaction_id": 123456,
  "actor_id": "actor_789012",
  "name_id": "account_name_345",
  "currency_id": 1,
  "account_type": "operational",
  "credit": 1000.00,
  "credit_hold": 50.00,
  "credit_min_limit": 0.00,
  "credit_max_limit": 5000.00,
  "debit": 500.00,
  "debit_hold": 0.00,
  "debit_min_limit": 0.00,
  "debit_max_limit": 1000.00,
  "created_at": 1621459200
}
```
