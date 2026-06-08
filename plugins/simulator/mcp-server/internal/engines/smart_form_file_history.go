package engines

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetFileHistory lists version history for a Smart Form file.
// GET /papi/1.0/file_history/<actorId>/<fileId>[?limit=<n>&offset=<n>]
// fileId is the numeric file ID from .manifest.json.
func handleGetFileHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	fileIDFloat, _ := args["fileId"].(float64)
	if fileIDFloat == 0 {
		return mcp.NewToolResultError("[Error] fileId is required"), nil
	}
	fileID := int(fileIDFloat)

	apiURL := fmt.Sprintf("%s/file_history/%s/%d", buildBaseURL(), seg(actorID), fileID)

	sep := "?"
	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		apiURL += fmt.Sprintf("%slimit=%d", sep, int(limit))
		sep = "&"
	}
	if offset, ok := args["offset"].(float64); ok && offset > 0 {
		apiURL += fmt.Sprintf("%soffset=%d", sep, int(offset))
	}

	respBytes, err := papiGET(apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] get file history: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}

// handleGetFileVersion fetches the source of one specific file version.
// GET /papi/1.0/file_history/<actorId>/<fileId>/<versionId>
func handleGetFileVersion(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	fileIDFloat, _ := args["fileId"].(float64)
	if fileIDFloat == 0 {
		return mcp.NewToolResultError("[Error] fileId is required"), nil
	}
	fileID := int(fileIDFloat)

	versionID, _ := args["versionId"].(string)
	if versionID == "" {
		return mcp.NewToolResultError("[Error] versionId is required"), nil
	}

	apiURL := fmt.Sprintf("%s/file_history/%s/%d/%s", buildBaseURL(), seg(actorID), fileID, seg(versionID))
	respBytes, err := papiGET(apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] get file version: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}

// handleRollbackFile restores a Smart Form file to a prior version.
// POST /papi/1.0/file_history/<actorId>/<fileId>/rollback
func handleRollbackFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	fileIDFloat, _ := args["fileId"].(float64)
	if fileIDFloat == 0 {
		return mcp.NewToolResultError("[Error] fileId is required"), nil
	}
	fileID := int(fileIDFloat)

	versionID, _ := args["versionId"].(string)
	if versionID == "" {
		return mcp.NewToolResultError("[Error] versionId is required"), nil
	}

	body, _ := json.Marshal(map[string]string{"versionId": versionID})
	apiURL := fmt.Sprintf("%s/file_history/%s/%d/rollback", buildBaseURL(), seg(actorID), fileID)
	respBytes, err := papiPOST(apiURL, body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] rollback file: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}

// handleListTrash lists soft-deleted objects in one environment of a Smart Form.
// GET /papi/1.0/file_history/trash/<actorId>/<envId>
// Accepts env name (e.g. "develop") and resolves to numeric envId internally.
func handleListTrash(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	envName, _ := args["env"].(string)
	if envName == "" {
		envName = "develop"
	}
	envID, err := resolveEnvID(actorID, envName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] resolve env: %v", err)), nil
	}

	apiURL := fmt.Sprintf("%s/file_history/trash/%s/%d", buildBaseURL(), seg(actorID), envID)
	respBytes, err := papiGET(apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] list trash: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}

// handleRestoreFromTrash restores a soft-deleted object from the Smart Form trash.
// POST /papi/1.0/file_history/trash/<actorId>/restore
func handleRestoreFromTrash(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	objectID, _ := args["objectId"].(string)
	if objectID == "" {
		return mcp.NewToolResultError("[Error] objectId is required"), nil
	}

	body, _ := json.Marshal(map[string]string{"id": objectID})
	apiURL := fmt.Sprintf("%s/file_history/trash/%s/restore", buildBaseURL(), seg(actorID))
	respBytes, err := papiPOST(apiURL, body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] restore from trash: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}
