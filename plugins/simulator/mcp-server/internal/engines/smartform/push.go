package smartform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/cduschema"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// createItem is one entry in the POST /app_content/:actorId batch (creating
// folders/files). For folders, only folderId+title+objType are set; for files
// type and source are populated as well.
type createItem struct {
	ObjType  string `json:"objType"`
	FolderID int    `json:"folderId"`
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
}

// updateItem is one entry in the PUT /app_content/:actorId batch (modifying
// existing files in place).
type updateItem struct {
	ID       int    `json:"id"`
	ObjType  string `json:"objType"`
	FolderID int    `json:"folderId"`
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
}

// createdObj is one element of the POST/PUT response payload.
type createdObj struct {
	ID       int    `json:"id"`
	ObjType  string `json:"objType"`
	FolderID int    `json:"folderId"`
	ParentID int    `json:"parentId"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Source   string `json:"source"`
}

// handlePushSmartForm reconciles the local develop env files with the server.
//
// It walks the local develop directory, diffs against .manifest.json, and:
//   - POSTs new folders (parents first, so child paths can resolve their parent id),
//   - POSTs new files (using either the freshly-created folder ids or ids from manifest),
//   - PUTs modified files (existing behaviour),
//   - updates .manifest.json with returned ids + content hashes.
//
// Files/folders present in the manifest but missing locally are reported but
// not deleted from the server (use the manage-trash flow for explicit deletes).
func handlePushSmartForm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()
	actorID, _ := args["actorId"].(string)
	if actorID == "" {
		return mcp.NewToolResultError("[Error] actorId is required"), nil
	}
	if r := ecore.RequireUUID("actorId", actorID); r != nil {
		return r, nil
	}

	envDir := ecore.ResolvePath(actorID, "develop")
	manifestPath := filepath.Join(envDir, manifestFileName)

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] cannot read %s — run pullSmartForm first: %v", manifestPath, err)), nil
	}
	var manifest smartFormManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] parse manifest: %v", err)), nil
	}
	if manifest.Folders == nil {
		manifest.Folders = make(map[string]int)
	}
	if manifest.Files == nil {
		manifest.Files = make(map[string]manifestNode)
	}

	// Old manifests (pulled before folder tracking was added) miss the folder
	// map and the env root folder id. Re-fetch the env tree to backfill them
	// so creates can resolve parent ids — no force-repull required.
	if manifest.EnvRootFolderID == 0 || len(manifest.Folders) == 0 {
		if err := backfillManifestFromServer(actorID, &manifest); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] backfill manifest: %v", err)), nil
		}
	}

	// Walk the local develop dir to discover files and folders.
	localFiles, localFolders, err := scanLocalEnv(envDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] scan local files: %v", err)), nil
	}

	// Diff folders.
	var newFolderPaths []string
	for path := range localFolders {
		if _, ok := manifest.Folders[path]; !ok {
			newFolderPaths = append(newFolderPaths, path)
		}
	}
	// Parents must exist before children: sort by depth, then lexicographically.
	sort.Slice(newFolderPaths, func(i, j int) bool {
		di := strings.Count(newFolderPaths[i], "/")
		dj := strings.Count(newFolderPaths[j], "/")
		if di != dj {
			return di < dj
		}
		return newFolderPaths[i] < newFolderPaths[j]
	})

	// Diff files: new (not in manifest) vs modified (in manifest but hash differs).
	var newFilePaths []string
	var modifiedFilePaths []string
	newHashes := make(map[string]string, len(localFiles))
	unchanged := 0
	for relPath, source := range localFiles {
		h := hashSource(source)
		newHashes[relPath] = h
		node, ok := manifest.Files[relPath]
		switch {
		case !ok:
			newFilePaths = append(newFilePaths, relPath)
		case node.Hash != h:
			modifiedFilePaths = append(modifiedFilePaths, relPath)
		default:
			unchanged++
		}
	}
	sort.Strings(newFilePaths)
	sort.Strings(modifiedFilePaths)

	// Validate every file we're about to write — both creates and updates —
	// before we touch the server, so a partial push doesn't leave inconsistent
	// state on the backend.
	var validationErrors []string
	for _, p := range newFilePaths {
		if errs := cduschema.ValidateFile(p, localFiles[p]); len(errs) > 0 {
			validationErrors = append(validationErrors, errs...)
		}
	}
	for _, p := range modifiedFilePaths {
		if errs := cduschema.ValidateFile(p, localFiles[p]); len(errs) > 0 {
			validationErrors = append(validationErrors, errs...)
		}
	}
	if len(validationErrors) > 0 {
		out, _ := json.Marshal(map[string]interface{}{
			"actorId":          actorID,
			"env":              "develop",
			"validationErrors": validationErrors,
			"message":          "push aborted: fix the validation errors below and retry",
		})
		return mcp.NewToolResultError(string(out)), nil
	}

	if len(newFolderPaths) == 0 && len(newFilePaths) == 0 && len(modifiedFilePaths) == 0 {
		out, _ := json.Marshal(map[string]interface{}{
			"actorId":   actorID,
			"env":       "develop",
			"created":   map[string]int{"folders": 0, "files": 0},
			"updated":   0,
			"unchanged": unchanged,
			"message":   "nothing to push",
		})
		return mcp.NewToolResultText(string(out)), nil
	}

	apiURL := fmt.Sprintf("%s/app_content/%s", ecore.BuildBaseURL(), ecore.Seg(actorID))

	// Phase 1 — create missing folders, parents first, one POST per folder so
	// we can pair the returned id back to its path deterministically.
	createdFolders := 0
	for _, relPath := range newFolderPaths {
		parentID, ok := resolveParentID(relPath, manifest.Folders, manifest.EnvRootFolderID)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] cannot resolve parent folder id for %q — run pullSmartForm to refresh manifest", relPath)), nil
		}
		batch := []createItem{{
			ObjType:  "folder",
			FolderID: parentID,
			Title:    filepath.Base(relPath),
		}}
		body, _ := json.Marshal(batch)
		respBytes, err := ecore.PapiPOST(apiURL, body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] create folder %q: %v", relPath, err)), nil
		}
		newID, err := pickCreatedID(respBytes, "folder")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse create-folder response for %q: %v", relPath, err)), nil
		}
		manifest.Folders[relPath] = newID
		createdFolders++
	}

	// Phase 2 — create missing files in one batch.
	createdFiles := 0
	if len(newFilePaths) > 0 {
		batch := make([]createItem, 0, len(newFilePaths))
		for _, relPath := range newFilePaths {
			parentID, ok := resolveParentID(relPath, manifest.Folders, manifest.EnvRootFolderID)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] cannot resolve parent folder id for %q — run pullSmartForm to refresh manifest", relPath)), nil
			}
			batch = append(batch, createItem{
				ObjType:  "file",
				FolderID: parentID,
				Title:    filepath.Base(relPath),
				Type:     defaultMimeType(relPath),
				Source:   localFiles[relPath],
			})
		}
		body, _ := json.Marshal(batch)
		respBytes, err := ecore.PapiPOST(apiURL, body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] create files: %v", err)), nil
		}
		created, err := parseCreatedObjs(respBytes)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse create-files response: %v", err)), nil
		}
		// The server returns the created objects with their ids; map them back
		// to the input by (folderId, title), which is unique within a folder.
		byKey := make(map[string]createdObj, len(created))
		for _, c := range created {
			if c.ObjType != "file" {
				continue
			}
			byKey[fmt.Sprintf("%d/%s", c.FolderID, c.Title)] = c
		}
		for _, relPath := range newFilePaths {
			parentID := manifest.Folders[filepath.Dir(relPath)]
			if filepath.Dir(relPath) == "." {
				parentID = manifest.EnvRootFolderID
			}
			key := fmt.Sprintf("%d/%s", parentID, filepath.Base(relPath))
			c, ok := byKey[key]
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] server did not return id for created file %q", relPath)), nil
			}
			manifest.Files[relPath] = manifestNode{
				FileID:   c.ID,
				FolderID: c.FolderID,
				MimeType: c.Type,
				Hash:     newHashes[relPath],
			}
			createdFiles++
		}
	}

	// Phase 3 — update modified existing files (one batch PUT).
	updated := 0
	if len(modifiedFilePaths) > 0 {
		batch := make([]updateItem, 0, len(modifiedFilePaths))
		for _, relPath := range modifiedFilePaths {
			node := manifest.Files[relPath]
			batch = append(batch, updateItem{
				ID:       node.FileID,
				ObjType:  "file",
				FolderID: node.FolderID,
				Title:    filepath.Base(relPath),
				Type:     node.MimeType,
				Source:   localFiles[relPath],
			})
		}
		body, _ := json.Marshal(batch)
		if _, err := ecore.PapiPUT(apiURL, body); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] update files: %v", err)), nil
		}
		for _, relPath := range modifiedFilePaths {
			node := manifest.Files[relPath]
			node.Hash = newHashes[relPath]
			manifest.Files[relPath] = node
			updated++
		}
	}

	// Persist the updated manifest.
	updatedManifest, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, updatedManifest, 0600); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] write manifest: %v", err)), nil
	}

	// Report manifest entries with no local counterpart so the caller can
	// notice (and reach for an explicit delete tool when one exists). We do
	// not remove them server-side here — destructive ops stay opt-in.
	var orphanFiles []string
	for relPath := range manifest.Files {
		if _, ok := localFiles[relPath]; !ok {
			orphanFiles = append(orphanFiles, relPath)
		}
	}
	sort.Strings(orphanFiles)

	out, _ := json.Marshal(map[string]interface{}{
		"actorId": actorID,
		"env":     "develop",
		"created": map[string]int{
			"folders": createdFolders,
			"files":   createdFiles,
		},
		"updated":           updated,
		"unchanged":         unchanged,
		"orphanFiles":       orphanFiles,
		"createdFolderPath": newFolderPaths,
		"createdFilePath":   newFilePaths,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// scanLocalEnv walks dir and returns:
//   - files: env-relative path → file source,
//   - folders: set of env-relative folder paths (excluding the env root "").
//
// The manifest file itself is excluded.
func scanLocalEnv(dir string) (map[string]string, map[string]struct{}, error) {
	files := make(map[string]string)
	folders := make(map[string]struct{})
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			folders[rel] = struct{}{}
			return nil
		}
		if filepath.Base(rel) == manifestFileName {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", rel, readErr)
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return files, folders, nil
}

// resolveParentID returns the folder id for the parent directory of relPath,
// looking it up in folders (env-relative dir → id), or envRootFolderID when
// the parent is the env root. Reports ok=false when neither resolves.
func resolveParentID(relPath string, folders map[string]int, envRootFolderID int) (int, bool) {
	parent := filepath.Dir(relPath)
	if parent == "." || parent == "/" || parent == "" {
		if envRootFolderID == 0 {
			return 0, false
		}
		return envRootFolderID, true
	}
	parent = filepath.ToSlash(parent)
	id, ok := folders[parent]
	return id, ok
}

// pickCreatedID extracts the single created object id from a POST response of
// the form { "data": [ { id, objType, ... } ] }, matching objType.
func pickCreatedID(respBytes []byte, objType string) (int, error) {
	created, err := parseCreatedObjs(respBytes)
	if err != nil {
		return 0, err
	}
	for _, c := range created {
		if c.ObjType == objType {
			return c.ID, nil
		}
	}
	return 0, fmt.Errorf("no %s object in response: %.200s", objType, respBytes)
}

// parseCreatedObjs unmarshals a { "data": [...] } envelope from POST/PUT.
func parseCreatedObjs(respBytes []byte) ([]createdObj, error) {
	var resp struct {
		Data []createdObj `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("%w (body: %.200s)", err, respBytes)
	}
	return resp.Data, nil
}

// defaultMimeType returns the MIME type to assign to a newly-created file
// based on its env-relative path. The default skeleton uses application/json
// everywhere except the styles tree, which is Less/CSS source.
func defaultMimeType(relPath string) string {
	if relPath == "styles" || strings.HasPrefix(relPath, "styles/") {
		return "text/css"
	}
	return "application/json"
}

// backfillManifestFromServer fetches the env struct and rebuilds the folder
// map + env root folder id on a manifest pulled before folder tracking was
// added. Files already in the manifest are left untouched.
func backfillManifestFromServer(actorID string, manifest *smartFormManifest) error {
	if manifest.EnvID == 0 {
		return fmt.Errorf("manifest has no envId — run pullSmartForm to refresh")
	}
	tree, err := fetchEnvStruct(actorID, manifest.EnvID)
	if err != nil {
		return err
	}
	folders := make(map[string]int)
	var rootFolderID int
	collectFoldersFromTree(*tree, "", folders, &rootFolderID)
	manifest.Folders = folders
	if rootFolderID != 0 {
		manifest.EnvRootFolderID = rootFolderID
	}
	return nil
}

// collectFoldersFromTree walks the env struct returned by app_content/struct
// and records every folder's id by its env-relative path. It also captures
// the env root folder id from any top-level child's parent reference.
func collectFoldersFromTree(node appTreeNode, relDir string, folders map[string]int, rootFolderID *int) {
	for _, child := range node.Children {
		if relDir == "" && *rootFolderID == 0 {
			switch child.ObjType {
			case "file":
				*rootFolderID = child.FolderID
			case "folder":
				*rootFolderID = child.ParentID
			}
		}
		if child.ObjType != "folder" {
			continue
		}
		childRel := child.Title
		if relDir != "" {
			childRel = relDir + "/" + child.Title
		}
		folders[childRel] = child.ID
		collectFoldersFromTree(child, childRel, folders, rootFolderID)
	}
}
