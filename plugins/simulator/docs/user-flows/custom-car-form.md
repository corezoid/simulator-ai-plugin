# Custom Car Form User Flow

This document walks through building a custom **Car** form, creating a car actor from it, and attaching accounts to that actor for both financial and non-financial tracking in the Simulator.Company platform.

## Overview

In Simulator.Company a **form** (also surfaced to end users as an **Account Template** / «Шаблон рахунків») defines the **field structure** that its actors instantiate. A form is an ordered array of **sections**, and each section holds an array of **fields**. Each actor created from the form stores its values in a free-form `data` object **keyed by the field `id`** (`item_<digits>`).

Accounts are **not** part of the form. In the v2 model accounts attach to **actors**, not forms — there is no form-level "account structure" that the system auto-creates. After you create a car actor you explicitly attach accounts to it (purchase value, maintenance, mileage, etc.) using the account tools.

This user flow demonstrates how to:

1. Create a custom **Car** form (`createForm`) with realistic field classes.
2. Create a **Car** actor (`createActor`) with `data` keyed by the form's field ids.
3. Attach **accounts to the actor** (`createCurrency` + `createAccountName` + `createAccount`).
4. Record value on an account (`createTransaction`).

> **Skills:** for guided help use the `simulator-forms` skill (form design), `simulator-actors` / `simulator-graph` (actor lifecycle), and `simulator-finance` (currencies, account names, accounts, transactions).

## Prerequisites

- A configured workspace id (`accId`). All create calls default to the configured workspace if `accId` is omitted.
- Authentication with the appropriate scopes:
  - `control.events:forms.management` for form operations
  - `control.events:actors.management` for actor operations
  - `control.events:accounts.management` for currency / account-name / account operations

## 1. Create the Car form

Create the form with `createForm(accId, isTemplate, title, sections, color?, description?, ref?)`. Each field in a section is `{id:"item_<digits>", class, title, visibility, …class-specific keys}`. The `id` is the stable key that actors use in their `data` — never the title.

Field classes used below:
- `edit` — text input; set `type` for typed input (`text`, `int`, `float`, `email`, `phone`, …).
- `select` — single-choice dropdown; static `options[]` of `{title, value, color?}`, or a dynamic source via `extra.optionsSource.type` (here `workspaceMembers` for the Owner field).

```json
{
  "accId": "<workspace-id>",
  "isTemplate": true,
  "title": "Car",
  "description": "Vehicle record with financial and usage accounts.",
  "color": "#409547",
  "ref": "car-form",
  "sections": [
    {
      "title": "Vehicle",
      "content": [
        { "id": "item_1001", "class": "edit", "type": "text", "title": "Make", "visibility": "visible" },
        { "id": "item_1002", "class": "edit", "type": "text", "title": "Model", "visibility": "visible" },
        { "id": "item_1003", "class": "edit", "type": "int", "title": "Year", "visibility": "visible" },
        {
          "id": "item_1004",
          "class": "select",
          "title": "Condition",
          "visibility": "visible",
          "options": [
            { "title": "New", "value": "new", "color": "#409547" },
            { "title": "Used", "value": "used", "color": "#f0a020" },
            { "title": "Salvage", "value": "salvage", "color": "#d03050" }
          ]
        },
        {
          "id": "item_1005",
          "class": "select",
          "title": "Owner",
          "visibility": "visible",
          "extra": { "optionsSource": { "type": "workspaceMembers" } }
        }
      ]
    }
  ]
}
```

`createForm` returns the new form's integer `id` (`formId`). You can read it back with `getForm(formId)` (pass `filter` to keep the response small) to verify the field ids before creating actors.

See [Forms](../entities/forms.md) and the `simulator-forms` skill.

## 2. Create a Car actor

Create the actor with `createActor(formId | formName, title, data, ref?, color?, contextLayerId?)`. The `data` object is keyed by the **field ids** from step 1, and each value's shape depends on the field's class:

- `edit` text → string
- `edit` int/float → **number**
- static `select` → **array of the chosen option object(s)** `[{title, value, color?}]`
- dynamic `select` with `workspaceMembers` → `[{type:"workspaceMembers", title, value:<userId number>}]`

```json
{
  "formId": 16952,
  "title": "Toyota Camry 2023",
  "ref": "vin-4T1BF1FK0CU511234",
  "color": "#409547",
  "data": {
    "item_1001": "Toyota",
    "item_1002": "Camry",
    "item_1003": 2023,
    "item_1004": [{ "title": "Used", "value": "used", "color": "#f0a020" }],
    "item_1005": [{ "type": "workspaceMembers", "title": "Jane Doe", "value": 42 }]
  }
}
```

`createActor` returns the actor's UUID (`actorId`). Read it back with `getActor(actorId)` (use `filter`) or `getActorByRef(formId, ref)` to verify.

See [Actors](../entities/actors.md) and the `simulator-actors` / `simulator-graph` skills.

## 3. Attach accounts to the actor

Accounts attach to the **actor** (`actorId`), not to the form. An account is the triple `(actorId, nameId, currencyId)` plus an `accountType`. So first create the building blocks once per workspace, then attach accounts to the car.

### 3a. Create currencies

Use `createCurrency(accId, name, symbol?, precision?)`. `precision` is **display-only** — amounts are stored as real decimal values and are **never** scaled by `10^precision` (e.g. precision `2` renders `1600` as `1600.00`, it does not mean `16.00`).

```json
{ "accId": "<workspace-id>", "name": "USD", "symbol": "$", "precision": 2 }
```

```json
{ "accId": "<workspace-id>", "name": "Km", "symbol": "km", "precision": 0 }
```

Each call returns a numeric currency `id` (`currencyId`). `getCurrencies(accId)` lists existing ones so you can reuse rather than duplicate.

### 3b. Create account names

Use `createAccountName(accId, name)` — the parameter is **`name`** (not `title`). Create one per logical account label.

```json
{ "accId": "<workspace-id>", "name": "Purchase Value" }
```

```json
{ "accId": "<workspace-id>", "name": "Maintenance" }
```

```json
{ "accId": "<workspace-id>", "name": "Mileage" }
```

Each call returns an account-name id (`nameId`). `getAccountNames(accId)` lists existing ones.

### 3c. Create the accounts on the actor

Use `createAccount(actorId, nameId, currencyId, accountType="fact", treeCalculation?, search?)`. Valid `accountType` values are `asset`, `liability`, `expense`, `income`, `counter`, `state`.

```json
{ "actorId": "<car-actor-id>", "nameId": "<purchase-value-name-id>", "currencyId": 101, "accountType": "asset" }
```

```json
{ "actorId": "<car-actor-id>", "nameId": "<maintenance-name-id>", "currencyId": 101, "accountType": "expense" }
```

```json
{ "actorId": "<car-actor-id>", "nameId": "<mileage-name-id>", "currencyId": 102, "accountType": "counter" }
```

This gives the car three accounts: **Purchase Value** (USD, asset), **Maintenance** (USD, expense), and **Mileage** (Km, counter). Use `getAccounts(actorId)` to list all accounts on the actor with their balances, and `getBalance(actorId, currencyId, nameId)` for a single account.

See [Accounts](../entities/accounts.md) (which also documents **currencies** and account names) and the `simulator-finance` skill.

## 4. Record a transaction

Record value with `createTransaction(accountId, amount, comment?, ref?)`. `amount` is the **real** signed value in the account's currency (e.g. `25000` means 25 000 USD) — it is stored as a decimal and is **never** scaled by the currency precision. Pass a stable `ref` to make the write idempotent.

Set the car's purchase value:

```json
{ "accountId": "<purchase-value-account-id>", "amount": 25000, "comment": "Purchase price", "ref": "car-vin-4T1...-purchase" }
```

Record a maintenance expense:

```json
{ "accountId": "<maintenance-account-id>", "amount": 320.50, "comment": "Oil change + brakes" }
```

Increment the mileage counter:

```json
{ "accountId": "<mileage-account-id>", "amount": 500, "comment": "Weekly mileage" }
```

List an account's history with `getTransactions(actorId, currencyId, nameId)`. For atomic moves between two accounts use `createTransfer`.

See [Transactions](../entities/transactions.md), [Counters](../entities/counters.md), and [Balances](../entities/balances.md).

## Putting it together

1. `createForm` → defines the Car field structure, returns `formId`.
2. `createActor` → instantiates one car with `data` keyed by the field ids, returns `actorId`.
3. `createCurrency` + `createAccountName` → workspace-level building blocks (reuse across cars).
4. `createAccount(actorId, …)` → attaches Purchase Value / Maintenance / Mileage accounts **to the car actor**.
5. `createTransaction` → records value on those accounts.

This same pattern adapts to any entity that needs a structured record plus per-instance financial or counter tracking.

## Related Documentation

- [Forms](../entities/forms.md) — field structure / Account Templates that actors instantiate
- [Actors](../entities/actors.md) — graph nodes; `data` keyed by form field ids
- [Accounts](../entities/accounts.md) — accounts attach to actors; also covers currencies and account names
- [Transactions](../entities/transactions.md) — recording value on accounts
- [Counters](../entities/counters.md) — counter-type accounts (e.g. mileage)
- [Balances](../entities/balances.md) — reading account balances
- [System Forms](../entities/system-forms.md) — predefined form templates

**Skills:** `simulator-forms`, `simulator-actors`, `simulator-graph`, `simulator-finance`.

## API Reference

For the full REST reference see the [Simulator.Company API Documentation](https://doc.simulator.company).
