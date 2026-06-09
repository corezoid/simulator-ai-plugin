---
name: simulator-finance
description: >
  Simulator.Company financial management specialist. Use when the user wants to manage
  financial and metric accounts on actors (balances, counters, plan vs fact), record
  transactions, transfer value between accounts, work with currencies and account-name
  categories, track non-financial metrics, or read balances and turnover. Activate when the
  user mentions "record transaction", "transfer funds", "account balance", "financial
  tracking", "depreciation", "expense", "budget", "counter", "mileage tracking", "запиши
  транзакцію", "переказ коштів", "баланс рахунку", "лічильник", "пробіг", "запиши
  транзакцию", "перевод средств", "баланс счёта", "счётчик", "пробег". Accounts attach to
  ACTORS — use `simulator-actors` to create/find the actor first; for sharing an account use
  `simulator-access`; for dashboards use `simulator-charts`.
---

> **Curated tool names (v2 server) — call these EXACT names.**
>
> | Operation | Tools |
> |---|---|
> | Accounts | `createAccount` `getAccount` `getAccounts` `getBalance` `getChildAccounts` `updateAccount` `setAccountAmount` `deleteAccount` |
> | Currencies | `createCurrency` `getCurrencies` `searchCurrencies` |
> | Account names | `createAccountName` `getAccountNames` `updateAccountName` `searchAccountNames` |
> | Counters | `saveCounters` `setCounters` `getCounters` |
> | Transactions | `createTransaction` `finalizeTransaction` `atomCreateTransaction` `getTransactions` `getAccountTransactions` `getTransactionByRef` |
> | Transfers | `createTransfer` `createTransferTwoStep` `getTransfer` `getTransferByRef` `filterTransfers` |
>
> Tools take **typed named arguments** (not a JSON `body` string). `accId` defaults to the
> configured workspace if omitted.
>
> For **user-to-user** money movement you also need `searchUsers` / `getUsers` (find the userId)
> and `getSystemActor` (the user's twin actor) — see "Transfers between users" below.

# Simulator.Company Financial Manager

You manage the financial subsystem via the `simulator` MCP server. **Accounts live on actors** —
each actor can have many accounts, one per `(accountName, currency, accountType)`.

```
Actor
  └── Account  (nameId + currencyId + accountType)
        ├── Transactions  (debits/credits on ONE account, with history)
        └── Transfers      (atomic debit-leg(s) + credit-leg(s) across accounts)
Counters     (Scylla-backed tallies — accounts WITHOUT transaction history; see below)
```

## Workspace Context Check (MANDATORY FIRST STEP)

Before any tool call, verify the Workspace ID (`accId`) is known. If the user has not provided
it (in the message, history, or session):

> Ask the user — **in their own language** (English, Ukrainian, or Russian) — which workspace to
> work in, i.e. for the Workspace ID (`accId`).

Do not call workspace-scoped tools until `accId` is known (it then defaults for the rest).

> **Relationship to the other skills**
> - **`simulator-actors`** — create/find the actor an account hangs off (you need its `actorId`).
> - **`simulator-access`** — share an account / grant who may view or modify it.
> - **`simulator-charts`** — visualise balances/turnover on a layer.

---

## Core concepts

### Amounts are REAL decimal values — never minor units

`amount: 500` on a USD account means **500 USD**. A currency's `precision` (a.k.a. decimals) is
**display only**: `precision: 2` renders `1600` as `1600.00` — it does **not** mean stored `1600`
is `16.00`. When you write or read an amount, use the actual value; **never** multiply/divide by
`10^precision`. (`highPrecision=true` on reads returns extra fractional digits.)

### What types an account: `accountType` + `counterType` (NOT asset/liability)

There is **no** asset/liability/expense/income enum on an account. What an account *means*
(cash, maintenance, revenue, …) is carried by its **account name** (`nameId`). An account is
typed by two orthogonal dimensions:

**`accountType`** — the *value* type. Default **`fact`**.

| `accountType` | Meaning |
|---|---|
| `fact` | The actual recorded balance (the normal case — usually omit, defaults to fact) |
| `plan` | A planned / budgeted figure (track plan vs fact) |
| `min` / `max` / `avg` | Aggregates over child actors' values |

**`counterType`** — whether it is a normal balance or a fast counter. Default **`amount`**.

| `counterType` | Meaning |
|---|---|
| `amount` | Normal balance account, full transaction history |
| `counter` / `uniqCounter` | Scylla-backed tally, NO history (analytics/anti-fraud); see Counters |
| `systemCounter` | System-managed counter |

So a normal account is `accountType=fact, counterType=amount` (both defaults — you can omit
both). A mileage counter is `counterType=counter`.

### incomeType (which direction raises the balance)

`credit` = credits increase the balance (money-in accounts); `debit` = debits increase it
(cost accounts). It is a **read / counter dimension** (`getAccounts`, `getTransactions`,
`saveCounters` filters), **not** a `createAccount` argument.

### Prerequisites for an account

A **currency** (`currencyId`) and an **account name** (`nameId`) must exist in the workspace
first. There is **no** single "account+currency pair" tool — create the two separately, then
attach the account to an actor.

**One more prerequisite that is easy to miss: pair access.** The `(nameId, currencyId)` pair —
written `<nameId>_<currencyId>` — is its own access-controlled object (`objType="account"`).
`createAccount` only checks access to the **actor**, so it happily attaches the account, but it
does **not** grant you access to the *pair*. Every per-account operation afterwards
(`getBalance`, `getAccount`, `setAccountAmount`, `createTransaction`, transfers) re-checks
**pair** access and will return **`403 Access Denied`** if you have none — unless you are the
workspace **Owner**, who bypasses the check. See **Account-pair access** below before you write
balances. (This is why creating an account "succeeds" yet the very next transaction 403s.)

---

## Currencies

```
getCurrencies(accId="ws_xxx", filter="id,name,symbol")
searchCurrencies(accId="ws_xxx", query="USD")          # find one by name

createCurrency(accId="ws_xxx", name="USD", symbol="$", precision=2)
# Non-financial units for counters:
createCurrency(accId="ws_xxx", name="Km",    symbol="km", precision=0)
createCurrency(accId="ws_xxx", name="Units", symbol="u",  precision=0)
```

Args are `name` / `symbol` / `precision` (not `title` / `decimals`).

## Account names (categories)

```
getAccountNames(accId="ws_xxx", filter="id,name")
searchAccountNames(accId="ws_xxx", query="maint")

createAccountName(accId="ws_xxx", name="Maintenance", abbreviation="MNT")
updateAccountName(nameId="<account-name id>", name="Maintenance costs")
```

---

## Accounts (on an actor)

```
# Attach an account to an actor (currency + name must already exist).
# accountType defaults to fact and counterType to amount — omit both for a normal account.
createAccount(actorId="<actor UUID>", nameId="<account-name id>", currencyId=1,
              treeCalculation=false,         # aggregate child-actor balances into this one
              minLimit=0, maxLimit=null,     # optional balance limits
              ignoreIfExist=true)            # don't error if it already exists

# A counter account (fast Scylla tally, no history): set counterType
createAccount(actorId="<actor UUID>", nameId="<mileage name>", currencyId=2, counterType="counter")

# Read
getAccounts(actorId="<actor UUID>", filter="id,amount,currencyId,nameId")   # all on the actor
getAccount(accountId="<account id>", trsCount=true)                          # one by id
getBalance(actorId="<actor UUID>", currencyId=1, nameId="<account-name id>") # one by coordinates
getChildAccounts(actorId="<actor UUID>", nameId="<account-name id>", currencyId="1") # same account on child actors

# Settings / fixed balance (accountType is part of the account key — usually "fact")
updateAccount(actorId="<actor UUID>", currencyId=1, nameId="<aname>", accountType="fact",
              treeCalculation=true, minLimit=0)
setAccountAmount(accountId="<account id>", amount=25000)   # set a FIXED balance (correction) — confirm first

# Delete (irreversible — confirm first)
deleteAccount(actorId="<actor UUID>", currencyId=1, nameId="<aname>", accountType="fact")
```

- `setAccountAmount` overrides the balance to a fixed value; `createTransaction` changes it incrementally with history. Prefer transactions for anything auditable.
- `getAccounts(from, to)` (unixtime **milliseconds**) returns each account's **turnover** for that window instead of the live balance.

> **Formula / block accounts** are documented in `accounts.md` but are **not** curated MCP tools
> yet — configure those in the UI; don't fabricate a tool call.

---

## Account-pair access (the usual cause of `403 Access Denied`)

Account-level access is **not** granted per actor — it is granted on the workspace-level
**pair** `<nameId>_<currencyId>` (`objType="account"`). The server resolves the pair's
workspace via its account-name and then allows the call only if the caller is the workspace
**Owner**, the pair is public+view, **or** the caller has an explicit access rule on that pair.
`createAccount` is gated on **actor** access only and never seeds a pair access rule, so a
non-owner (Admin / Member / Guest) who attaches an account still gets **403** on `getBalance`,
`getAccount`, `setAccountAmount`, `createTransaction` and transfers. (Workspace owners don't see
this because the owner shortcut grants view/modify on everything. System account-names are also
exempt for `view`.)

**Bootstrapping your own access to a fresh pair — `POST /accounts/pair/{accId}`.** This is the
only call that *self-grants* access to a pair (it creates the account name + currency if missing
and grants the **caller** view+modify+remove), so it is what makes a non-owner's accounts usable.
The UI modal `CreateActorAccount` runs it (check pair → create pair → manage access → attach to
actor). It is **not yet a curated MCP tool** — until one is added, a non-owner reaches it via the
UI or a direct call:

```
POST {apiBaseURL}/accounts/pair/{accId}
  Authorization: Simulator <token>
  { "accountName": "Deal Value", "currencyName": "USD" }
# → creates the (name, currency) pair if needed and grants YOU access to it.
# Afterwards createAccount + createTransaction + getBalance on that pair work.
```

If the pair **already** has access rules and you are not among them, it returns **403** — then a
workspace **Owner** (or an existing grantee) must grant you. Workspace Owners skip all of this
(the owner shortcut covers them). To manage who else may use a pair you already hold:

```
# Grant a user/group access to ONE pair. objId = "<nameId>_<currencyId>" (composite, NOT the per-row account UUID).
saveAccessRules(
  objType="account",
  objId="<nameId>_<currencyId>",
  rules=[{ "action": "create",
           "data": { "saId": <saId>,                 # or userId / groupId
                     "privs": { "view": true, "modify": true, "remove": true } } }])

# Grant a WHOLE account category (all currencies of a name) across every actor.
# objId is the account-NAME id prefix — it matches all accounts named "<nameId>_*".
bulkSaveAccountPairsAccessRules(
  accId="ws_xxx",
  items=[{ "objId": "<nameId>",
           "rules": [{ "action": "create",
                       "data": { "saId": <saId>, "privs": { "view": true, "modify": true } } }] }])

# Inspect who currently has pair access (use the composite pair id, NOT the per-row account UUID):
getAccessRules(objType="account", objId="<nameId>_<currencyId>")
```

> `saveAccessRules` / `bulkSaveAccountPairsAccessRules` require you to **already** hold the pair
> (or be the workspace Owner) — they do **not** self-grant. For your own first access to a brand
> new pair, the `/accounts/pair` endpoint is the only bootstrap.

Notes & gotchas:
- The pair id is `<nameId>_<currencyId>` (e.g. `c16d1bd0-…-…_933`), **not** the per-row account
  UUID that `createAccount` returns. Using the bare UUID with the access-rule tools also 403s.
- Granting pair access itself needs access to grant: only the workspace **Owner** (or someone who
  already holds the pair) can seed the first rule. If you are a non-owner facing a pair nobody can
  reach, an Owner must grant it (or create it through the UI's `/accounts/pair` flow, which
  self-grants). Making the user the workspace Owner also resolves it globally.
- This pair model is why account ops can 403 on a fresh local workspace where the user is only a
  Member/Guest, while the same flow works on cloud where the user is the workspace Owner.

---

## Transactions (one account, with history)

```
# Immediate. `ref` makes it idempotent (safe to retry). Note: `comment`, not `description`.
createTransaction(accountId="<account id>", amount=450, comment="Oil change",
                  ref="service-jan-2024", data={"invoice": "INV-001"})

# 2-step hold → finalize. Step 1 authorizes; pass a `ref` so you can finalize by it.
createTransaction(accountId="<account id>", amount=500, comment="Pending maintenance",
                  ref="maint-hold-1", expiration=1735689600)   # creates an 'authorized' hold
finalizeTransaction(accountId="<account id>", type="completed", ref="maint-hold-1")  # or type="canceled"

# Atomic multi-leg (all-or-nothing) — e.g. double-entry depreciation
atomCreateTransaction(accId="ws_xxx", items=[
  { "accountId": "<asset acc>", "amount": -3000, "comment": "Depreciation debit", "ref": "dep-2023-a" },
  { "accountId": "<depr acc>",  "amount":  3000, "comment": "Depreciation credit", "ref": "dep-2023-b" }
])

# Read
getTransactions(actorId="<actor UUID>", currencyId=1, nameId="<aname>", limit=20)  # currencyId+nameId required
getAccountTransactions(accountId="<account id>", limit=20, offset=0)               # limit+offset BOTH required
getTransactionByRef(accountId="<account id>", ref="service-jan-2024")             # confirm a write landed
```

`finalizeTransaction` `type` ∈ `completed` | `canceled` (also `declined`/`reversed`); identify the
hold by its `ref` (or `parentRef`), not a transaction id.

---

## Transfers (move value between accounts)

> **Rule: any movement of value BETWEEN accounts uses a TRANSFER, not a transaction.**
> `createTransaction` writes to **one** account. The moment value moves between **two or more
> accounts** — whether they belong to **different actors** *or* are **two accounts of the same
> actor** — use `createTransfer` (or `createTransferTwoStep` for a hold). The `from` legs debit,
> the `to` legs credit, and the whole thing is atomic.

A transfer has **`from`** (debit legs) and **`to`** (credit legs) — each an array of
`{accountId, amount, ...}`. It is atomic.

```
# Immediate
createTransfer(accId="ws_xxx",
  from=[{ "accountId": "<source acc>", "amount": 200 }],
  to=[{   "accountId": "<dest acc>",   "amount": 200 }],
  comment="Budget reallocation", ref="transfer-q1")

# 2-step: authorize a hold, then complete/cancel
createTransferTwoStep(accId="ws_xxx", type="authorized",
  from=[{ "accountId": "<source acc>", "amount": 1000 }],
  to=[{   "accountId": "<dest acc>",   "amount": 1000 }],
  ref="payout-99")
createTransferTwoStep(accId="ws_xxx", type="completed", transferId="<tr id>")   # or type="canceled"

# Read
getTransfer(transferId="<tr id>", filter="id,amount,createdAt,comment")
getTransferByRef(accId="ws_xxx", ref="payout-99")
filterTransfers(accId="ws_xxx", from=1704067200, to=1706745600,         # window (unixtime)
                incomeType="debit", amount=200, oper="gte", limit=50)
```

### Transfers between users (via their twin actors)

Every workspace **user has a system "twin" actor** that carries their accounts. To move value
between users (or between a user and any other actor), you transfer between **their twin actors'
accounts** — there is no "transfer to a userId" call. The flow:

1. **Find the user(s) by name** → `searchUsers(accId, query)` (or `getUsers`) → get each `userId`.
2. **Resolve each user's twin actor** → `getSystemActor(accId, objType="user", objId="<userId>")`
   → the user's actor (and its `id`).
3. **Find the account on each twin actor** → `getAccounts(actorId=<twin actor>)` to get the
   `accountId` for the agreed `(accountName, currency)` (create it with `createAccount` if missing).
4. **Transfer** between those accounts with `createTransfer` (the `from`/`to` legs reference the
   twin actors' `accountId`s).

```
# "Transfer 100 USD from Olena to Petro on their Wallet accounts"
searchUsers(accId="ws", query="Olena")        → userId 4210
searchUsers(accId="ws", query="Petro")        → userId 4310
getSystemActor(accId="ws", objType="user", objId="4210")  → actor A (Olena's twin)
getSystemActor(accId="ws", objType="user", objId="4310")  → actor B (Petro's twin)
getAccounts(actorId="<A>")  → Olena's Wallet accountId  accO
getAccounts(actorId="<B>")  → Petro's Wallet accountId  accP
createTransfer(accId="ws",
  from=[{ "accountId": "accO", "amount": 100 }],
  to=[{   "accountId": "accP", "amount": 100 }],
  comment="Olena → Petro", ref="o2p-1")
```

For **more than two** parties, add more legs to `from`/`to` (the totals must balance), e.g.
split one debit across several credit legs. Multi-account moves on a **single** actor work the
same way — just reference that actor's several `accountId`s in the legs.

---

## Counters (Scylla-backed analytical tallies)

**Counter accounts are accounts**, but a special fast kind: their balances live in **ScyllaDB**
and they keep **no per-transaction history**. They are cheap, high-throughput running counts/sums
for **analytics and anti-fraud** — not an auditable ledger. Use the regular account + transaction
tools when you need history; use the counters API for fast totals.

- Addressed by `(formId, actorRef, accountName, currency, incomeType)` — **not** by account id.
- `counter` = running sum (record deltas via `amount`); `uniqCounter` = dedupes by `trsRef`
  (the same event counted twice lands once).

```
# Record a mileage reading (running counter)
saveCounters(accId="ws_xxx", openAccounts=true, items=[
  { "formId": 42, "actorRef": "car-camry", "accountName": "mileage", "currency": "Km",
    "incomeType": "debit", "type": "counter", "amount": 45000 }])

# Override a counter to a fixed value
setCounters(accId="ws_xxx", items=[
  { "formId": 42, "actorRef": "car-camry", "accountName": "mileage", "currency": "Km",
    "incomeType": "debit", "amount": 50000 }])

# Read counter values
getCounters(accId="ws_xxx", items=[
  { "formId": 42, "actorRef": "car-camry", "accountName": "mileage", "currency": "Km",
    "incomeType": "debit", "type": "counter" }])
```

> Two ways to get a counter, don't confuse them: a **counter account** created with
> `createAccount(... counterType="counter")` (addressed later by account id / actor+name like any
> account) vs the **counters API** above (`saveCounters`/`getCounters`, addressed by
> actorRef+accountName, bulk). Both are Scylla-backed and history-less; use the counters API for
> fast bulk analytics/anti-fraud counts, `createAccount(counterType=…)` to attach one to an actor.

---

## End-to-end: car financial tracking

```
# 1. One-time workspace setup
createCurrency(accId="ws", name="USD", symbol="$", precision=2)        → currencyId 1
createCurrency(accId="ws", name="Km",  symbol="km", precision=0)        → currencyId 2
createAccountName(accId="ws", name="Purchase Value")                    → nameId <val>
createAccountName(accId="ws", name="Depreciation")                      → nameId <dep>
createAccountName(accId="ws", name="Maintenance")                       → nameId <mnt>
createAccountName(accId="ws", name="Mileage")                           → nameId <mil>

# 2. The actor (a simulator-actors task) — get its UUID, e.g. searchActors / createActor
car = "<car actor UUID>"

# 3. Attach accounts (the NAME carries the meaning; accountType defaults to fact)
createAccount(actorId=car, nameId="<val>", currencyId=1)                      # Purchase Value
createAccount(actorId=car, nameId="<dep>", currencyId=1)                      # Depreciation
createAccount(actorId=car, nameId="<mnt>", currencyId=1)                      # Maintenance
createAccount(actorId=car, nameId="<mil>", currencyId=2, counterType="counter")  # Mileage (counter)

# 4. Record value (find each account's id via getAccounts(actorId=car))
createTransaction(accountId="<acc_val>", amount=25000, comment="Initial purchase", ref="purchase-2023")
createTransaction(accountId="<acc_mnt>", amount=450,   comment="Oil change",        ref="service-jan-2024")
atomCreateTransaction(accId="ws", items=[                       # balanced depreciation entry
  { "accountId": "<acc_val>", "amount": -3000, "ref": "dep-2023-a" },
  { "accountId": "<acc_dep>", "amount":  3000, "ref": "dep-2023-b" }])
setAccountAmount(accountId="<acc_mil>", amount=45230)           # odometer reading (counter account)

# 5. Report
getAccounts(actorId=car)                                        # live balances per account
getAccounts(actorId=car, from=1704067200000, to=1706745600000) # turnover for the window (MILLISECONDS)
getAccountTransactions(accountId="<acc_mnt>", limit=50, offset=0)
```

## Rank / filter actors by balance (a `simulator-actors` tool)

`filterActors` finds/ranks a form's actors by an account balance in one server-side query:

```
filterActors(formId=42, accountNameId="<aname>", currencyId=1,
             orderBy="balance", orderValue="DESC", withStats=true)   # top by balance
filterActors(formId=42, accountNameId="<aname>", currencyId=1, amountFrom=10000) # balance ≥ 10000
```

`amountFrom` = balance ≥, `amountTo` = balance ≤; CURRENT balance only (for turnover use
`getAccounts(from, to)`). Full filter/data semantics live in `simulator-actors`.

---

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/accounts.md` | Account types, income types, tree calculation, formulas |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transactions.md` | Transaction states, 2-step flow, atomic transactions |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/transfers.md` | Transfer mechanics, holds, filtering |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/balances.md` | Balance history, credit/debit split |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/counters.md` | ScyllaDB counters, time-series metrics |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/custom-car-form.md` | Complete financial-tracking example |

## Tips

- **Amounts are real decimal values** — `amount: 500` is 500 USD; never scale by `10^precision`.
- **Create the currency and account name before the account** — `currencyId` and `nameId` are required; there is no account+currency "pair" tool.
- `accountType` is the **value** type (`fact` default | `plan` | `min`/`max`/`avg`) and `counterType` selects `amount` (default) vs `counter`/`uniqCounter`; the account **name** carries the cash/expense/revenue meaning. There is no asset/liability/income enum. `incomeType` (debit/credit) is a read/counter dimension, not a `createAccount` arg.
- Transactions use **`comment`** (not `description`); pass a stable **`ref`** for idempotency and to finalize 2-step holds.
- Transfers use **`from`/`to` arrays of legs** `{accountId, amount}` — not `fromAccountId`/`toAccountId`.
- `setAccountAmount` sets a fixed balance (correction); `createTransaction` moves it with history. Prefer transactions when audit history matters.
- `atomCreateTransaction` for entries that must balance (double-entry).
- `getAccountTransactions` requires **both** `limit` and `offset`; `getTransactions` requires `currencyId` + `nameId`.
- **Counters API** = Scylla tallies, no history (analytics/anti-fraud); distinct from `counter`-type accounts.
- **Save tokens with `filter`** on reads (`getAccount`, `getAccounts`, `getBalance`, `getTransactions`, `getTransfer`, `getCurrencies`, `getAccountNames`) — a comma-separated field allow-list (`filter="id,amount,currencyId"`), NOT a row filter.
- Sharing an account (who can view/modify) → `simulator-access`; the actor itself → `simulator-actors`.
