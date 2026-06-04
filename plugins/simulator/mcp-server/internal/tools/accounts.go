package tools

// accountTypes are the FORM_VALUE_ACCOUNT_TYPES surfaced for guidance.
var accountTypes = []string{"asset", "liability", "expense", "income", "counter", "state"}

// accountOps — accounts on actors, plus the workspace-level currency and
// account-name reference data they depend on.
var accountOps = []Operation{
	{
		Name: "createAccount", Method: "POST", Path: "/accounts/{actorId}",
		Summary: "Create a financial/metric account on an actor. Identified by (nameId, currencyId); accountType selects the ledger semantics.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID the account belongs to."},
			{Name: "nameId", In: InBody, Type: "string", Required: true, Desc: "Account name id (see getAccountNames / createAccountName)."},
			{Name: "currencyId", In: InBody, Type: "number", Required: true, Desc: "Currency id (see getCurrencies / createCurrency)."},
			{Name: "accountType", In: InBody, Type: "string", Enum: accountTypes, Desc: "Ledger type."},
			{Name: "treeCalculation", In: InBody, Type: "boolean", Desc: "Aggregate child actor balances into this account."},
			{Name: "minLimit", In: InBody, Type: "number", Desc: "Optional minimum balance limit."},
			{Name: "maxLimit", In: InBody, Type: "number", Desc: "Optional maximum balance limit."},
			{Name: "search", In: InBody, Type: "boolean", Desc: "Whether the account is searchable."},
			{Name: "ignoreIfExist", In: InQuery, Type: "boolean", Desc: "Do not error if the account already exists."},
		},
	},
	{
		Name: "getAccounts", Method: "GET", Path: "/accounts/{actorId}",
		Summary: "List the accounts on an actor with their balances. Pass `from`/`to` to get each account's turnover/balance over that period (the account's movements summed within the window) — this is the account turnover for the period.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "accountType", In: InQuery, Type: "string", Enum: accountTypes, Desc: "Filter by account type."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Period start, unixtime in MILLISECONDS. Balances become the turnover from this moment."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Period end, unixtime in MILLISECONDS."},
			{Name: "incomeType", In: InQuery, Type: "string", Enum: []string{"debit", "credit"}, Desc: "Restrict the period turnover to one direction (debit = outgoing, credit = incoming)."},
			{Name: "amountFrom", In: InQuery, Type: "number", Desc: "Only accounts whose (period) amount is >= this value."},
			{Name: "amountTo", In: InQuery, Type: "number", Desc: "Only accounts whose (period) amount is <= this value."},
			{Name: "withAggTypes", In: InQuery, Type: "boolean", Desc: "Include aggregated turnover by type in the response."},
			{Name: "query", In: InQuery, Type: "string", Desc: "Search accounts by name."},
			{Name: "total", In: InQuery, Type: "boolean", Desc: "Return the total count instead of the list."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 100)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "filter", In: InQuery, Type: "string", Desc: "Filter expression on accounts."},
			{Name: "highPrecision", In: InQuery, Type: "boolean", Desc: "Return transaction sums with high precision."},
		},
	},
	{
		Name: "getBalance", Method: "GET", Path: "/accounts/{actorId}/{currencyId}/{nameId}",
		Summary: "Get the current balance of one account on an actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
		},
	},
	{
		Name: "updateAccount", Method: "PUT", Path: "/accounts/{actorId}/{currencyId}/{nameId}/{accountType}",
		Summary: "Update an account's settings (limits, tree calculation, searchability).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InPath, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InPath, Type: "string", Required: true, Desc: "Account name id."},
			{Name: "accountType", In: InPath, Type: "string", Required: true, Enum: accountTypes, Desc: "Account type."},
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
			{Name: "accountType", In: InPath, Type: "string", Required: true, Enum: accountTypes, Desc: "Account type."},
		},
	},
	{
		Name: "createCurrency", Method: "POST", Path: "/currencies/{accId}",
		Summary: "Create a currency / unit of value in the workspace (e.g. USD, Km, Units).",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "name", In: InBody, Type: "string", Required: true, Desc: "Currency name."},
			{Name: "symbol", In: InBody, Type: "string", Desc: "Display symbol."},
			{Name: "precision", In: InBody, Type: "number", Desc: "Decimal precision."},
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
		},
	},
	{
		Name: "createAccountName", Method: "POST", Path: "/account_names/{accId}",
		Summary: "Create an account-name category (e.g. \"Cash\", \"Revenue\") in the workspace.",
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
		},
	},
}
