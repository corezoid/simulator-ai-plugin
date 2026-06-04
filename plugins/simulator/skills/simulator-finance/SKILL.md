---
name: simulator-finance
description: >
  Simulator.Company financial management specialist. Use when the user wants to
  manage financial accounts (asset, liability, expense, income), record
  transactions, create transfers between accounts, work with currencies, set up
  account name definitions, use formula accounts, manage counters and state
  accounts, or generate financial reports. Activate when the user mentions
  "record transaction", "transfer funds", "account balance", "financial tracking",
  "depreciation", "expense", "budget", "counter", or "mileage tracking".
---

> **Curated tool names (v2 server).** Call tools by the exact names listed under "Curated tool set" in `/simulator`; a few examples below may still show older names.

# Simulator.Company Financial Manager

You are a specialist in the financial subsystem of Simulator.Company using the
`simulator` MCP server. Every actor can have multiple accounts for tracking
both financial and non-financial metrics.

## Workspace Context Check (MANDATORY FIRST STEP)

**Before doing anything else**, verify the WorkspaceID (`accId`) is known:

1. Check whether the user already specified `accId` (in the current message, conversation history, or session context).
2. If `accId` is **not** provided, immediately ask:

   > "В каком воркспейсе нужно работать? Укажите, пожалуйста, Workspace ID (`accId`)."

   Do **not** call any MCP tools until the user provides `accId`.
3. Once `accId` is known, proceed normally and use it in all subsequent API calls.

---

## Financial Architecture

```
Actor
  └── Accounts (many, each = name + currency + type)
        ├── Transactions (credits/debits on one account)
        └── Transfers    (atomic debit + credit across two accounts)
```

### Account Types

| Type | Description | Use Case |
|------|-------------|----------|
| `asset` | Things owned | Cash, equipment, vehicles |
| `liability` | Things owed | Loans, payables |
| `expense` | Costs incurred | Maintenance, fuel, salaries |
| `income` | Revenue earned | Sales, rent received |
| `counter` | Non-financial metric | Mileage, units produced, visits |
| `state` | Categorical value | Status code, stage index |
| `boolean` | True/false flag | Is active, is approved |

### Income Type (direction that increases balance)

| `incomeType` | Meaning |
|---|---|
| `credit` | Credits increase the balance (e.g. asset accounts — money IN) |
| `debit` | Debits increase the balance (e.g. expense accounts — more spending = higher) |

### Prerequisites

Before creating accounts, you need:
1. **Currency** (`currencyId`) — e.g. USD, EUR, Km, Units
2. **Account Name** (`nameId`) — category label (e.g. "Maintenance", "Budget")

These must exist in the workspace first.

---

## Currency Operations

### List Currencies
```
get-currencies-accId(accId="ws_xxx")
```

### Create Currency
```
post-currencies-accId(
  accId="ws_xxx",
  body='{"title": "USD", "symbol": "$", "decimals": 2}')
# Returns: {"id": "cur_xxx", "title": "USD", ...}

# Non-financial counter currencies:
post-currencies-accId(accId="ws_xxx", body='{"title": "Km", "symbol": "km", "decimals": 0}')
post-currencies-accId(accId="ws_xxx", body='{"title": "Units", "symbol": "u", "decimals": 0}')
```

> **`decimals` / `precision` is display only.** Amounts are stored as their real
> decimal value, not in minor units. `decimals: 2` just renders `1600` as `1600.00` —
> it does **not** mean the stored `1600` represents `16.00`. When you record or read an
> amount, pass/interpret the actual value (500 = 500 USD); never multiply or divide by
> `10^decimals`.

---

## Account Name Operations

### List Account Names
```
get-account_names-accId(accId="ws_xxx")
```

### Create Account Name
```
post-account_names-accId(
  accId="ws_xxx",
  body='{"title": "Purchase Value"}')
# Returns: {"id": "aname_xxx", "title": "Purchase Value"}

# Create account name + currency pair in one call
post-accounts-pair-accId(
  accId="ws_xxx",
  body='{"accountName": "Maintenance", "currencyName": "USD"}')
# Returns: {"accountName": {"id": "...", "title": "Maintenance"},
#            "currency":    {"id": "...", "title": "USD"}}
```

---

## Account Operations

### Create Account for an Actor
```
post-accounts-actorId(
  actorId="actor_xxx",
  body='{
    "nameId":     "aname_xxx",
    "currencyId": "cur_usd",
    "type":       "asset",
    "incomeType": "credit",
    "min":        0,
    "max":        null
  }')
# Returns: {"id": "acc_xxx", "amount": 0, ...}

# Create expense account
post-accounts-actorId(
  actorId="actor_xxx",
  body='{"nameId": "aname_maint", "currencyId": "cur_usd", "type": "expense", "incomeType": "debit"}')

# Create mileage counter
post-accounts-actorId(
  actorId="actor_xxx",
  body='{"nameId": "aname_mileage", "currencyId": "cur_km", "type": "counter", "incomeType": "debit"}')
```

### Get Accounts
```
# All accounts for an actor
get-accounts-actorId(actorId="actor_xxx")

# Single account by ID
get-accounts-single-accountId(accountId="acc_xxx")

# Account by actor ref
get-accounts-ref-formId-ref(formId="42", ref="car-toyota")

# Single account by actor ref (unique name+currency combination)
get-accounts-single-ref-formId-ref(formId="42", ref="car-toyota")

# Accounts by actor ID + currency + name
get-accounts-actorId-currencyId-nameId(actorId="actor_xxx", currencyId="cur_usd", nameId="aname_maint")

# Bulk get by IDs
get-accounts-bulk(ids="acc_1,acc_2,acc_3")

# Children accounts (actor hierarchy)
get-accounts-children-actorId(actorId="actor_xxx")
```

### Set Balance Directly
```
put-accounts-amount-accountId(
  accountId="acc_xxx",
  body='{"amount": 25000}')
```

### Formula Account
```
# Set a formula (for calculated accounts)
post-accounts-formula-accountId(
  accountId="acc_xxx",
  body='{"formula": "purchase_value - total_depreciation"}')

# Get formula info
get-accounts-formula_info-accountId(accountId="acc_xxx")
```

### Delete Account
```
delete-accounts-actorId-currencyId-nameId-accountType(
  actorId="actor_xxx",
  currencyId="cur_usd",
  nameId="aname_maint",
  accountType="expense")
```

---

## Transaction Operations

Transactions record a debit or credit on a **single account**.

### Create Transaction (immediate)
```
post-transactions-accountId(
  accountId="acc_xxx",
  body='{
    "amount":      1000,
    "description": "Initial purchase value",
    "ref":         "txn-initial-value",
    "data":        {"invoice": "INV-001"}
  }')
# Returns: {"id": "txn_xxx", "status": "completed", "amount": 1000, ...}
```

### 2-Step Transaction (authorize → complete/cancel)
```
# Step 1: Authorize (holds the funds)
post-transactions-accountId-authorized(
  accountId="acc_xxx",
  body='{"amount": 500, "description": "Pending maintenance", "ref": "txn-maint-pending"}')
# → status: "authorized", amount is held

# Step 2a: Complete (confirms the transaction)
post-transactions-accountId-completed(
  accountId="acc_xxx",
  body='{"transactionId": "txn_xxx"}')

# Step 2b: Cancel (reverses the hold)
post-transactions-accountId-canceled(
  accountId="acc_xxx",
  body='{"transactionId": "txn_xxx"}')
```

### Atomic Multi-Account Transactions
```
# Create multiple transactions atomically (all succeed or all fail)
post-transactions-atom-accId(
  accId="ws_xxx",
  body='[
    {"accountId": "acc_asset", "amount": -3000, "description": "Depreciation debit"},
    {"accountId": "acc_depr",  "amount":  3000, "description": "Depreciation credit"}
  ]')
```

### List Transactions
```
# By account
get-transactions-list-accountId(accountId="acc_xxx")

# By actor
get-transactions-actorId(actorId="actor_xxx")

# By actor ref
get-transactions-actor_ref-formId-actorRef(formId="42", actorRef="car-toyota")

# By transaction ref
get-transactions-ref-accountId-ref(accountId="acc_xxx", ref="txn-initial-value")

# Child transactions
get-transactions-children-transactionId(transactionId="txn_xxx")
```

---

## Transfer Operations

Transfers move value between two accounts atomically (one debits, one credits).

### Create Transfer (immediate)
```
post-transfers-accId(
  accId="ws_xxx",
  body='{
    "fromAccountId": "acc_source",
    "toAccountId":   "acc_dest",
    "amount":        500,
    "description":   "Budget reallocation",
    "ref":           "transfer-budget-q1"
  }')
```

### Create Transfer Holding (2-step)
```
# Step 1: Create authorized transfer (holds from source)
post-transfers-accId-authorized(
  accId="ws_xxx",
  body='{
    "fromAccountId": "acc_source",
    "toAccountId":   "acc_dest",
    "amount":        1000,
    "ref":           "transfer-pending"
  }')
# → transferId = "tr_xxx", status = "authorized"

# Step 2: Get transfer to verify
get-transfers-transferId(transferId="tr_xxx")
```

### Filter / Search Transfers
```
post-transfers-filter-accId(
  accId="ws_xxx",
  body='{
    "fromAccountId": "acc_source",
    "status":        "completed",
    "dateFrom":      1700000000,
    "dateTo":        1710000000
  }')
```

---

## Complete Example: Car Financial Tracking

### Setup (one-time per workspace)
```
# Create currencies
post-currencies-accId(accId="ws", body='{"title": "USD", "symbol": "$", "decimals": 2}')
post-currencies-accId(accId="ws", body='{"title": "Km",  "symbol": "km", "decimals": 0}')

# Create account name categories
post-account_names-accId(accId="ws", body='{"title": "Purchase Value"}')
post-account_names-accId(accId="ws", body='{"title": "Depreciation"}')
post-account_names-accId(accId="ws", body='{"title": "Maintenance"}')
post-account_names-accId(accId="ws", body='{"title": "Mileage"}')
```

### Per Car Actor: Initialize Accounts
```
car = "actor_camry_2023"

# Asset: purchase value
post-accounts-actorId(actorId=car,
  body='{"nameId": "<val_id>", "currencyId": "<usd_id>", "type": "asset", "incomeType": "credit"}')

# Expense: depreciation
post-accounts-actorId(actorId=car,
  body='{"nameId": "<dep_id>", "currencyId": "<usd_id>", "type": "expense", "incomeType": "debit"}')

# Expense: maintenance
post-accounts-actorId(actorId=car,
  body='{"nameId": "<mnt_id>", "currencyId": "<usd_id>", "type": "expense", "incomeType": "debit"}')

# Counter: mileage
post-accounts-actorId(actorId=car,
  body='{"nameId": "<mil_id>", "currencyId": "<km_id>", "type": "counter", "incomeType": "debit"}')

# Record initial purchase value
post-transactions-accountId(
  accountId="<acc_val_id>",
  body='{"amount": 25000, "description": "Initial purchase", "ref": "purchase-2023"}')
```

### Record Expenses
```
# Maintenance expense
post-transactions-accountId(
  accountId="acc_mnt_xxx",
  body='{"amount": 450, "description": "Oil change + filters", "ref": "service-jan-2024"}')

# Annual depreciation (3000 USD)
post-transactions-accountId(
  accountId="acc_dep_xxx",
  body='{"amount": 3000, "description": "Annual depreciation 2023"}')

# Add mileage (counter) — set absolute odometer reading
put-accounts-amount-accountId(
  accountId="acc_mil_xxx",
  body='{"amount": 45230}')
```

### Get Financial Report
```
# Get all accounts with balances
getAccounts(actorId="actor_camry_2023")
# → [{type: "asset", amount: 25000}, {type: "expense", amount: 450}, ...]

# Account turnover over a period: pass from/to (unixtime in MILLISECONDS).
# The returned amounts are the account's turnover for that window.
getAccounts(actorId="actor_camry_2023", from=1704067200000, to=1706745600000)
# Optionally restrict direction with incomeType="debit"|"credit".

# Get maintenance transaction history
get-transactions-list-accountId(accountId="acc_mnt_xxx")
```

### Rank / filter actors by account balance
Use `filterActors` to find the actors of a form whose account balance crosses a threshold,
or to rank them — in one server-side query (no per-actor balance reads).
```
# Top clients by "Cash" balance:
filterActors(formId=42, accountNameId="name_cash", currencyId=1,
             orderBy="balance", orderValue="DESC", withStats=true)

# Clients with Cash balance > 10000:
filterActors(formId=42, accountNameId="name_cash", currencyId=1, amountFrom=10000)

# Scope to one anchor actor's related actors (graph neighbours along the hierarchy
# link) — "related actors of X whose Cash balance is below 100":
filterActors(formId=42, linkedToActorId="actor_parent", accountNameId="name_cash",
             currencyId=1, amountTo=100)
```
`amountFrom` = balance ≥, `amountTo` = balance ≤. Filters on CURRENT balance only — for
turnover over a period, read each actor's accounts with `getAccounts(from, to)`.

---

## Reference Documents

Use the `Read` tool to load these files when you need more detail:

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/accounts.md` | Account types, income types, tree calculation, formulas |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transactions.md` | Transaction states, 2-step flow, atomic transactions |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transfers.md` | Transfer mechanics, holding, filtering |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/balances.md` | Balance history, credit/debit split |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/counters.md` | ScyllaDB counters, time-series metrics |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/custom-car-form.md` | Complete financial tracking example (car with purchase value, depreciation, mileage) |

## Tips

- **Amounts are real decimal values, not minor units** — `amount: 500` on a USD account is 500 USD. Currency `decimals`/`precision` only affects UI rounding; never scale amounts by `10^decimals` when writing or reading them
- **Always create currency and account name before creating an account** — both `currencyId` and `nameId` are required
- Use `post-accounts-pair-accId` to create both name and currency together
- For financial accounts: `asset/income` typically use `incomeType: credit`; `expense/liability` use `incomeType: debit`
- Use `counter` type for non-monetary metrics (km, units, visits) — they're not financial but follow the same API
- `put-accounts-amount-accountId` sets the absolute value (good for counters/odometers), transactions add incrementally
- Use `post-transactions-atom-accId` for accounting entries that must be balanced (double-entry bookkeeping)
- 2-step transactions are reversible — prefer them for pending/draft operations
- `get-accounts-children-actorId` aggregates accounts up the actor hierarchy (if `treeCalculation: true`)
