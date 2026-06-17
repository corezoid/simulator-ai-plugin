package tools

// smartFormOps — the **Smart Form runtime** primitives: drive any Smart Form
// (CDU / Script application) the way the web renderer does, by speaking the
// platform's page protocol. A Smart Form is a server-driven UI state machine —
// `appGetPage` renders a page (its forms + components), `appSendForm` submits a
// form and returns the next state. The env's Corezoid process supplies the
// runtime data, validation and control flow; these tools carry NO app-specific
// logic, so the same two tools drive every mini-app (contracts, invoices, …).
//
// A Smart Form is addressed at runtime by (accId, ref, envTitle); envTitle is
// `develop` or `production`. The full page/forms/component model and the
// get/send response codes are specified in
// docs/user-flows/cdu-page-protocol.md (§2–§5, §7). The page-serving routes are
// public (`/papi/1.0/pages/...`, scope actors.readonly + the app's sharedWith
// rule), so no backend change is needed to expose them as tools.
//
// These are the runtime counterpart to the authoring surface used by the
// simulator-smart-forms skill (createSmartForm / app_content / releases).
var smartFormOps = []Operation{
	{
		Name: "appGetPage", Method: "GET", Path: "/pages/{accId}/{ref}/{envTitle}/{page}",
		Summary: "Render one page of a Smart Form (CDU / Script application) — the runtime equivalent of opening " +
			"the app's page in the UI. Returns a Page `{ grid, forms[], notifications[], query, language }` " +
			"(code 200); the env's Corezoid process supplies the dynamic viewModel/data. Read `forms[].sections[].content[]` " +
			"to see the components (items) to fill, and `notifications[]` for messages from the process. Then submit with " +
			"appSendForm. Start a flow at page `index`. Smart Forms are addressed by (accId, ref, envTitle); use the " +
			"`production` env unless testing `develop`. See docs/user-flows/cdu-page-protocol.md.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id the Smart Form belongs to. Defaults to the configured workspace if omitted."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "The Smart Form's ref (its identity in the `scripts` system form). Resolve from the App Catalog or by searching the scripts form."},
			{Name: "envTitle", In: InPath, Type: "string", Required: true, Desc: "Environment to serve: `production` (live) or `develop` (editable).", Enum: []string{"production", "develop"}},
			{Name: "page", In: InPath, Type: "string", Required: true, Desc: "Page id to render. Use `index` for the app's landing page; follow `nextPage` / `pageId` from prior responses for subsequent pages."},
		},
	},
	{
		Name: "appSendForm", Method: "POST", Path: "/pages/{accId}/{ref}/{envTitle}/{page}",
		Summary: "Submit a form on a Smart Form page — the runtime equivalent of clicking a button in the app. " +
			"Pass the `formId` and `buttonId` from the page (appGetPage) and `data` = the collected item values keyed by item id. " +
			"The env's Corezoid process validates, runs side effects (e.g. creating actors / transactions), and returns one of " +
			"three outcomes: 200 = patch the current page `{ changes[], notifications[], query }` (apply the change protocol §7); " +
			"205 RESET_CONTENT = re-render a (possibly different) page `{ page, pageId? }`; 302 REDIRECT = navigate `{ nextPage, target? }`. " +
			"Read `notifications[]` for success/error messages and self-correct on validation errors. CAUTION: a submit can have " +
			"irreversible side effects (created actors, money movements) — confirm with the user before sending those. " +
			"See docs/user-flows/cdu-page-protocol.md §2.3.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id the Smart Form belongs to. Defaults to the configured workspace if omitted."},
			{Name: "ref", In: InPath, Type: "string", Required: true, Desc: "The Smart Form's ref (identity in the `scripts` system form)."},
			{Name: "envTitle", In: InPath, Type: "string", Required: true, Desc: "Environment: `production` or `develop`.", Enum: []string{"production", "develop"}},
			// `page` goes into BOTH the path segment and the body (the /send
			// handler reads it from the body, but it's also the route segment).
			// InPathBody expresses that as one argument / one schema property.
			{Name: "page", In: InPathBody, Type: "string", Required: true, Desc: "Page id the form belongs to (the page you submitted from)."},
			{Name: "formId", In: InBody, Type: "string", Required: true, Desc: "Id of the form being submitted (from `forms[].id` on the page)."},
			{Name: "sectionId", In: InBody, Type: "string", Required: true, Desc: "Id of the section the submitted form belongs to (from `forms[].sections[].id`). Required by the backend."},
			{Name: "data", In: InBody, Type: "object", Required: true, Desc: "Collected values of the form's value-bearing items, keyed by item id, e.g. {\"counterparty\":\"Acme\",\"value\":50000}. Use {} if the button submits no field values."},
			{Name: "buttonId", In: InBody, Type: "string", Desc: "Optional id of the button that triggered the submit (from a `button` item). Omit for auto-submit / submit-on-change."},
			{Name: "buttonData", In: InBody, Type: "object", Desc: "Optional extra payload carried by the button (e.g. a menu choice or auto-submit counter)."},
		},
	},
}
