package smartform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleUpdateSmartFormEnv updates the Corezoid credentials bound to one
// environment of a Smart Form via PUT /papi/1.0/applications/env/{actorId}/{envId}.
// Accepts env name (develop/production) and resolves to numeric envId internally.
// Updating develop credentials does NOT create a release; production credentials
// are updated independently. Requires actors.management scope.
func handleUpdateSmartFormEnv(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	// Resolve env name → numeric ID.
	envName, _ := args["env"].(string)
	if envName == "" {
		envName = "develop"
	}
	envID, err := resolveEnvID(ctx, actorID, envName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] resolve env: %v", err)), nil
	}

	apiLogin, _ := args["apiLogin"].(string)
	apiSecret, _ := args["apiSecret"].(string)
	procID, _ := args["procId"].(string)
	companyID, _ := args["companyId"].(string)

	payload := map[string]string{
		"apiLogin":  apiLogin,
		"apiSecret": apiSecret,
		"procId":    procID,
		"companyId": companyID,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal: %v", err)), nil
	}

	apiURL := fmt.Sprintf("%s/applications/env/%s/%d", ecore.BuildBaseURLForContext(ctx), ecore.Seg(actorID), envID)
	respBytes, err := ecore.PapiPUT(ctx, apiURL, bodyBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] update env: %v", err)), nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return mcp.NewToolResultText(string(respBytes)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId": actorID,
		"env":     envName,
		"envId":   envID,
		"result":  raw,
	})
	return mcp.NewToolResultText(string(out)), nil
}
