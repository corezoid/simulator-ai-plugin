package tools

// appOps — applications and smart forms (CDU / Script forms). Applications are
// higher-level actors; smart forms install reusable form logic into a workspace.
var appOps = []Operation{
	{
		Name: "createApplication", Method: "POST", Path: "/applications/{accId}",
		Summary: "Create an application in the workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "ref", In: InBody, Type: "string", Required: true, Desc: "Short application reference (slug)."},
			{Name: "title", In: InBody, Type: "string", Required: true, Desc: "Application title (max 255)."},
			{Name: "corezoidCredentials", In: InBody, Type: "object", Required: true, Desc: "Credentials object with develop/production each holding {apiLogin, apiSecret, projectId, ...}."},
			{Name: "picture", In: InBody, Type: "string", Desc: "Icon storage path / URL."},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
			{Name: "withGraph", In: InQuery, Type: "boolean", Desc: "Also create the application graph."},
		},
	},
	{
		Name: "createSmartForm", Method: "POST", Path: "/smart_forms/{accId}",
		Summary: "Install a smart form (CDU / Script form) into the workspace from a file URL.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "fileUrl", In: InBody, Type: "string", Required: true, Desc: "URL of the smart-form definition file."},
			{Name: "smartFormRef", In: InBody, Type: "string", Required: true, Desc: "Smart form reference."},
			{Name: "title", In: InBody, Type: "string", Required: true, Desc: "Smart form title."},
			{Name: "ref", In: InBody, Type: "string", Required: true, Desc: "Install reference / slug."},
			{Name: "description", In: InBody, Type: "string", Desc: "Optional description."},
		},
	},
	{
		Name: "listSmartForms", Method: "GET", Path: "/smart_forms/list/{accId}",
		Summary: "List smart forms installed in the workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
		},
	},
	{
		Name: "manageAppContent", Method: "PUT", Path: "/app_content/{actorId}",
		Summary: "Create/update application content (pages/folders/files). Pass an array of {id, title, objType: folder|file, folderId?, type?, source?} objects.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Application actor UUID."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: "Array of content objects."},
		},
	},
}
