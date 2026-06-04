package tools

// searchOps — workspace-wide global search (the public mirror of the backend's
// /api search). Supports text and semantic (vector) search across actors and
// users, optionally scoped to a form.
var searchOps = []Operation{
	{
		Name: "searchAll", Method: "GET", Path: "/search/{accId}/{query}",
		Summary: "Global workspace search across actors (and users) — text or semantic. Use to find existing entities before creating. `filters` selects what to search.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search text (min 2 chars)."},
			{Name: "filters", In: InQuery, Type: "string", Required: true, Desc: "Comma-separated targets: actors, fullTextActor, users (e.g. \"actors\" or \"actors,users\")."},
			{Name: "searchType", In: InQuery, Type: "string", Enum: []string{"text", "semantic"}, Desc: "Search mode: text (default) or semantic (vector)."},
			{Name: "formId", In: InQuery, Type: "string", Desc: "Restrict the search to actors of this form."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-100)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
}
