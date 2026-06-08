package smartform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleCreateSmartForm creates a new Smart Form (CDU / Script application) actor
// with two environments (develop + production) via POST /papi/1.0/applications/:accId.
// After creation the caller should run pullSmartForm to download the initial file tree.
func handleCreateSmartForm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()

	title, _ := args["title"].(string)
	if title == "" {
		return mcp.NewToolResultError("[Error] title is required"), nil
	}
	ref, _ := args["ref"].(string)
	if ref == "" {
		return mcp.NewToolResultError("[Error] ref is required"), nil
	}

	accID := os.Getenv("WORKSPACE_ID")
	if accID == "" {
		return mcp.NewToolResultError("[Error] WORKSPACE_ID not set — run set-workspace first"), nil
	}

	sharedWith, _ := args["sharedWith"].(string)
	if sharedWith == "" {
		sharedWith = "userList"
	}

	// Build corezoidCredentials.
	// Accept either a full JSON string or individual flat fields (applied to both envs).
	var creds map[string]interface{}
	if raw, _ := args["corezoidCredentials"].(string); raw != "" {
		if err := json.Unmarshal([]byte(raw), &creds); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] corezoidCredentials is not valid JSON: %v", err)), nil
		}
	} else {
		apiLogin, _ := args["apiLogin"].(string)
		apiSecret, _ := args["apiSecret"].(string)
		procID, _ := args["procId"].(string)
		companyID, _ := args["companyId"].(string)
		envCreds := map[string]interface{}{
			"apiLogin":  apiLogin,
			"apiSecret": apiSecret,
			"procId":    procID,
			"companyId": companyID,
		}
		creds = map[string]interface{}{
			"develop":    envCreds,
			"production": envCreds,
		}
	}

	body := map[string]interface{}{
		"title":               title,
		"ref":                 ref,
		"sharedWith":          sharedWith,
		"corezoidCredentials": creds,
	}
	if desc, _ := args["description"].(string); desc != "" {
		body["description"] = desc
	}
	if pic, _ := args["picture"].(string); pic != "" {
		body["picture"] = pic
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal: %v", err)), nil
	}

	apiURL := fmt.Sprintf("%s/applications/%s", ecore.BuildBaseURL(), ecore.Seg(accID))
	respBytes, err := ecore.PapiPOST(apiURL, bodyBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] create application: %v", err)), nil
	}

	// The server returns { data: { id, ref, title, envs: [{id, title, readonly}] } }.
	var resp struct {
		Data struct {
			ID    string `json:"id"`
			Ref   string `json:"ref"`
			Title string `json:"title"`
			Envs  []struct {
				ID       int    `json:"id"`
				Title    string `json:"title"`
				Readonly bool   `json:"readonly"`
			} `json:"envs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil || resp.Data.ID == "" {
		// Return raw response so the caller can still read the actor id.
		return mcp.NewToolResultText(string(respBytes)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"actorId": resp.Data.ID,
		"ref":     resp.Data.Ref,
		"title":   resp.Data.Title,
		"envs":    resp.Data.Envs,
		"next":    fmt.Sprintf(`run pullSmartForm(actorId="%s") to download the initial file tree`, resp.Data.ID),
	})
	return mcp.NewToolResultText(string(out)), nil
}
