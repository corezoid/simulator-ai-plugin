package tools

import (
	"context"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// accountTypes are the FORM_VALUE_ACCOUNT_TYPES — the account's VALUE type:
// fact (the actual recorded value) | plan (a planned/budget value) | min | max | avg
// (aggregates over children). Default fact. This is NOT an accounting category
// (there is no asset/liability/expense/income field on an account).
var accountTypes = []string{"fact", "plan", "min", "max", "avg"}

// counterTypes are the ACCOUNT_COUNTER_TYPE values — whether an account is a plain
// amount account or a counter. `counter`/`uniqCounter` make it a Scylla-backed,
// history-less tally (see the counters API). Default amount.
var counterTypes = []string{"amount", "counter", "systemCounter", "uniqCounter"}

// accountOps — accounts on actors, plus the workspace-level currency and
// account-name reference data they depend on.
var accountOps = []Operation{
	{
		Name: "createAccount", Method: "POST", Path: "/accounts/{actorId}",
		Summary: "Attach an account to an actor. Identified by (nameId, currencyId, accountType). Use counterType=counter/uniqCounter for a fast Scylla-backed metric (mileage, counts); leave it amount for a normal balance account.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID the account belongs to."},
			{Name: "nameId", In: InBody, Type: "string", Required: true, Desc: "Account name id (see getAccountNames / createAccountName)."},
			{Name: "currencyId", In: InBody, Type: "number", Required: true, Desc: "Currency id (see getCurrencies / createCurrency)."},
			{Name: "accountType", In: InBody, Type: "string", Enum: accountTypes, Desc: "Value type: fact (actual, default) | plan (planned/budget) | min | max | avg (aggregates)."},
			{Name: "counterType", In: InBody, Type: "string", Enum: counterTypes, Desc: "amount (normal balance, default) | counter | uniqCounter (Scylla tally, no history) | systemCounter."},
			{Name: "treeCalculation", In: InBody, Type: "boolean", Desc: "Aggregate child actor balances into this account."},
			{Name: "minLimit", In: InBody, Type: "number", Desc: "Optional minimum balance limit."},
			{Name: "maxLimit", In: InBody, Type: "number", Desc: "Optional maximum balance limit."},
			{Name: "search", In: InBody, Type: "boolean", Desc: "Whether the account is searchable."},
			{Name: "ignoreIfExist", In: InQuery, Type: "boolean", Desc: "Do not error if the account already exists."},
		},
	},
	{
		Name: "getAccounts", Method: "GET", Path: "/accounts/{actorId}",
		Summary: "List the accounts on an actor with their balances. Pass `from`/`to` to get each account's turnover/balance over that period (the account's movements summed within the window) — this is the account turnover for the period. The returned `amount` is the real balance value as a decimal (e.g. 1600 = 1600 USD); the currency `precision` only controls display rounding — do NOT divide by 10^precision. " +
			"PAGINATION: the backend's default page is ~30 rows and the response carries NO has-more marker, so accounts created last (usually the ones you are looking for) silently fall off the first page — this tool therefore requests limit=100 (the backend max) when you don't pass `limit` yourself. If an actor may hold more than 100 accounts, page with `offset` until a short page. Note `total:true` counts only fact-type accounts. " +
			"An account you KNOW exists (createAccount succeeded / \"already exist\") but that never appears in any page means you lack access to its (nameId, currencyId) PAIR — see createAccountPair.",
		Resolve: defaultAccountsLimit,
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "accountType", In: InQuery, Type: "string", Enum: accountTypes, Desc: "Filter by account type."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Period start, unixtime in MILLISECONDS. Balances become the turnover from this moment."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Period end, unixtime in MILLISECONDS."},
			{Name: "incomeType", In: InQuery, Type: "string", Enum: []string{"debit", "credit"}, Desc: "Restrict the period turnover to one direction (debit = outgoing, credit = incoming)."},
			{Name: "amountFrom", In: InQuery, Type: "number", Desc: "Only accounts whose (period) amount is >= this value."},
			{Name: "amountTo", In: InQuery, Type: "number", Desc: "Only accounts whose (period) amount is <= this value."},
			{Name: "tag", In: InQuery, Type: "string", Desc: "Only accounts whose pair is linked to this tag actor — pass the Tags-form actor's UUID (see saveAccountActors)."},
			{Name: "ungrouped", In: InQuery, Type: "boolean", Desc: "Only accounts that have NO tags."},
			{Name: "withTags", In: InQuery, Type: "boolean", Desc: "Include each account's tags (the Tags-form actors linked to its pair) in the response."},
			{Name: "withTriggers", In: InQuery, Type: "boolean", Desc: "Include each account's triggers (the AccountTriggers-form actors linked to its pair or to the account) in the response."},
			{Name: "withAggTypes", In: InQuery, Type: "boolean", Desc: "Include aggregated turnover by type in the response."},
			{Name: "query", In: InQuery, Type: "string", Desc: "Search accounts by name."},
			{Name: "total", In: InQuery, Type: "boolean", Desc: "Return the total count instead of the list."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 100; defaults to 100 — the backend's own default is ~30 with no has-more marker)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,accountName,currencyName,amount,availableAmount,incomeType,counterType"),
			{Name: "highPrecision", In: InQuery, Type: "boolean", Desc: "Return transaction sums with high precision."},
		},
	},
	{
		Name: "getBalance", Method: "GET", Path: "/accounts/{actorId}/{currencyId}/{nameId}",
		Summary: "Get the current balance of one account on an actor. The returned `amount` is the real value as a decimal; the currency `precision` only controls display rounding, it is not a scaling factor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
			fieldFilterParam("amount,availableAmount,holdAmount,currencyName,accountName"),
		},
	},
	{
		Name: "updateAccount", Method: "PUT", Path: "/accounts/{actorId}/{currencyId}/{nameId}/{accountType}",
		Summary: "Update an account's settings (limits, tree calculation, searchability).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
			{Name: "accountType", In: InPath, Type: "string", Required: true, Enum: accountTypes, Desc: "Value type identifying the account (usually \"fact\")."},
			{Name: "treeCalculation", In: InBody, Type: "boolean", Desc: "Aggregate child balances."},
			{Name: "minLimit", In: InBody, Type: "number", Desc: "Minimum balance limit."},
			{Name: "maxLimit", In: InBody, Type: "number", Desc: "Maximum balance limit."},
			{Name: "search", In: InBody, Type: "boolean", Desc: "Searchable flag."},
		},
	},
	{
		Name: "deleteAccount", Method: "DELETE", Path: "/accounts/{actorId}/{currencyId}/{nameId}/{accountType}",
		Summary: "Delete an account from an actor. Irreversible — confirm first.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
			{Name: "accountType", In: InPath, Type: "string", Required: true, Enum: accountTypes, Desc: "Value type identifying the account (usually \"fact\")."},
		},
	},
	{
		Name: "createCurrency", Method: "POST", Path: "/currencies/{accId}",
		Summary: "Create a currency / unit of value in the workspace (e.g. USD, Km, Units). " +
			"PREFER `createAccountPair` when this currency is being created so it can be used on an account: `createAccountPair` creates the currency (and the account-name) if missing AND grants the caller pair-level access in one call — bare `createCurrency` leaves the resulting (name, currency) pair without an access rule, so the next balance/transaction call 403s on any non-owner workspace. Only use bare `createCurrency` for the rare case of a workspace currency that no account will use.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "name", In: InBody, Type: "string", Required: true, Desc: "Currency name."},
			{Name: "symbol", In: InBody, Type: "string", Desc: "Display symbol."},
			{Name: "precision", In: InBody, Type: "number", Desc: "Number of decimal places shown in the UI (display only). Amounts are stored as decimals at their real value — precision is not a scaling factor, e.g. precision 2 displays 1600 as 1600.00, it does not mean 16.00."},
			{Name: "type", In: InBody, Type: "string", Desc: "Currency type."},
		},
	},
	{
		Name: "getCurrencies", Method: "GET", Path: "/currencies/{accId}",
		Summary: "List currencies in the workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,name,symbol,precision"),
		},
	},
	{
		Name: "createAccountName", Method: "POST", Path: "/account_names/{accId}",
		Summary: "Create an account-name category (e.g. \"Cash\", \"Revenue\") in the workspace. " +
			"PREFER `createAccountPair` when this account-name is being created so it can be used on an account: `createAccountPair` creates the account-name (and the currency) if missing AND grants the caller pair-level access in one call — bare `createAccountName` leaves the resulting (name, currency) pair without an access rule, so the next balance/transaction call 403s on any non-owner workspace. Only use bare `createAccountName` for the rare case of a workspace category that no account will use.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "name", In: InBody, Type: "string", Required: true, Desc: "Account name."},
			{Name: "abbreviation", In: InBody, Type: "string", Desc: "Short label."},
		},
	},
	{
		Name: "getAccountNames", Method: "GET", Path: "/account_names/{accId}",
		Summary: "List account-name categories in the workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,name,abbreviation"),
		},
	},
	{
		Name: "updateAccountName", Method: "PUT", Path: "/account_names/{nameId}",
		Summary: "Rename an account-name category (and/or its abbreviation), and/or flag it transfer-only.",
		Params: []Param{
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
			{Name: "name", In: InBody, Type: "string", Required: true, Desc: "New account name."},
			{Name: "abbreviation", In: InBody, Type: "string", Desc: "New short label."},
			{Name: "transferOnly", In: InBody, Type: "boolean", Desc: "When true, accounts using this account-name reject plain transactions (single, atomic, and standalone 2-step) and can only be moved via a transfer — useful for clearing/settlement categories that must always balance against a counterparty. Defaults false. Omit to leave the current value unchanged. Returned on every account-name read."},
		},
	},
	{
		Name: "searchAccountNames", Method: "GET", Path: "/account_names/search/{accId}/{query}",
		Summary: "Search account-name categories by name in a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search text (name or fragment)."},
			{Name: "withStats", In: InQuery, Type: "boolean", Desc: "Include usage stats per name."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 50)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
	{
		Name: "searchCurrencies", Method: "GET", Path: "/currencies/search/{accId}/{query}",
		Summary: "Search currencies by name in a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search text (name or fragment)."},
			{Name: "withStats", In: InQuery, Type: "boolean", Desc: "Include usage stats per currency."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 50)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
	{
		Name: "getAccount", Method: "GET", Path: "/accounts/single/{accountId}",
		Summary: "Get one account by its id (balance, settings, optional privileges/turnover). " +
			"The `amount` is the real balance value as a decimal — do NOT scale by the currency precision.",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Account id."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Period start, unixtime in MILLISECONDS (turns the balance into the period turnover)."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Period end, unixtime in MILLISECONDS."},
			{Name: "trsCount", In: InQuery, Type: "boolean", Desc: "Include the transaction count."},
			{Name: "withPrivs", In: InQuery, Type: "boolean", Desc: "Include the caller's privileges on the account."},
			{Name: "highPrecision", In: InQuery, Type: "boolean", Desc: "Return sums with high precision."},
			fieldFilterParam("id,accountName,currencyName,amount,availableAmount,incomeType,counterType"),
		},
	},
	{
		Name: "setAccountAmount", Method: "PUT", Path: "/accounts/amount/{accountId}",
		Summary: "Set an account's balance to a fixed value (a correction/override, not a transaction). " +
			"Pass the real value as a decimal — not scaled by the currency precision. Irreversible adjustment — confirm first.",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Account id."},
			{Name: "amount", In: InBody, Type: "number", Required: true, Desc: "The new fixed balance, as the real value in the account's currency."},
		},
	},
	{
		Name: "getChildAccounts", Method: "GET", Path: "/accounts/children/{actorId}",
		Summary: "List the matching accounts on the child actors of an actor (for one account name + currency).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent actor UUID."},
			{Name: "nameId", In: InQuery, Type: "string", Required: true, Desc: "Account name id to look up on each child."},
			{Name: "currencyId", In: InQuery, Type: "string", Required: true, Desc: "Currency id to look up on each child."},
			{Name: "accountType", In: InQuery, Type: "string", Enum: accountTypes, Desc: "Optional account type filter."},
		},
	},
	{
		Name: "createAccountPair", Method: "POST", Path: "/accounts/pair/{accId}",
		Summary: "Create the workspace-level (account-name + currency) PAIR and GRANT THE CALLER access to it. " +
			"Account access is enforced on the pair `<nameId>_<currencyId>` (objType=account), NOT per actor: `createAccount` attaches an account to an actor but never seeds pair access, so a non-owner then gets 403 on getBalance / getAccount / setAccountAmount / createTransaction / transfers. " +
			"Call this once per (name, currency) to bootstrap access BEFORE recording values — it creates the account name and the currency if they are missing, then grants you view+modify+remove on the pair. " +
			"Identified BY NAME (accountName / currencyName), not ids. Safe to repeat. Note: if the pair already has access rules and you are not among them it returns 403 — then a workspace Owner (or an existing grantee) must grant you via saveAccessRules. Workspace Owners don't need this at all.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "accountName", In: InBody, Type: "string", Required: true, Desc: "Account-name category BY NAME (e.g. \"Deal Value\"). Created if it does not exist."},
			{Name: "currencyName", In: InBody, Type: "string", Required: true, Desc: "Currency BY NAME (e.g. \"USD\"). Created if it does not exist (using symbol/precision/type below)."},
			{Name: "symbol", In: InBody, Type: "string", Desc: "Display symbol — only used when the currency is created."},
			{Name: "precision", In: InBody, Type: "number", Desc: "Decimal places shown (display only; default 2) — only used when the currency is created."},
			{Name: "type", In: InBody, Type: "string", Desc: "Currency type (default \"number\") — only used when the currency is created."},
		},
	},
	{
		Name: "setAccountFormula", Method: "POST", Path: "/accounts/formula/{accountId}",
		Summary: "Turn an account into a COMPUTED (formula) account: its balance is a math expression over OTHER " +
			"accounts. The `formula` references source accounts by their full account UUID — each UUID is substituted " +
			"with that account's AVAILABLE balance (amount − hold) — then evaluated (mathjs: + - * / ( ), etc.); the " +
			"numeric result becomes this account's balance and recalculates automatically when a source account changes. " +
			"RULE: you CANNOT set a formula on an account that already has transactions. " +
			"To CLEAR the formula (make it a plain account again), pass an empty string. " +
			"Get the source account UUIDs from getAccount / getAccounts (the account `id`, not the actor id).",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Id of the account to make computed (the calc account)."},
			{Name: "formula", In: InBody, Type: "string", Required: true, Desc: "Math expression referencing source account UUIDs, e.g. \"<accUuidA> + <accUuidB> * 0.2\". Each UUID resolves to that account's available balance (amount − hold). Empty string clears the formula."},
		},
	},
	{
		Name: "getAccountFormula", Method: "GET", Path: "/accounts/formula_info/{accountId}",
		Summary: "Inspect a computed account's formula: returns the source accounts it references (their balances + " +
			"owning actor), or an empty object if the account has no formula. Use to see what a formula account depends on.",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Id of the computed (formula) account to inspect."},
			fieldFilterParam("formula,accounts"),
		},
	},
}

// defaultAccountsLimit fills limit=100 (the backend max) when the caller does
// not page explicitly. The backend's own default page is ~30 rows and the
// response has no has-more marker, so accounts created last — usually exactly
// the ones the caller is trying to see — silently fell off the first page and
// read as "the account was never created". Callers that page explicitly are
// untouched.
func defaultAccountsLimit(_ context.Context, args map[string]any, _ *apiclient.Client) error {
	if _, ok := args["limit"]; !ok {
		args["limit"] = float64(100)
	}
	return nil
}
