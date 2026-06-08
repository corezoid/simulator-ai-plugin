package smartform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/cduschema"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// appContentItem is one entry in the PUT /app_content/:actorId batch.
type appContentItem struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	ObjType  string `json:"objType"`
	FolderID int    `json:"folderId"`
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
}

// handlePushSmartForm reads the local develop env files, diffs them against the
// hashes stored in .manifest.json, and sends changed files to the server in a
// single batch PUT. Updates .manifest.json hashes on success.
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

	// Only develop is writable; production is readonly on the server side.
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

	// Diff: collect files whose content changed since the last pull/push.
	// Validate changed files against the CDU page protocol schema before sending.
	var batch []appContentItem
	var validationErrors []string
	newHashes := make(map[string]string, len(manifest.Files))
	unchanged := 0

	for relPath, node := range manifest.Files {
		content, err := os.ReadFile(filepath.Join(envDir, relPath))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] read %s: %v", relPath, err)), nil
		}
		source := string(content)
		currentHash := hashSource(source)
		newHashes[relPath] = currentHash
		if currentHash == node.Hash {
			unchanged++
			continue
		}
		if errs := cduschema.ValidateFile(relPath, source); len(errs) > 0 {
			validationErrors = append(validationErrors, errs...)
		}
		batch = append(batch, appContentItem{
			ID:       node.FileID,
			Title:    filepath.Base(relPath),
			ObjType:  "file",
			FolderID: node.FolderID,
			Type:     node.MimeType,
			Source:   source,
		})
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

	if len(batch) == 0 {
		out, _ := json.Marshal(map[string]interface{}{
			"actorId":   actorID,
			"env":       "develop",
			"changed":   0,
			"unchanged": unchanged,
			"message":   "nothing to push",
		})
		return mcp.NewToolResultText(string(out)), nil
	}

	bodyBytes, err := json.Marshal(batch)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal batch: %v", err)), nil
	}

	apiURL := fmt.Sprintf("%s/app_content/%s", ecore.BuildBaseURL(), ecore.Seg(actorID))
	if _, err := ecore.PapiPUT(apiURL, bodyBytes); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] push: %v", err)), nil
	}

	// Update hashes in manifest for successfully pushed files.
	for relPath, node := range manifest.Files {
		if h, ok := newHashes[relPath]; ok {
			node.Hash = h
			manifest.Files[relPath] = node
		}
	}
	updatedManifest, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(manifestPath, updatedManifest, 0600)

	out, _ := json.Marshal(map[string]interface{}{
		"actorId":   actorID,
		"env":       "develop",
		"changed":   len(batch),
		"unchanged": unchanged,
	})
	return mcp.NewToolResultText(string(out)), nil
}
