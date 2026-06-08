package smartform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	ActorID string                  `json:"actorId"`
	EnvID   int                     `json:"envId"`
	EnvName string                  `json:"envName"`
	Files   map[string]manifestNode `json:"files"` // env-relative path → node
}

const manifestFileName = ".manifest.json"

func hashSource(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---- Fetch helpers ----

func fetchAppEnvs(actorID string) ([]appEnvItem, error) {
	body, err := ecore.PapiGET(fmt.Sprintf("%s/applications/envs/%s", ecore.BuildBaseURL(), ecore.Seg(actorID)))
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

func fetchEnvStruct(actorID string, envID int) (*appTreeNode, error) {
	body, err := ecore.PapiGET(fmt.Sprintf("%s/app_content/struct/%s/%d", ecore.BuildBaseURL(), ecore.Seg(actorID), envID))
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

// writeEnvTree recursively writes file sources to disk and populates files with
// manifest entries (env-relative path → manifestNode). Returns total files written.
func writeEnvTree(node appTreeNode, dir, relDir string, files map[string]manifestNode) (int, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	count := 0
	for _, child := range node.Children {
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
			n, err := writeEnvTree(child, filepath.Join(dir, child.Title), childRel, files)
			count += n
			if err != nil {
				return count, err
			}
		}
	}
	return count, nil
}

// handlePullSmartForm fetches the full file tree of every environment of a smart
// form and writes the files to <actorId>/<envTitle>/... in the current working
// directory. A .manifest.json is written in each env folder to track file IDs and
// content hashes (used by pushSmartForm for diffing).
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

	envs, err := fetchAppEnvs(actorID)
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

	for _, env := range envs {
		tree, err := fetchEnvStruct(actorID, env.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch struct for env %q (id=%d): %v", env.Title, env.ID, err)), nil
		}
		envDir := filepath.Join(actorID, env.Title)
		files := make(map[string]manifestNode)
		n, err := writeEnvTree(*tree, envDir, "", files)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] write env %q: %v", env.Title, err)), nil
		}

		manifest := smartFormManifest{
			ActorID: actorID,
			EnvID:   env.ID,
			EnvName: env.Title,
			Files:   files,
		}
		manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(filepath.Join(envDir, manifestFileName), manifestBytes, 0600); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] write manifest for env %q: %v", env.Title, err)), nil
		}

		summary = append(summary, envSummary{Env: env.Title, Dir: envDir, Files: n})
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId": actorID,
		"baseDir": actorID,
		"envs":    summary,
	})
	return mcp.NewToolResultText(string(out)), nil
}
