package smartform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---- API response types ----

type appEnvItem struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Readonly bool   `json:"readonly"`
}

// appTreeNode is one node in the env tree returned by app_content/struct.
// The server returns a flat children array; objType "folder"|"file" distinguishes the two.
type appTreeNode struct {
	ID       int           `json:"id"`
	FolderID int           `json:"folderId"` // parent folder id (files only)
	ParentID int           `json:"parentId"` // parent folder id (folders only)
	Title    string        `json:"title"`
	ObjType  string        `json:"objType"` // "folder" or "file"
	Type     string        `json:"type"`    // MIME type (files only)
	Source   string        `json:"source"`
	Children []appTreeNode `json:"children"`
}

// ---- Manifest types ----

type manifestNode struct {
	FileID   int    `json:"fileId"`
	FolderID int    `json:"folderId"`
	MimeType string `json:"mimeType"`
	Hash     string `json:"hash"` // SHA-256 of source at pull time, for change detection
}

type smartFormManifest struct {
	ActorID         string                  `json:"actorId"`
	EnvID           int                     `json:"envId"`
	EnvName         string                  `json:"envName"`
	EnvRootFolderID int                     `json:"envRootFolderId,omitempty"`
	Folders         map[string]int          `json:"folders,omitempty"` // env-relative dir → folder id ("" = env root)
	Files           map[string]manifestNode `json:"files"`             // env-relative path → node
}

const manifestFileName = ".manifest.json"

func hashSource(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---- Fetch helpers ----

func fetchAppEnvs(ctx context.Context, actorID string) ([]appEnvItem, error) {
	body, err := ecore.PapiGET(ctx, fmt.Sprintf("%s/applications/envs/%s", ecore.BuildBaseURLForContext(ctx), ecore.Seg(actorID)))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []appEnvItem `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse envs: %w (body: %.200s)", err, body)
	}
	return resp.Data, nil
}

func fetchEnvStruct(ctx context.Context, actorID string, envID int) (*appTreeNode, error) {
	body, err := ecore.PapiGET(ctx, fmt.Sprintf("%s/app_content/struct/%s/%d", ecore.BuildBaseURLForContext(ctx), ecore.Seg(actorID), envID))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data appTreeNode `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse env struct: %w (body: %.200s)", err, body)
	}
	return &resp.Data, nil
}

// writeEnvTree recursively writes file sources to disk and populates the
// manifest maps: files (env-relative path → node) and folders (env-relative
// dir → folder id). rootFolderID receives the env root folder id when the
// top-level call discovers it from a child's parent reference. Returns total
// files written.
func writeEnvTree(node appTreeNode, dir, relDir string, files map[string]manifestNode, folders map[string]int, rootFolderID *int) (int, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	count := 0
	for _, child := range node.Children {
		// At the top level any child's parent reference (folderId on files,
		// parentId on folders) equals the env root folder id — capture it
		// for the manifest so push can create new top-level objects.
		if relDir == "" && rootFolderID != nil && *rootFolderID == 0 {
			switch child.ObjType {
			case "file":
				*rootFolderID = child.FolderID
			case "folder":
				*rootFolderID = child.ParentID
			}
		}
		childRel := child.Title
		if relDir != "" {
			childRel = relDir + "/" + child.Title
		}
		switch child.ObjType {
		case "file":
			if err := os.WriteFile(filepath.Join(dir, child.Title), []byte(child.Source), 0600); err != nil {
				return count, fmt.Errorf("write %s: %w", childRel, err)
			}
			files[childRel] = manifestNode{
				FileID:   child.ID,
				FolderID: child.FolderID,
				MimeType: child.Type,
				Hash:     hashSource(child.Source),
			}
			count++
		case "folder":
			folders[childRel] = child.ID
			n, err := writeEnvTree(child, filepath.Join(dir, child.Title), childRel, files, folders, rootFolderID)
			count += n
			if err != nil {
				return count, err
			}
		}
	}
	return count, nil
}

// handlePullSmartForm fetches the full file tree of every environment of a smart
// form and writes the files to <SIMULATOR_WORK_DIR>/<actorId>/<envTitle>/...
// (falling back to cwd when the env var is unset — see ecore.WorkDir). A
// .manifest.json is written in each env folder to track file IDs and content
// hashes (used by pushSmartForm for diffing).
//
// Conflict detection: if a prior manifest exists and any local file has been
// modified since the last pull (local hash ≠ manifest hash), the pull refuses
// to overwrite those local edits unless force=true is passed. This prevents
// silent loss of work when a pushSmartForm succeeded server-side but the caller
// treated it as failed and edited locally a second time.
func handlePullSmartForm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	force, _ := args["force"].(bool)

	envs, err := fetchAppEnvs(ctx, actorID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch envs: %v", err)), nil
	}
	if len(envs) == 0 {
		return mcp.NewToolResultError("[Error] no environments found for this actor"), nil
	}

	type envSummary struct {
		Env   string `json:"env"`
		Dir   string `json:"dir"`
		Files int    `json:"files"`
	}
	var summary []envSummary

	baseDir := ecore.ResolvePath(actorID)
	for _, env := range envs {
		envDir := filepath.Join(baseDir, env.Title)

		// Conflict detection: compare local files against the stored manifest
		// hashes. A file is "locally modified" when it exists on disk with a
		// hash that differs from what was recorded at the last pull — that means
		// the caller edited it (possibly after a push that succeeded server-side
		// but appeared to fail locally). Overwriting such a file silently would
		// discard unsynced work.
		if !force {
			if conflicts := detectPullConflicts(envDir); len(conflicts) > 0 {
				out, _ := json.Marshal(map[string]interface{}{
					"actorId":   actorID,
					"env":       env.Title,
					"conflicts": conflicts,
					"message": "pull aborted: local files have unsaved edits that differ from the last-pulled " +
						"state. Push them first (pushSmartForm) or re-run pullSmartForm with force=true to " +
						"discard local changes.",
				})
				return mcp.NewToolResultError(string(out)), nil
			}
		}

		tree, err := fetchEnvStruct(ctx, actorID, env.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch struct for env %q (id=%d): %v", env.Title, env.ID, err)), nil
		}
		files := make(map[string]manifestNode)
		folders := make(map[string]int)
		var rootFolderID int
		n, err := writeEnvTree(*tree, envDir, "", files, folders, &rootFolderID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] write env %q: %v", env.Title, err)), nil
		}

		manifest := smartFormManifest{
			ActorID:         actorID,
			EnvID:           env.ID,
			EnvName:         env.Title,
			EnvRootFolderID: rootFolderID,
			Folders:         folders,
			Files:           files,
		}
		manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(filepath.Join(envDir, manifestFileName), manifestBytes, 0600); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] write manifest for env %q: %v", env.Title, err)), nil
		}

		summary = append(summary, envSummary{Env: env.Title, Dir: envDir, Files: n})
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId": actorID,
		"baseDir": baseDir,
		"envs":    summary,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// detectPullConflicts loads the existing .manifest.json from envDir (if any)
// and returns the env-relative paths of every locally-modified file — i.e.
// files whose on-disk content hash differs from the hash recorded at the last
// pull. Returns nil when no manifest exists yet (first-ever pull is always safe)
// or when no local modifications are detected.
func detectPullConflicts(envDir string) []string {
	manifestPath := filepath.Join(envDir, manifestFileName)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		// No manifest → first pull, no conflicts possible.
		return nil
	}
	var manifest smartFormManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		// Corrupt manifest: can't tell; treat as no conflicts (pull will fix it).
		return nil
	}

	var conflicts []string
	for relPath, node := range manifest.Files {
		localPath := filepath.Join(envDir, filepath.FromSlash(relPath))
		data, err := os.ReadFile(localPath)
		if err != nil {
			// File not present locally: not a conflict (it's an orphan the
			// pull will restore; that's expected behaviour).
			continue
		}
		if hashSource(string(data)) != node.Hash {
			conflicts = append(conflicts, relPath)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}
