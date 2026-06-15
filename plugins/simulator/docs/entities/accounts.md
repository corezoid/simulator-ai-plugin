# Accounts

Accounts in the Simulator.Company platform represent financial entities that can track balances, transactions, and transfers between different actors.

## Overview

Accounts are used to track financial information within the platform. Each account has a name, currency, and can be associated with actors to track balances and transactions. The platform supports various account types, including formula-based accounts that calculate their balances based on other accounts.

## Account Components

The account system consists of several components:

### Account Names

Account names define the categories or types of accounts available in a workspace.

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the account name |
| acc_id | String | Workspace ID the account name belongs to |
| user_id | Integer | ID of the user who created the account name |
| name | Text | Name of the account |
| is_system | Boolean | Whether this is a system account name |
| abbreviation | String | Short abbreviation for the account name |
| status | Enum | Status of the account name (active, removed) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Account Currencies

Account currencies define the types of currencies that can be used in accounts.

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the currency |
| acc_id | String | Workspace ID the currency belongs to |
| user_id | Integer | ID of the user who created the currency |
| name | Text | Name of the currency |
| symbol | String | Symbol representing the currency |
| precision | Integer | Number of decimal places shown in the UI (display only). Amounts/balances are stored as their real decimal value — precision is not a scaling factor, so do not multiply or divide amounts by `10^precision` |
| type | Enum | Type of currency (number, etc.) |
| is_system | Boolean | Whether this is a system currency |
| created_at | Integer | Unix timestamp of creation time |

### Actors Accounts

Actors Accounts link financial accounts to specific actors, tracking balances and transactions.

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the account |
| actor_id | String | ID of the actor this account belongs to |
| user_id | Integer | ID of the user who created the account |
| name_id | String | ID of the account name |
| type | Enum | Account type (operational, analytical, etc.) |
| counter_type | Enum | Counter type for metrics (amount, count) |
| income_type | Enum | Income classification (debit, credit) |
| currency_id | Integer | ID of the currency for this account |
| amount | Decimal | Current balance amount |
| hold_amount | Decimal | Amount currently on hold |
| system_type | Enum | Whether the account is manual or system-managed |
| status | Enum | Account status (active, removed, blocked) |
| formula | Text | Formula for calculated accounts |
| min_limit | Decimal | Minimum balance limit |
| max_limit | Decimal | Maximum balance limit |
| search | Boolean | Whether the account appears in search results |
| tree_calculation | Boolean | Whether the account participates in hierarchical calculations |
| connectors | Boolean | Whether the account uses external connectors |
| limit_service_url | String | URL for external limit service |
| commission_service_url | String | URL for external commission service |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

### Account Formulas

Account Formulas define relationships between source accounts and calculated accounts for formula-based balance calculations.

| Property | Type | Description |
|----------|------|-------------|
| id | Integer | Unique identifier for the formula relationship |
| source_account_id | String | ID of the source account used in the formula |
| calc_account_id | String | ID of the calculated account using the formula |
| user_id | Integer | ID of the user who created the formula relationship |
| created_at | Integer | Unix timestamp of creation time |

## Formula-Based Accounts

The platform supports formula-based accounts that calculate their balances using mathematical expressions referencing other accounts. These accounts:

- Use the `formula` field in ActorsAccounts to define the calculation expression
- Track dependencies between accounts using the AccountFormulas table
- Support complex mathematical operations (addition, subtraction, multiplication, division)
- Can reference other accounts by ID or by using special variables
- Update automatically when source accounts change
- Can be used for aggregation, allocation, and other financial calculations

### Setting a formula (tools)

Manage formulas with **`setAccountFormula`** (POST) and **`getAccountFormula`** (read):

```
setAccountFormula(accountId="<calcAccountId>",
  formula="<srcAccUuidA> + <srcAccUuidB> * 0.2")   # numeric expression over source account UUIDs
getAccountFormula(accountId="<calcAccountId>")       # → the source accounts it references (or {} if none)
```

How the formula is evaluated (from the backend):

- **Reference each source account by its full account UUID** (the account `id` from
  `getAccount`/`getAccounts` — **not** the actor id). At calc time, every UUID in the string is
  substituted with that account's **available balance = `amount − hold`**, then the whole
  expression is evaluated with **mathjs** (`+ - * / ( )`, functions, etc.). The result must be
  **numeric** and becomes the account's balance.
- The formula account is **derived/computed** — it has no transactions of its own and
  **recalculates automatically** whenever a referenced source account's balance changes
  (the recompute cascades to formulas that depend on it).
- **Rule: you cannot set a formula on an account that already has transactions** — the backend
  rejects it. Use a fresh account (or one that has only ever been formula-driven).
- **Clear** a formula (make it a plain account again) by passing an **empty string** for `formula`.

## Account Types

The platform supports several account types:

- **Operational** - Standard accounts for tracking actual balances
- **Analytical** - Accounts for analysis and reporting
- **Formula** - Accounts with balances calculated from other accounts
- **System** - System-managed accounts for internal operations

## API Endpoints

For detailed API documentation on accounts, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Accounts API Documentation](https://doc.simulator.company/#tag/accounts)
[Account Names API Documentation](https://doc.simulator.company/#tag/account-names)
[Currencies API Documentation](https://doc.simulator.company/#tag/currencies)

The API provides endpoints for:

- Getting all account names in a workspace
- Creating new account names
- Getting all currencies in a workspace
- Creating new currencies
- Getting all accounts for a specific actor
- Creating new accounts for actors
- Updating existing accounts
- Deleting accounts

All API requests require appropriate OAuth2 scopes (`control.events:accounts.readonly` for read operations and `control.events:accounts.management` for write operations).

## Transactions and Transfers

Accounts can have transactions and transfers:

- **Transactions**: Record financial activities within an account
- **Transfers**: Move funds between accounts

See the [Transactions](transactions.md) and [Transfers](transfers.md) documentation for more details.

## Hierarchical Balance Calculation

The platform supports hierarchical balance calculation through the `tree_calculation` flag:

- Parent accounts can automatically aggregate balances from child accounts
- Changes to child accounts propagate up the hierarchy
- Hierarchical relationships are defined through actor links with the hierarchy edge type
- This enables organizational rollups and departmental aggregations

## Account Tags and Triggers (account_to_actors)

Accounts can be linked to actors of two system forms through the `account_to_actors`
mechanism — one table powering three features:

- **Tags** — actors of the system form `Tags` linked to a workspace-level
  `(name_id, currency_id)` pair. The tag labels every account of that pair (on every
  actor). Used for grouping/filtering accounts (`getAccounts` supports `withTags`,
  `tag=<tag actor UUID>`, `ungrouped`).
- **Account triggers** — actors of the system form `AccountTriggers` linked to a pair
  (all its accounts) or to one specific account (`account_id`). On every transaction the
  watched value (balance or transaction count, optionally over a calendar period) is
  classified into `lowerZone` / `normalZone` / `upperZone`, and the trigger fires only on
  a zone **transition**, delivering a `type: "trigger"` webhook to API users with access
  to the trigger actor. Trigger settings live in the actor's `data`: `valueType`
  (`amount` | `count` | `data`), `accountIncomeType` (`total`/`debit`/`credit`),
  `lowerValue`/`upperValue`, `periodName`+`periodValue`, `endDate`, `customParams`.
- **Data triggers** — `AccountTriggers` actors with `valueType=data`, an `operator`
  (`=`/`!=`) and a `comparisonValue`, bound to a data field of one actor
  (`target_actor_id`) or of every actor of a form (`target_form_id`). They fire when the
  watched field changes and the comparison outcome flips (matched ↔ unmatched).

Linking is a **replace** operation: saving a new set first removes the existing links in
scope (pair + linked-actor form, account, or target+dataField), then inserts the new
list. MCP tools: `saveAccountActors`, `getDataFieldActorsByActor`,
`saveDataFieldActorsByActor`, `getDataFieldActorsByForm`, `saveDataFieldActorsByForm`.

## External Integrations

Accounts can integrate with external systems through:

- `limit_service_url` - External service for dynamic limit calculations
- `commission_service_url` - External service for commission calculations
- `connectors` flag - Enables integration with external financial systems

## Examples

### Account Name

```json
{
  "id": "account_name_123",
  "name": "Cash Account",
  "abbreviation": "CASH",
  "accId": "workspace_456",
  "userId": 789,
  "createdAt": 1621459200,
  "updatedAt": 1621545600
}
```

### Currency

```json
{
  "id": 42,
  "name": "US Dollar",
  "symbol": "$",
  "precision": 2,
  "type": "number",
  "accId": "workspace_456",
  "userId": 789,
  "createdAt": 1621459200
}
```

### Actor Account

```json
{
  "id": "account_123456",
  "actor_id": "actor_789012",
  "name_id": "account_name_345",
  "type": "operational",
  "income_type": "debit",
  "currency_id": 42,
  "amount": 1000.00,
  "hold_amount": 50.00,
  "status": "active",
  "min_limit": 0.00,
  "max_limit": 5000.00,
  "tree_calculation": false,
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Formula Account

```json
{
  "id": "account_654321",
  "actor_id": "actor_789012",
  "name_id": "account_name_678",
  "type": "analytical",
  "income_type": "debit",
  "currency_id": 42,
  "amount": 1500.00,
  "formula": "account_123456 + account_234567",
  "status": "active",
  "tree_calculation": false,
  "created_at": 1621459200,
  "updated_at": 1621545600
}
```

### Account Formula Relationship

```json
{
  "id": 789,
  "source_account_id": "account_123456",
  "calc_account_id": "account_654321",
  "user_id": 42,
  "created_at": 1621459200
}
```
