package engines

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/cduschema"
	"github.com/mark3labs/mcp-go/mcp"
)

// papiPOST sends an authenticated POST with a JSON body and returns the response body.
func papiPOST(apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := apiHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

// papiPUT sends an authenticated PUT with a JSON body and returns the response body.
func papiPUT(apiURL string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := apiHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PUT %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

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
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()
	actorID, _ := args["actorId"].(string)
	if actorID == "" {
		return mcp.NewToolResultError("[Error] actorId is required"), nil
	}
	if r := requireUUID("actorId", actorID); r != nil {
		return r, nil
	}

	// Only develop is writable; production is readonly on the server side.
	envDir := filepath.Join(actorID, "develop")
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

	apiURL := fmt.Sprintf("%s/app_content/%s", buildBaseURL(), seg(actorID))
	if _, err := papiPUT(apiURL, bodyBytes); err != nil {
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
