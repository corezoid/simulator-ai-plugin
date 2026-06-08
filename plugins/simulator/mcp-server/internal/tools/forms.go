package tools

// sectionsDesc documents the form `sections` payload for create/updateForm so the
// model emits a valid template. Forms are also surfaced to end users as "Account
// Templates" (Шаблон рахунків). See docs/entities/forms.md for the full catalogue.
const sectionsDesc = "Ordered array of sections. Each section = {title, content[]}. " +
	"Each content item is one field: " +
	"{id:\"item_<digits>\" (the stable key actors use in their `data` — NOT title/`key`), " +
	"class, title, visibility:\"visible\"|\"disabled\"|\"hidden\", and class-specific keys}. " +
	"Classes: " +
	"edit (text input; optional type:text|password|email|phone|int|float, regexp, errorMsg), " +
	"check (checkbox), radio (single choice; options[] of {title,value}, optional align), " +
	"select (single dropdown; static options[] of {title,value,color?} OR dynamic via " +
	"extra.optionsSource.type = manual(=static)|layer{value.id}|actorFilter{value.id}|actorsBag|actors{value.ids[]}|" +
	"forms|formFilter|currencies|accountNames|workspaceMembers|api|corezoidSyncApi{value.convId,apiLogin,apiSecret}), " +
	"multiSelect (multi dropdown; static options[]), " +
	"calendar (date/time; extra.{time,minDate,maxDate,dateRange,timeZone,static}, unix seconds), " +
	"upload (file), label/button/image (display-only; image value is a URL — these produce NO actor data). " +
	"Generate unique item ids per field."

// formOps — form template CRUD. Forms define the field structure (and default
// accounts) that actors instantiate. accId is the workspace; formId is integer.
var formOps = []Operation{
	{
		Name: "createForm", Method: "POST", Path: "/forms/{accId}/{isTemplate}",
		Summary: "Create a form template (a.k.a. Account Template / Шаблон рахунків). Defines the field structure (sections) actors of this form will have. Set isTemplate=true for a reusable template.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "isTemplate", In: InPath, Type: "boolean", Required: true, Desc: "Whether the form is a reusable template."},
			{Name: "title", In: InBody, Type: "string", Required: true, Desc: "Form name."},
			{Name: "sections", In: InBody, Type: "array", Required: true, Desc: sectionsDesc},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
			{Name: "color", In: InBody, Type: "string", Desc: "Hex color for actors of this form (e.g. #409547)."},
			{Name: "picture", In: InBody, Type: "string", Desc: "Storage path / URL of the form icon."},
			{Name: "ref", In: InBody, Type: "string", Desc: "Optional external reference id."},
			{Name: "settings", In: InBody, Type: "object", Desc: "Optional form settings object."},
			{Name: "tags", In: InBody, Type: "array", Desc: "Optional list of tags."},
			{Name: "parentId", In: InBody, Type: "number", Desc: "Optional parent form id for inheritance."},
			{Name: "withRelations", In: InQuery, Type: "boolean", Desc: "Return related entities in the response."},
		},
	},
	{
		Name: "getForm", Method: "GET", Path: "/forms/{formId}",
		Summary: "Get a form template by its integer id. Pass `filter` to fetch only the fields you need (form templates can be large).",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
			{Name: "withRelations", In: InQuery, Type: "boolean", Desc: "Include related entities."},
			fieldFilterParam("id,title,sections"),
		},
	},
	{
		Name: "getForms", Method: "GET", Path: "/forms/templates/{accId}",
		Summary: "List form templates in a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (default 20)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
			{Name: "status", In: InQuery, Type: "string", Desc: "Filter by form status."},
			{Name: "formTypes", In: InQuery, Type: "string", Desc: "Filter by form types."},
			{Name: "withRelations", In: InQuery, Type: "boolean", Desc: "Include related entities."},
			fieldFilterParam("id,title,status"),
		},
	},
	{
		Name: "updateForm", Method: "PUT", Path: "/forms/{formId}",
		Summary: "Update a form template (replaces title/sections).",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
			{Name: "title", In: InBody, Type: "string", Required: true, Desc: "Form name."},
			{Name: "sections", In: InBody, Type: "array", Required: true, Desc: sectionsDesc},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
			{Name: "color", In: InBody, Type: "string", Desc: "Hex color."},
		},
	},
	{
		Name: "deleteForm", Method: "DELETE", Path: "/forms/{formId}",
		Summary: "Delete a form template by id.",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
		},
	},
	{
		Name: "setFormStatus", Method: "PUT", Path: "/forms/status/{formId}",
		Summary: "Set a form's status (e.g. active / template).",
		Params: []Param{
			{Name: "formId", In: InPath, Type: "number", Required: true, Desc: "Form id."},
			{Name: "status", In: InBody, Type: "string", Desc: "New form status value."},
		},
	},
	{
		Name: "searchForms", Method: "GET", Path: "/forms/search/{accId}/{q}",
		Summary: "Search form templates by name/text in a workspace. Use before createForm to check whether a form already exists.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "q", In: InPath, Type: "string", Required: true, Desc: "Search query (form name or fragment)."},
			fieldFilterParam("id,title,status"),
		},
	},
}
