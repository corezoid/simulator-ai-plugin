package tools

// counterOps — counter accounts. Counters ARE accounts, but a special, fast kind:
// their balances live in ScyllaDB and they keep NO per-transaction history — so
// they are cheap, high-throughput analytical tallies (counts/sums) for analytics
// and anti-fraud, not a ledger. Use the regular account/transaction tools when you
// need an auditable history; use counters for fast running totals.
//
// Counters are addressed by (formId, actorRef, accountName, currency, incomeType)
// rather than by an account id. Two flavours: `counter` (a running sum — record
// deltas with `amount`) and `uniqCounter` (deduplicates by `trsRef`, so the same
// event counted twice lands once). See the simulator-finance skill for the
// mileage/counter workflow.
var counterOps = []Operation{
	{
		Name: "saveCounters", Method: "POST", Path: "/counters/{accId}",
		Summary: "Record counter values in bulk (fast Scylla-backed tallies for analytics/anti-fraud; no transaction history is kept). " +
			"Addressed by actor ref + account name, not account id.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-30) of " +
				"{formId:int, actorRef:string, accountName:string, currency:string, " +
				"incomeType:\"debit\"|\"credit\", type:\"counter\"|\"uniqCounter\", " +
				"amount:number (required for counter — the delta to add), trsRef:string (required for uniqCounter — dedup key), title?, data?}."},
			{Name: "openAccounts", In: InQuery, Type: "boolean", Desc: "Create the counter accounts if they don't exist yet."},
			{Name: "lastValueOnly", In: InQuery, Type: "boolean", Desc: "Return only the latest value per counter."},
		},
	},
	{
		Name: "setCounters", Method: "POST", Path: "/counters/set/{accId}",
		Summary: "Set counter balances to fixed values in bulk (override, not increment).",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-30) of " +
				"{formId:int, actorRef:string, accountName:string, currency:string, " +
				"incomeType:\"debit\"|\"credit\", amount:number} — all required."},
			{Name: "openAccounts", In: InQuery, Type: "boolean", Desc: "Create the counter accounts if they don't exist yet."},
		},
	},
	{
		Name: "getCounters", Method: "POST", Path: "/counters/list/{accId}",
		Summary: "Read counter balances/values in bulk (Scylla-backed — fast, no transaction history). Body selects which counters by actor ref + account name.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array (1-30) of " +
				"{formId:int, actorRef:string, accountName:string, currency:string, " +
				"incomeType:\"debit\"|\"credit\", type:\"counter\"|\"uniqCounter\"} — all required."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Period start (unixtime)."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Period end (unixtime)."},
			{Name: "highPrecision", In: InQuery, Type: "boolean", Desc: "Return sums with high precision."},
		},
	},
}
