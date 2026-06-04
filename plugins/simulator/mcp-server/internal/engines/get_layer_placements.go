package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
)

// getAllLayerPlacements walks the paginated /graph_layers/paginated/{layerId}
// endpoint and returns every (actorId, laId, formId, title, x, y) row for the
// layer in a single MCP call. The existing `getLayerActorsByFormId` forces the
// caller to enumerate every formId in use (often 15+ forms on a busy layer),
// which is slow and easy to get wrong — this tool is one round-trip with all
// the data needed for layout / dedup / bulk-position scripting.

type layerPlacementRow struct {
	ActorID  string `json:"actorId"`
	LaID     int    `json:"laId"`
	FormID   int    `json:"formId"`
	Title    string `json:"title"`
	Position struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
}

type paginatedLayerActor struct {
	ID       string `json:"id"`
	LaID     int    `json:"laId"`
	FormID   int    `json:"formId"`
	Title    string `json:"title"`
	Position struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
	// formId can also live under nested form objects on some endpoints; the
	// paginated route exposes it at the top level, so this struct is enough.
}

func handleGetAllLayerPlacements(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		return mcp.NewToolResultError("[Error] layerId is required"), nil
	}
	if r := requireUUID("layerId", layerID); r != nil {
		return r, nil
	}

	client := apiHTTPClient()

	var rows []layerPlacementRow
	const limit = 100
	offset := 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=nodes&limit=%d&offset=%d",
			buildBaseURL(), layerID, limit, offset)
		httpReq, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] new request: %v", err)), nil
		}
		httpReq.Header.Set("Authorization", Cfg.Authorization)
		resp, err := client.Do(httpReq)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] http: %v", err)), nil
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return mcp.NewToolResultError(
				fmt.Sprintf("[Error] HTTP %d: %.200s", resp.StatusCode, body)), nil
		}
		var page struct {
			Data []paginatedLayerActor `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("[Error] parse: %v (body: %.200s)", err, body)), nil
		}
		for _, p := range page.Data {
			row := layerPlacementRow{
				ActorID: p.ID,
				LaID:    p.LaID,
				FormID:  p.FormID,
				Title:   p.Title,
			}
			row.Position.X = p.Position.X
			row.Position.Y = p.Position.Y
			rows = append(rows, row)
		}
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}

	// Workspace id only embedded for callers that want to chain into other
	// endpoints — not required for downstream layerActorsPosition / manageLayer.
	out := map[string]interface{}{
		"layerId":     layerID,
		"workspaceId": os.Getenv("WORKSPACE_ID"),
		"total":       len(rows),
		"placements":  rows,
	}
	resultBytes, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(resultBytes)), nil
}
