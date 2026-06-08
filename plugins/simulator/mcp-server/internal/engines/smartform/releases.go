package smartform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// resolveEnvID finds an env by title (case-insensitive) and returns its numeric ID.
func resolveEnvID(actorID, envTitle string) (int, error) {
	envs, err := fetchAppEnvs(actorID)
	if err != nil {
		return 0, err
	}
	for _, e := range envs {
		if e.Title == envTitle {
			return e.ID, nil
		}
	}
	titles := make([]string, len(envs))
	for i, e := range envs {
		titles[i] = e.Title
	}
	return 0, fmt.Errorf("env %q not found; available: %v", envTitle, titles)
}

// handleDeploySmartForm deploys one environment of a Smart Form to another
// by POSTing to POST /papi/1.0/applications/deploy/<actorId>.
func handleDeploySmartForm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	sourceEnv, _ := args["sourceEnv"].(string)
	if sourceEnv == "" {
		sourceEnv = "develop"
	}
	targetEnv, _ := args["targetEnv"].(string)
	if targetEnv == "" {
		targetEnv = "production"
	}

	sourceID, err := resolveEnvID(actorID, sourceEnv)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] resolve source env: %v", err)), nil
	}
	targetID, err := resolveEnvID(actorID, targetEnv)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] resolve target env: %v", err)), nil
	}

	body, _ := json.Marshal(map[string]int{
		"sourceEnvId": sourceID,
		"targetEnvId": targetID,
	})
	apiURL := fmt.Sprintf("%s/applications/deploy/%s", ecore.BuildBaseURL(), ecore.Seg(actorID))
	respBytes, err := ecore.PapiPOST(apiURL, body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] deploy: %v", err)), nil
	}

	var resp struct {
		OK      bool `json:"ok"`
		Release struct {
			ID            int    `json:"id"`
			ReleaseNumber int    `json:"release_number"`
			Status        string `json:"status"`
		} `json:"release"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil || !resp.OK {
		return mcp.NewToolResultText(string(respBytes)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId":       actorID,
		"sourceEnv":     sourceEnv,
		"targetEnv":     targetEnv,
		"releaseId":     resp.Release.ID,
		"releaseNumber": resp.Release.ReleaseNumber,
		"status":        resp.Release.Status,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// handleListReleases lists releases for one environment of a Smart Form.
// Calls GET /papi/1.0/releases/<actorId>?envId=<envId>.
func handleListReleases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	envName, _ := args["env"].(string)
	if envName == "" {
		envName = "production"
	}
	envID, err := resolveEnvID(actorID, envName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] resolve env: %v", err)), nil
	}

	apiURL := fmt.Sprintf("%s/releases/%s?envId=%d", ecore.BuildBaseURL(), ecore.Seg(actorID), envID)
	respBytes, err := ecore.PapiGET(apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] list releases: %v", err)), nil
	}

	// Normalise: return { actorId, env, releases: [...] } when parseable.
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil || raw.Data == nil {
		return mcp.NewToolResultText(string(respBytes)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId":  actorID,
		"env":      envName,
		"releases": raw.Data,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// handleDiffReleases returns the diff (added/removed/modified) between two releases.
// Calls GET /papi/1.0/releases/<actorId>/<releaseId>/diff?vs=<vsReleaseId>.
func handleDiffReleases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	releaseID, _ := args["releaseId"].(string)
	if releaseID == "" {
		return mcp.NewToolResultError("[Error] releaseId is required"), nil
	}
	vsReleaseID, _ := args["vsReleaseId"].(string)
	if vsReleaseID == "" {
		return mcp.NewToolResultError("[Error] vsReleaseId is required"), nil
	}

	apiURL := fmt.Sprintf("%s/releases/%s/%s/diff?vs=%s",
		ecore.BuildBaseURL(), ecore.Seg(actorID), ecore.Seg(releaseID), ecore.Seg(vsReleaseID))
	respBytes, err := ecore.PapiGET(apiURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] diff releases: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBytes)), nil
}

// handleRollbackRelease rolls back a Smart Form environment to a prior release.
// Calls POST /papi/1.0/releases/<actorId>/<releaseId>/rollback.
// Rollback is forward-only: a new active release is created with the target's content.
func handleRollbackRelease(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	releaseID, _ := args["releaseId"].(string)
	if releaseID == "" {
		return mcp.NewToolResultError("[Error] releaseId is required"), nil
	}

	apiURL := fmt.Sprintf("%s/releases/%s/%s/rollback", ecore.BuildBaseURL(), ecore.Seg(actorID), ecore.Seg(releaseID))
	respBytes, err := ecore.PapiPOST(apiURL, []byte("{}"))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] rollback: %v", err)), nil
	}

	var resp struct {
		OK      bool `json:"ok"`
		Release struct {
			ID            int    `json:"id"`
			ReleaseNumber int    `json:"release_number"`
			Status        string `json:"status"`
		} `json:"release"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil || !resp.OK {
		return mcp.NewToolResultText(string(respBytes)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId":       actorID,
		"rolledBackTo":  releaseID,
		"newReleaseId":  resp.Release.ID,
		"releaseNumber": resp.Release.ReleaseNumber,
		"status":        resp.Release.Status,
	})
	return mcp.NewToolResultText(string(out)), nil
}
