package tools

// transactionOps — record value on accounts (immediate or 2-step) and move value
// atomically between accounts (transfers). `ref` makes a write idempotent.
var transactionOps = []Operation{
	{
		Name: "createTransaction", Method: "POST", Path: "/transactions/{accountId}",
		Summary: "Record a transaction on an account. Pass a stable `ref` to make it idempotent (safe to retry).",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Target account id."},
			{Name: "amount", In: InBody, Type: "number", Desc: "Signed amount in the account's currency, as the real value (e.g. 500 means 500 USD). Stored as a decimal — do NOT scale by the currency precision/10^decimals; precision only controls display rounding."},
			{Name: "comment", In: InBody, Type: "string", Desc: "Human-readable note."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Idempotency reference (max 255)."},
			{Name: "commission", In: InBody, Type: "number", Desc: "Optional commission amount."},
			{Name: "data", In: InBody, Type: "object", Desc: "Optional structured payload."},
			{Name: "parentRef", In: InBody, Type: "string", Desc: "Reference of a parent transaction."},
			{Name: "expiration", In: InBody, Type: "number", Desc: "Expiration timestamp for a 2-step hold."},
			{Name: "noRetry", In: InQuery, Type: "boolean", Desc: "Disable server-side retry."},
		},
	},
	{
		Name: "finalizeTransaction", Method: "POST", Path: "/transactions/{accountId}/{type}",
		Summary: "Finalize a 2-step transaction (complete or cancel a previously authorized hold).",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Target account id."},
			{Name: "type", In: InPath, Type: "string", Required: true, Desc: "Finalization type (e.g. completed / canceled)."},
			{Name: "amount", In: InBody, Type: "number", Desc: "Amount as the real value in the account's currency (required unless completing the full hold). Not scaled by precision — see createTransaction."},
			{Name: "comment", In: InBody, Type: "string", Desc: "Note."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Idempotency reference."},
			{Name: "parentRef", In: InBody, Type: "string", Desc: "Parent transaction reference."},
		},
	},
	{
		Name: "getTransactions", Method: "GET", Path: "/transactions/{actorId}",
		Summary: "List transactions for one account on an actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor UUID."},
			{Name: "currencyId", In: InQuery, Type: "number", Required: true, Desc: "Currency id."},
			{Name: "nameId", In: InQuery, Type: "string", Required: true, Desc: "Account name id."},
			{Name: "accountType", In: InQuery, Type: "string", Enum: accountTypes, Desc: "Account type."},
			{Name: "incomeType", In: InQuery, Type: "string", Enum: []string{"debit", "credit"}, Desc: "Filter by direction."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			fieldFilterParam("id,amount,createdAt,comment,ref,incomeType"),
		},
	},
	{
		Name: "createTransfer", Method: "POST", Path: "/transfers/{accId}",
		Summary: "Atomically move value between accounts. `from`/`to` are arrays of {accountId, amount, ...} legs. Pass `ref` for idempotency.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "from", In: InBody, Type: "array", Desc: "Debit legs: array of {accountId, amount, ...} (0-20)."},
			{Name: "to", In: InBody, Type: "array", Desc: "Credit legs: array of {accountId, amount, ...} (0-20)."},
			{Name: "comment", In: InBody, Type: "string", Desc: "Note."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Idempotency reference (max 255)."},
			{Name: "transferId", In: InBody, Type: "string", Desc: "Optional explicit transfer id."},
			{Name: "noRetry", In: InQuery, Type: "boolean", Desc: "Disable server-side retry."},
		},
	},
	{
		Name: "getTransfer", Method: "GET", Path: "/transfers/{transferId}",
		Summary: "Get a transfer by id. Pass `filter` to fetch only the fields you need.",
		Params: []Param{
			{Name: "transferId", In: InPath, Type: "string", Required: true, Desc: "Transfer id."},
			fieldFilterParam("id,amount,comment,ref,status,createdAt"),
		},
	},
	{
		Name: "atomCreateTransaction", Method: "POST", Path: "/transactions/atom/{accId}",
		Summary: "Create several transactions atomically (all succeed or all fail) across accounts in the workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-20) of " +
				"{accountId:string (required), amount:number (required unless type=completed), type?, comment?, ref?, parentRef?, parentId?, data?, expiration?}. " +
				"Pass per-leg `ref` for idempotency."},
			{Name: "noRetry", In: InQuery, Type: "boolean", Desc: "Disable server-side retry."},
		},
	},
	{
		Name: "getAccountTransactions", Method: "GET", Path: "/transactions/list/{accountId}",
		Summary: "List transactions of one account, addressed directly by its account id. " +
			"Unlike the other list tools, `limit` and `offset` are both REQUIRED here.",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Account id."},
			{Name: "limit", In: InQuery, Type: "number", Required: true, Desc: "Page size (required by this endpoint)."},
			{Name: "offset", In: InQuery, Type: "number", Required: true, Desc: "Page offset (required by this endpoint; pass 0 for the first page)."},
		},
	},
	{
		Name: "getTransactionByRef", Method: "GET", Path: "/transactions/ref/{accountId}/{ref}",
		Summary: "Look up a transaction on an account by its idempotency `ref` (e.g. to confirm a prior write landed).",
		Params: []Param{
			{Name: "accountId", In: InPath, Type: "string", Required: true, Desc: "Account id."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "The transaction's idempotency reference."},
		},
	},
	{
		Name: "createTransferTwoStep", Method: "POST", Path: "/transfers/{accId}/{type}",
		Summary: "Two-step transfer: type=authorized places a hold, then type=completed / canceled finalizes it (declined/reversed also available). Mirrors finalizeTransaction for transfers.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "type", In: InPath, Type: "string", Required: true, Desc: "Step type: authorized | completed | canceled | declined | reversed."},
			{Name: "from", In: InBody, Type: "array", Desc: "Debit legs: array of {accountId, amount, ...} (0-20)."},
			{Name: "to", In: InBody, Type: "array", Desc: "Credit legs: array of {accountId, amount, ...} (0-20)."},
			{Name: "comment", In: InBody, Type: "string", Desc: "Note."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Idempotency reference (max 255)."},
			{Name: "transferId", In: InBody, Type: "string", Desc: "Transfer id to finalize (for the completing/canceling step)."},
			{Name: "data", In: InBody, Type: "object", Desc: "Optional structured payload."},
			{Name: "noRetry", In: InQuery, Type: "boolean", Desc: "Disable server-side retry."},
		},
	},
	{
		Name: "filterTransfers", Method: "POST", Path: "/transfers/filter/{accId}",
		Summary: "List/filter transfers in a workspace over a time window, optionally by account, actor, amount, direction or text.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "from", In: InBody, Type: "number", Required: true, Desc: "Window start, unixtime."},
			{Name: "to", In: InBody, Type: "number", Required: true, Desc: "Window end, unixtime."},
			{Name: "accounts", In: InBody, Type: "array", Desc: "Filter by accounts: array of {nameId, currencyId}."},
			{Name: "actors", In: InBody, Type: "array", Desc: "Filter by actor UUIDs."},
			{Name: "incomeType", In: InBody, Type: "string", Enum: []string{"credit", "debit"}, Desc: "Direction filter."},
			{Name: "amount", In: InBody, Type: "number", Desc: "Amount to compare (with `oper`)."},
			{Name: "oper", In: InBody, Type: "string", Enum: []string{"eq", "gt", "gte", "lt", "lte", "ne"}, Desc: "Comparison operator for `amount`."},
			{Name: "query", In: InBody, Type: "string", Desc: "Full-text search."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Filter by transfer ref."},
			{Name: "limit", In: InBody, Type: "number", Desc: "Page size (max 50)."},
			{Name: "offset", In: InBody, Type: "number", Desc: "Page offset."},
			{Name: "total", In: InQuery, Type: "boolean", Desc: "Return the total count instead of the list."},
		},
	},
	{
		Name: "getTransferByRef", Method: "GET", Path: "/transfers/{accId}/{ref}",
		Summary: "Get a transfer by its idempotency ref within a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "The transfer's idempotency reference."},
		},
	},
}
