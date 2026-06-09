package smartform

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Register adds all Smart Form engine tools to the MCP server.
func Register(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("createSmartForm",
			mcp.WithDescription("Create a new Smart Form (CDU / Script application) actor with develop + production environments. Corezoid credentials are optional — omit them for static/design-only forms and configure the binding later. After creation, run pullSmartForm to download the initial file tree. Requires actors.management scope."),
			mcp.WithString("title", mcp.Description("Display name of the Smart Form."), mcp.Required()),
			mcp.WithString("ref", mcp.Description("Unique slug in the workspace (lowercase letters, digits, hyphens)."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description.")),
			mcp.WithString("sharedWith", mcp.Description("Access policy: userList (default) | allWorkspaceUsers | allRegisteredUsers | anyone.")),
			mcp.WithString("picture", mcp.Description("Icon URL or storage path.")),
			mcp.WithString("corezoidCredentials", mcp.Description("Full credentials JSON: {\"develop\":{\"apiLogin\":\"...\",\"apiSecret\":\"...\",\"procId\":\"...\",\"companyId\":\"...\"},\"production\":{...}}. Use this OR the individual apiLogin/apiSecret/procId/companyId fields below.")),
			mcp.WithString("apiLogin", mcp.Description("Corezoid API login applied to both develop and production envs (ignored when corezoidCredentials is provided).")),
			mcp.WithString("apiSecret", mcp.Description("Corezoid API secret applied to both develop and production envs (ignored when corezoidCredentials is provided).")),
			mcp.WithString("procId", mcp.Description("Corezoid process ID applied to both envs (optional).")),
			mcp.WithString("companyId", mcp.Description("Corezoid company ID applied to both envs (optional).")),
		),
		handleCreateSmartForm,
	)

	s.AddTool(
		mcp.NewTool("pullSmartForm",
			mcp.WithDescription("Fetch all environment file trees (pages, locale, viewModel, styles, definitions, widgets) of a smart form (CDU / Script application) and write them to <actorId>/<envTitle>/... in the current working directory. Also writes a .manifest.json in each env folder with file IDs and content hashes for use by pushSmartForm. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart form actor UUID."), mcp.Required()),
		),
		handlePullSmartForm,
	)

	s.AddTool(
		mcp.NewTool("pushSmartForm",
			mcp.WithDescription("Reconcile the local develop tree with the server: POST any new folders (parents first) and new files (e.g. a new page like pages/<id>/config + locale), PUT any modified files, and update .manifest.json with the returned ids and content hashes. Files/folders present in the manifest but missing locally are reported as orphanFiles but never deleted server-side. MIME defaults: text/css under styles/, application/json elsewhere. Only the develop env is writable; run pullSmartForm first to create the manifest. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart form actor UUID — directory <actorId>/develop/ must exist with a .manifest.json."), mcp.Required()),
		),
		handlePushSmartForm,
	)

	s.AddTool(
		mcp.NewTool("deploySmartForm",
			mcp.WithDescription("Deploy a Smart Form environment to another (typically develop → production). Resolves env names to IDs internally — no need to look up env IDs manually. Creates a new release in the target env. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("sourceEnv", mcp.Description("Source environment name. Default: develop.")),
			mcp.WithString("targetEnv", mcp.Description("Target environment name. Default: production.")),
		),
		handleDeploySmartForm,
	)

	s.AddTool(
		mcp.NewTool("listReleases",
			mcp.WithDescription("List releases for one environment of a Smart Form. Returns id, release_number, status, and timestamps for each release. Use releaseId values from the result with diffReleases or rollbackRelease."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("env", mcp.Description("Environment name to list releases for. Default: production.")),
		),
		handleListReleases,
	)

	s.AddTool(
		mcp.NewTool("diffReleases",
			mcp.WithDescription("Return the diff (added, removed, modified files) between two releases of a Smart Form. Comparison is by source_hash — no file bytes transferred. Useful before a rollback to understand what will change."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("releaseId", mcp.Description("The release to inspect."), mcp.Required()),
			mcp.WithString("vsReleaseId", mcp.Description("The release to compare against."), mcp.Required()),
		),
		handleDiffReleases,
	)

	s.AddTool(
		mcp.NewTool("rollbackRelease",
			mcp.WithDescription("Roll back a Smart Form to a prior release. Forward-only: creates a new active release whose content equals the target release — history is never rewritten. Retention: 5 releases per env; releases outside this window cannot be rolled back. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("releaseId", mcp.Description("Release ID to roll back to (get IDs from listReleases)."), mcp.Required()),
		),
		handleRollbackRelease,
	)

	s.AddTool(
		mcp.NewTool("getFileHistory",
			mcp.WithDescription("List version history for a file in a Smart Form. Returns up to 50 versions per file with operation type (create/update/move/rename/delete), timestamps, and version IDs. fileId comes from .manifest.json."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithNumber("fileId", mcp.Description("Numeric file ID from .manifest.json."), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Page size. Default server limit applies.")),
			mcp.WithNumber("offset", mcp.Description("Pagination offset.")),
		),
		handleGetFileHistory,
	)

	s.AddTool(
		mcp.NewTool("getFileVersion",
			mcp.WithDescription("Fetch the full source of one specific version of a Smart Form file. Use getFileHistory to find version IDs."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithNumber("fileId", mcp.Description("Numeric file ID from .manifest.json."), mcp.Required()),
			mcp.WithString("versionId", mcp.Description("Version ID from getFileHistory."), mcp.Required()),
		),
		handleGetFileVersion,
	)

	s.AddTool(
		mcp.NewTool("rollbackFile",
			mcp.WithDescription("Restore a Smart Form file to a prior version. Creates a new version whose content equals the target version. Run pullSmartForm afterwards to refresh the local file and manifest. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithNumber("fileId", mcp.Description("Numeric file ID from .manifest.json."), mcp.Required()),
			mcp.WithString("versionId", mcp.Description("Version ID to restore to (from getFileHistory)."), mcp.Required()),
		),
		handleRollbackFile,
	)

	s.AddTool(
		mcp.NewTool("listTrash",
			mcp.WithDescription("List soft-deleted objects (files and folders) in one environment of a Smart Form. Returns object IDs needed for restoreFromTrash. Accepts env name; resolves to env ID internally."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("env", mcp.Description("Environment name. Default: develop.")),
		),
		handleListTrash,
	)

	s.AddTool(
		mcp.NewTool("restoreFromTrash",
			mcp.WithDescription("Restore a soft-deleted file or folder from the Smart Form trash. Use listTrash to get the objectId. Requires actors.management scope."),
			mcp.WithString("actorId", mcp.Description("Smart Form actor UUID."), mcp.Required()),
			mcp.WithString("objectId", mcp.Description("Object ID to restore (from listTrash)."), mcp.Required()),
		),
		handleRestoreFromTrash,
	)
}
