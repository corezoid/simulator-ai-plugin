package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// taskResponse is the JSON envelope returned by the pong-server task API.
type taskResponse struct {
	Data taskData `json:"data"`
}

type taskData struct {
	ID      int             `json:"id"`
	Name    string          `json:"name"`
	Status  string          `json:"status"`
	Details json.RawMessage `json:"details,omitempty"`
}

// unwrapDetails detects whether Details is a JSON-encoded string (pong-server
// stores it as TEXT in PostgreSQL) and, if so, replaces it with the parsed
// JSON object so callers see structured data instead of an escaped string.
func unwrapDetails(d json.RawMessage) json.RawMessage {
	if len(d) == 0 || d[0] != '"' {
		return d
	}
	var s string
	if json.Unmarshal(d, &s) == nil && len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		return json.RawMessage(s)
	}
	return d
}

// exportedFileName extracts details.file.fileName from an unwrapped task
// details payload, or "" if absent (e.g. the task isn't a completed export).
func exportedFileName(details json.RawMessage) string {
	var d struct {
		File struct {
			FileName string `json:"fileName"`
		} `json:"file"`
	}
	if json.Unmarshal(details, &d) != nil {
		return ""
	}
	return d.File.FileName
}

// encodeDownloadPath percent-escapes each path segment of a storage file name
// while keeping the slash separators, mirroring internal/tools/download.go's
// encodeFilePath for the same PAPI /download/{fileName} route.
func encodeDownloadPath(fileName string) string {
	parts := strings.Split(fileName, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// maxGraphFileBytes caps the download size for .graph files fetched by URL.
const maxGraphFileBytes = 100 << 20 // 100 MiB

// maxGraphFileBase64Chars caps the base64 source so decoded output can't
// exceed maxGraphFileBytes (base64 expands raw bytes by 4/3).
const maxGraphFileBase64Chars = (maxGraphFileBytes / 3) * 4

// downloadGraphFileFromURL downloads a file from a public URL.
func downloadGraphFileFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := ecore.APIHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGraphFileBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// handleExportGraph creates an async export task for graph actors.
// The caller should poll getTaskStatus until the task completes.
func handleExportGraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	accID := ecore.WorkspaceIDForContext(ctx)
	if accID == "" {
		return mcp.NewToolResultError("[Error] workspace ID is not set"), nil
	}

	args := req.GetArguments()

	var actors []string
	if raw, ok := args["actors"]; ok && raw != nil {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok && s != "" {
					if !ecore.IsUUID(s) {
						return mcp.NewToolResultError(fmt.Sprintf("[Error] actors: %q is not a valid UUID", s)), nil
					}
					actors = append(actors, s)
				}
			}
		}
	}

	var forms []int
	if raw, ok := args["forms"]; ok && raw != nil {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if n := toInt(v); n != 0 {
					forms = append(forms, n)
				}
			}
		}
	}

	allWorkspace, _ := args["allWorkspace"].(bool)

	if len(actors) == 0 && len(forms) == 0 && !allWorkspace {
		return mcp.NewToolResultError("[Error] provide at least one filter: actors, forms, or allWorkspace"), nil
	}

	// Export options (control-tasks ExportTaskOps).
	ops := map[string]interface{}{}
	for _, key := range []string{
		"attachments", "transactions", "processes", "users",
		"balances", "connectorsToAccounts", "accountToActors", "systemCounters",
	} {
		if v, ok := args[key].(bool); ok {
			ops[key] = v
		}
	}
	if n := toInt(args["maxRecursionLevel"]); n > 0 {
		ops["maxRecursionLevel"] = n
	}

	body := map[string]interface{}{}
	if len(actors) > 0 {
		body["actors"] = actors
	}
	if len(forms) > 0 {
		body["forms"] = forms
	}
	if allWorkspace {
		body["allWorkspace"] = true
	}
	if len(ops) > 0 {
		body["ops"] = ops
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal request: %v", err)), nil
	}

	url := fmt.Sprintf("%s/tasks/%s/export", ecore.BuildBaseURLForContext(ctx), ecore.Seg(accID))
	respBytes, err := ecore.PapiPOST(ctx, url, bodyBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] create export task: %v", err)), nil
	}

	var resp taskResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] parse response: %v (body: %.200s)", err, respBytes)), nil
	}

	out, _ := json.Marshal(resp.Data)
	return mcp.NewToolResultText(string(out)), nil
}

// handleImportGraph creates an async import task from a .graph file.
// The caller should poll getTaskStatus until the task completes.
func handleImportGraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	accID := ecore.WorkspaceIDForContext(ctx)
	if accID == "" {
		return mcp.NewToolResultError("[Error] workspace ID is not set"), nil
	}

	args := req.GetArguments()

	fileName, _ := args["fileName"].(string)
	if fileName == "" {
		return mcp.NewToolResultError("[Error] fileName is required"), nil
	}

	var users []map[string]interface{}
	if raw, ok := args["users"]; ok && raw != nil {
		if arr, ok := raw.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					fromID := toInt(m["fromId"])
					var toIDs []int
					if rawTo, ok := m["toIds"].([]any); ok {
						for _, v := range rawTo {
							if n := toInt(v); n != 0 {
								toIDs = append(toIDs, n)
							}
						}
					}
					if fromID != 0 || len(toIDs) > 0 {
						users = append(users, map[string]interface{}{
							"fromId": fromID,
							"toIds":  toIDs,
						})
					}
				}
			}
		}
	}

	// Import options (control-tasks ImportTaskOps).
	ops := map[string]interface{}{}
	for _, key := range []string{
		"actorRefStrategy", "actorRefReplacePrefix",
		"formRefStrategy", "formRefReplacePrefix",
		"transferRefStrategy", "transferRefReplacePrefix",
		"transactionRefStrategy", "transactionRefReplacePrefix",
		"processesRefStrategy",
	} {
		if v, _ := args[key].(string); v != "" {
			ops[key] = v
		}
	}

	// dataReplace — from/to replacement rules (passed through as-is).
	var dataReplace []interface{}
	if raw, ok := args["dataReplace"]; ok && raw != nil {
		if arr, ok := raw.([]any); ok {
			dataReplace = arr
		}
	}

	body := map[string]interface{}{
		"fileName": fileName,
	}
	if len(users) > 0 {
		body["users"] = users
	}
	if len(ops) > 0 {
		body["ops"] = ops
	}
	if len(dataReplace) > 0 {
		body["dataReplace"] = dataReplace
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal request: %v", err)), nil
	}

	url := fmt.Sprintf("%s/tasks/%s/import", ecore.BuildBaseURLForContext(ctx), ecore.Seg(accID))
	respBytes, err := ecore.PapiPOST(ctx, url, bodyBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] create import task: %v", err)), nil
	}

	var resp taskResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] parse response: %v (body: %.200s)", err, respBytes)), nil
	}

	out, _ := json.Marshal(resp.Data)
	return mcp.NewToolResultText(string(out)), nil
}

// handleGetTaskStatus polls the status of an async import or export task.
func handleGetTaskStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	accID := ecore.WorkspaceIDForContext(ctx)
	if accID == "" {
		return mcp.NewToolResultError("[Error] workspace ID is not set"), nil
	}

	args := req.GetArguments()

	taskID := toInt(args["taskId"])
	if taskID <= 0 {
		return mcp.NewToolResultError("[Error] taskId is required and must be a positive number"), nil
	}

	name, _ := args["name"].(string)
	if name != "import" && name != "export" {
		return mcp.NewToolResultError("[Error] name is required and must be \"import\" or \"export\""), nil
	}

	apiURL := fmt.Sprintf("%s/tasks/%s/%s/%d", ecore.BuildBaseURLForContext(ctx), ecore.Seg(accID), ecore.Seg(name), taskID)
	respBytes, err := ecore.PapiGET(ctx, apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] get task status: %v", err)), nil
	}

	var resp taskResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] parse response: %v (body: %.200s)", err, respBytes)), nil
	}

	resp.Data.Details = unwrapDetails(resp.Data.Details)

	outMap := map[string]interface{}{
		"id":      resp.Data.ID,
		"name":    resp.Data.Name,
		"status":  resp.Data.Status,
		"details": resp.Data.Details,
	}
	if fileName := exportedFileName(resp.Data.Details); fileName != "" {
		outMap["downloadUrl"] = fmt.Sprintf("%s/download/%s", ecore.BuildBaseURLForContext(ctx), encodeDownloadPath(fileName))
	}

	out, _ := json.Marshal(outMap)
	return mcp.NewToolResultText(string(out)), nil
}

// handleUploadGraphFile uploads a .graph file to the simulator storage so it
// can be used with importGraph. Accepts the file as base64 content or as a
// public URL. Returns the storage fileName for use as importGraph's fileName.
func handleUploadGraphFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	accID := ecore.WorkspaceIDForContext(ctx)
	if accID == "" {
		return mcp.NewToolResultError("[Error] workspace ID is not set"), nil
	}

	args := req.GetArguments()

	var (
		fileBytes []byte
		filename  string
	)

	if b64, ok := args["base64"].(string); ok && b64 != "" {
		// Strip optional data URI prefix.
		if i := strings.Index(b64, "base64,"); i >= 0 {
			b64 = b64[i+len("base64,"):]
		}
		if len(b64) > maxGraphFileBase64Chars {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] base64 payload too large (max %d MiB)", maxGraphFileBytes>>20)), nil
		}
		b, err := decodeBase64Flexible(b64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] decode base64: %v", err)), nil
		}
		fileBytes = b
	} else if u, ok := args["fileUrl"].(string); ok && u != "" {
		b, err := downloadGraphFileFromURL(ctx, u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] download fileUrl: %v", err)), nil
		}
		fileBytes = b
		filename = filepath.Base(u)
		if i := strings.IndexAny(filename, "?#"); i >= 0 {
			filename = filename[:i]
		}
	} else {
		return mcp.NewToolResultError("[Error] one of base64 or fileUrl is required"), nil
	}

	if fn, ok := args["originalName"].(string); ok && fn != "" {
		filename = fn
	}
	if filename == "" {
		filename = "import.graph"
	}

	storageName, err := uploadFile(ctx, accID, filename, "application/octet-stream", fileBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] upload: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]string{
		"fileName": storageName,
		"title":    filename,
	})
	return mcp.NewToolResultText(string(out)), nil
}
