# Custom Car Form User Flow

This document describes how to create a custom form for cars and then create actors from this form with appropriate accounts in the Simulator.Company platform.

## Overview

The Simulator.Company platform allows you to create custom forms for any entity type, including vehicles like cars. These forms define the structure and behavior of actors created from them, including:

- Data fields and their types
- Validation rules
- Default values
- Associated accounts for both financial and non-financial tracking

This user flow demonstrates how to:
1. Create a custom form for cars
2. Define account structures for the form
3. Create car actors using the form
4. Manage both financial and non-financial aspects of car actors through accounts

## Prerequisites

Before using the custom form API endpoints, you need:

1. Authentication token with appropriate scopes:
   - `control.events:forms.management` for form operations
   - `control.events:actors.management` for actor operations
   - `control.events:accounts.management` for account operations

2. Knowledge of the system forms used for custom forms, particularly:
   - Scripts/Smart Forms/CDU
   - Accounts
   - Currencies
   - Transactions

## Creating a Custom Car Form

The first step is to create a custom form that defines the structure for car actors. This is done using the form creation API.

For detailed information about creating forms, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

A car form typically includes fields such as:
- Make
- Model
- Year
- Color
- VIN (Vehicle Identification Number)
- Registration number
- Purchase price
- Current value
- Maintenance history

These fields will be stored in the actor's `data` JSON field in the database, as defined in the Actors model. Each actor created from this form will have a unique ID, reference, title, and other metadata fields that are automatically managed by the system.

Once the form is created, you can retrieve it using the form retrieval API. This returns the form definition, including all fields, validation rules, and account structures. You can use this to verify that your car form was created correctly.

For detailed information about forms, see [Forms](../entities/forms.md).

## Creating Currencies and Account Name-Currency Pairs

Before defining account structures for car actors, you need to create currencies and account name-currency pairs. These are prerequisites for creating accounts in the Simulator.Company platform.

### Creating Currencies

To create a new currency that can be used for car-related accounts, use the currency creation API. You'll typically define:

- Currency name (e.g., "USD", "EUR", or custom currencies like "CarPoints")
- Currency symbol
- Decimal places
- Other currency properties

For detailed request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

### Creating Account Name-Currency Pairs

To create an account name-currency pair, which is required before creating accounts for car actors, use the account name-currency pair API. The pair links a specific account name (e.g., "Maintenance", "Fuel", "Mileage") with a currency.

For example:
```json
{
  "accountName": "CarMaintenance",
  "currencyName": "USD"
}
```

The response will include IDs for both the account name and currency, which you'll use when creating accounts for car actors:

```json
{
  "accountName": {
    "id": "account-name-id",
    "title": "CarMaintenance"
  },
  "currency": {
    "id": "currency-id",
    "title": "USD"
  }
}
```

For detailed information about currencies, see [Currencies](../entities/currencies.md).

## Defining Account Structures

When creating a custom form for cars, you can define account structures that will be automatically created for each car actor. These accounts can track both financial and non-financial aspects:

### Financial Aspects:
- Purchase value (asset type)
- Depreciation (expense type)
- Maintenance costs (expense type)
- Fuel expenses (expense type)
- Insurance costs (expense type)

### Non-Financial Aspects:
- Mileage tracking (counter type)
- Service intervals (counter type)
- Performance metrics (counter type)
- Usage statistics (counter type)
- Status indicators (state type)
- Feature availability (boolean type)

Each account is linked to the actor via the `actor_id` field in the ActorsAccounts model. Accounts have specific types (asset, expense, liability, counter, state, etc.), income types (debit/credit), and are associated with a currency and account name through the name-currency pair. You can also define minimum and maximum limits for accounts, and use formula-based calculations for derived values.

For detailed information about accounts, see [Accounts](../entities/accounts.md).

## Creating Car Actors

Once the custom form is created, you can create car actors using the form. This is done using the actor creation API.

For detailed information about creating actors, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

Each car actor represents a specific vehicle with its own data and accounts. The actor will be stored in the database with the following key fields:

- `id`: Unique identifier for the actor
- `form_id`: Reference to the car form template
- `title`: Name of the specific car (e.g., "Toyota Camry 2023")
- `data`: JSON object containing all the car-specific fields (make, model, year, etc.)
- `meta_info`: Additional metadata about the car
- Various system fields for tracking creation date, status, etc.

When an actor is created, the system can automatically create the account structures defined in the form, linking them to the actor through the `actor_id` field in the ActorsAccounts table.

Once the car actor is created, you can retrieve it using the actor retrieval API. This returns the car actor with all its data, including the car-specific fields defined in the form. You can use this to verify that your car actor was created correctly.

**Additional Actor Operations:**

- Update an existing car actor
- Delete a car actor
- Retrieve a car actor by its reference

For all these operations, refer to the [Simulator.Company API Documentation](https://doc.simulator.company) for detailed request and response formats.

For detailed information about actors, see [Actors](../entities/actors.md).

## Managing Car Accounts

Car actors have associated accounts that can be used to track both financial and non-financial aspects of the vehicle. These accounts are created and managed using the accounts API.

For detailed information about creating and managing accounts, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

You can create accounts for a car actor, linking them to the actor through the `actor_id` field in the ActorsAccounts table. When creating accounts, you can specify the account name, currency, and type (asset, expense, counter, state, etc.).

You can also retrieve all accounts associated with a car actor, allowing you to view balances, counters, states, and other account details.

**Account Operations for Financial Aspects:**

- Record transactions for expenses
- Track depreciation
- Calculate total cost of ownership
- Generate financial reports

**Account Operations for Non-Financial Aspects:**

- Track mileage and service intervals
- Monitor performance metrics
- Record maintenance history
- Track feature usage and availability
- Store vehicle state information

Each account is stored in the ActorsAccounts table with fields for:
- `amount`: Current balance or counter value
- `hold_amount`: Amount on hold (for pending transactions)
- `currency_id`: Reference to the currency
- `type`: Account type (asset, expense, counter, state, etc.)
- `income_type`: Whether credits or debits increase the balance
- `tree_calculation`: Whether the account participates in hierarchical calculations
- `formula`: Mathematical expression for calculated accounts
- Additional metadata fields for non-financial tracking

**Additional Account Operations:**

- Retrieve a specific account by its ID
- Update account properties like limits, formula, or counter values
- Delete accounts associated with a car actor

For all these operations, refer to the [Simulator.Company API Documentation](https://doc.simulator.company) for detailed request and response formats.

For detailed information about transactions and other account operations, see:
- [Transactions](../entities/transactions.md)
- [Counters](../entities/counters.md)
- [Balances](../entities/balances.md)

## Complete User Flow Example

The following example demonstrates the complete process of creating a custom car form and using it to manage car actors with financial tracking.

### 1. Create a Custom Car Form

To create a custom form for cars, use the form creation API. This API accepts a form definition that includes fields, validation rules, and account structures.

For a car form, you'll typically define:

1. **Basic form metadata** - Title, description, and reference
2. **Field definitions** - Data fields for car properties (make, model, year, etc.)
3. **Account structures** - Financial accounts for tracking car value and expenses
4. **Validation rules** - Rules for ensuring data integrity

The form definition should include all necessary fields and account definitions for tracking cars. For the complete request format and parameters, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

After creating the form, you'll receive a response that includes the form ID, which you'll use when creating car actors from this form. You can verify the form was created correctly using the form retrieval API.

**Additional Form Operations:**

- Update an existing car form
- Delete a car form (this will not affect existing car actors)
- Search for car forms by title or description

For all these operations, refer to the [Simulator.Company API Documentation](https://doc.simulator.company) for detailed request and response formats.

### 2. Create a Car Actor

To create a car actor, use the actor creation API. This API accepts a request body that includes the car's title, description, and data fields.

For a car actor, you'll typically include:

1. **Title** - A human-readable name for the car (e.g., "Toyota Camry 2023")
2. **Description** - Optional additional information about the car
3. **Data** - An object containing all the car-specific fields defined in the form (make, model, year, etc.)
4. **Reference** - Optional unique reference for the car (if not provided, the system will generate one)

For detailed request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

After creating the car actor, you'll receive a response that includes the actor ID, which you'll use for subsequent operations like creating transactions or updating the car's data.

You can verify the car actor was created correctly using the actor retrieval API. This will return all the car's data, including any system-generated fields.

**Important Notes:**

- The data structure must conform to the validation rules defined in the car form
- Required fields (as defined in the form) must be included in the request
- The system will automatically create the account structures defined in the form for the new car actor

### 3. Initialize Car Accounts

When the car actor is created, the system automatically creates the accounts defined in the form. You can then initialize these accounts with initial balances using the transaction creation API.

For detailed information about creating transactions, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

To initialize a car account with an initial balance, you'll need to:

1. First retrieve the account ID using the account retrieval API
2. Create a transaction for the specific account (e.g., the "value" account for the car's purchase value)
3. Specify the amount, currency, and description for the transaction

The transaction will update the account balance, setting the initial value for the car account.

**Important Notes:**
- Each account type (asset, expense, etc.) may have different behavior for debit and credit transactions
- For asset accounts like "Car Value", a credit transaction typically increases the balance
- For expense accounts like "Maintenance Costs", a debit transaction typically increases the balance

For detailed information about transactions, see [Transactions](../entities/transactions.md).

### 4. Record Maintenance Expense

To record a maintenance expense for the car, use the transaction creation API. The process is similar to initializing accounts:

1. Retrieve the maintenance account ID using the account retrieval API
2. Create a transaction for the maintenance account
3. Specify the amount, currency, and description for the maintenance expense

The transaction will update the maintenance account balance, recording the expense. For detailed request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

**Additional Transaction Operations:**

- Retrieve all transactions for a specific account
- Retrieve a specific transaction by its ID
- Delete a transaction (if permitted by business rules)

For all these operations, refer to the [Simulator.Company API Documentation](https://doc.simulator.company) for detailed request and response formats.

### 5. Record Depreciation

To record depreciation of the car's value, use the transaction creation API with a request like:

```json
{
  "actorId": "car-actor-id",
  "accountName": "depreciation",
  "amount": 3000,
  "currency": "USD",
  "description": "Annual depreciation"
}
```

For detailed request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

### 6. Update Car Current Value

After recording depreciation, update the car's current value using the actor update API with a request like:

```json
{
  "formId": "car-form-id",
  "actorId": "car-actor-id",
  "data": {
    "currentValue": 22000
  }
}
```

For detailed request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

### 7. Track Mileage and Service Intervals

To track non-financial aspects of the car such as mileage and service intervals, you can use counter-type accounts.

For detailed information about creating and updating accounts, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

Create a mileage counter account:
```json
{
  "nameId": "mileage-name-id",
  "currencyId": "miles-currency-id",
  "type": "counter",
  "incomeType": "debit"
}
```

Update the mileage counter:
```json
{
  "accountId": "mileage-account-id",
  "amount": 500,
  "description": "Weekly mileage update"
}
```

This allows you to track non-financial metrics for the car, such as:
- Total mileage
- Service intervals
- Performance metrics
- Usage statistics

### 8. Generate Comprehensive Report

To generate a comprehensive report for the car, retrieve all accounts and transactions using the account and transaction retrieval APIs.

For detailed information about retrieving accounts and transactions, including request parameters, response formats, and authentication requirements, refer to the [Simulator.Company API Documentation](https://doc.simulator.company).

This retrieves all accounts and transactions associated with the car actor, allowing you to calculate:
- Total maintenance costs
- Total fuel costs
- Total insurance costs
- Total depreciation
- Current value
- Total cost of ownership
- Current mileage
- Service status
- Performance metrics

## API Reference

For detailed API documentation, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Simulator.Company API Documentation](https://doc.simulator.company)

## Related Documentation

- [Actors](../entities/actors.md) - Core entity representing nodes in business process graph
- [Forms](../entities/forms.md) - Reusable data structure templates for actors
- [Accounts](../entities/accounts.md) - Financial tracking for actors
- [Transactions](../entities/transactions.md) - Financial operations within accounts
- [Currencies](../entities/currencies.md) - Units of value for financial operations
- [System Forms](../entities/system-forms.md) - Predefined form templates for system functionality

## Authentication and Authorization

All API requests require OAuth2 authentication. The specific scopes required for each endpoint are documented in the official API documentation.

Common scopes used in these user flows include:

- `control.events:forms.readonly` - Read-only access to forms
- `control.events:forms.management` - Create, update, and delete forms
- `control.events:actors.readonly` - Read-only access to actors
- `control.events:actors.management` - Create, update, and delete actors
- `control.events:accounts.readonly` - Read-only access to accounts
- `control.events:accounts.management` - Create, update, and delete accounts

## Conclusion

The custom car form user flow demonstrates how to create a specialized form for tracking vehicles, create car actors using the form, and manage the financial aspects of vehicles through associated accounts. This approach can be adapted for any type of entity that requires structured data and financial tracking.
