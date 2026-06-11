package tools

// attachItemDesc documents one entry of the attach/detach body array.
const attachItemDesc = "Array (min 1) of {attachId:int (attachment record id from uploadBase64 / getAttachments), " +
	"actorId:string (the actor or reaction the file is attached to)}."

// attachmentOps — workspace files (attachments) and their links to actors/reactions.
// Attachment records are created by uploading a file (uploadBase64); these ops then
// list them, link/unlink them to actors, or rename them.
var attachmentOps = []Operation{
	{
		Name: "getAttachments", Method: "GET", Path: "/attachments/{accId}",
		Summary: "List the files (attachments) in a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "starred", In: InQuery, Type: "boolean", Desc: "Only starred attachments."},
			{Name: "orderBy", In: InQuery, Type: "string", Enum: []string{"created_at", "title"}, Desc: "Sort field."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"DESC", "ASC"}, Desc: "Sort direction."},
		},
	},
	{
		Name: "getActorAttachments", Method: "GET", Path: "/attachments/actor/{actorId}",
		Summary: "List the files (attachments) linked to a specific actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Actor whose attachments to list."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-100, default 100)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
		},
	},
	{
		Name: "addAttachments", Method: "POST", Path: "/attachments/{accId}",
		Summary: "Link one or more existing uploaded files to actors/reactions in a workspace.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: attachItemDesc},
		},
	},
	{
		Name: "updateAttachment", Method: "PUT", Path: "/attachments/{attachId}",
		Summary: "Rename an attachment (set its display title).",
		Params: []Param{
			{Name: "attachId", In: InPath, Type: "number", Required: true, Desc: "Attachment record id."},
			{Name: "title", In: InBody, Type: "string", Required: true, Desc: "New display name."},
		},
	},
	{
		Name: "removeAttachments", Method: "DELETE", Path: "/attachments/{accId}",
		Summary: "Unlink one or more files from their actors/reactions. Does not delete the stored file itself.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "items", In: InBodyRoot, Type: "array", Required: true, Desc: attachItemDesc},
		},
	},
}

// uploadOps — store a file in the workspace. Only the base64 variant is exposed as
// a curated tool (a single JSON call); multipart uploads go through the
// uploadActorPicture engine tool. The response carries the attachment record (with
// its `attachId` and storage `fileName`) you then pass to addAttachments or a
// reaction's `attachments`.
var uploadOps = []Operation{
	{
		Name: "uploadBase64", Method: "POST", Path: "/upload/base64/{accId}",
		Summary: "Upload a file from base64 content; returns the stored attachment record (attachId, fileName). " +
			"Use the returned attachId with addAttachments or createReaction's attachments.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "file", In: InBody, Type: "string", Required: true, Desc: "File content as a base64 string. Prefer raw base64; a `data:<mime>;base64,` URI prefix is also accepted (it is stripped server-side)."},
			{Name: "originalName", In: InBody, Type: "string", Required: true, Desc: "Original file name with extension (e.g. report.pdf) — sets the type and title."},
			{Name: "ttl", In: InQuery, Type: "number", Desc: "Lifetime in seconds; 0 = permanent."},
			{Name: "compressionLevel", In: InQuery, Type: "string", Enum: []string{"low", "medium", "high"}, Desc: "Optional image/video compression level."},
		},
	},
}
