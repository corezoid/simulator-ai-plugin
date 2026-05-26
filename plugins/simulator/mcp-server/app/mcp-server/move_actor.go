package mcpserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// The Simulator Public REST API does not expose a "change formId" mutation
// on an existing actor. The web UI uses a private mw.simulator.company
// endpoint for it (session-cookie auth) that is not reachable via the
// OAuth/`Simulator <token>` flow this plugin uses. Workaround: recreate the
// actor under the target form, replay every link, replay every layer
// placement, delete the old actor.
//
// The result preserves: title, color, picture, description, data, ref, and
// every link / placement. IDs in graphs that referenced the OLD actorId will
// continue to resolve to the new one only if upstream rebuilds — that is
// why we always return both ids so callers can re-stitch external refs.

type apiActorResponse struct {
	Data struct {
		ID          string                 `json:"id"`
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		Picture     string                 `json:"picture"`
		Color       string                 `json:"color"`
		Ref         string                 `json:"ref"`
		FormID      int                    `json:"formId"`
		Data        map[string]interface{} `json:"data"`
	} `json:"data"`
	StatusCode int    `json:"statusCode,omitempty"`
	Message    string `json:"message,omitempty"`
}

type apiLink struct {
	ID         string `json:"id"`
	EdgeTypeID int    `json:"edgeTypeId"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Name       string `json:"name"`
}

type apiLinksResponse struct {
	Data       []apiLink `json:"data"`
	StatusCode int       `json:"statusCode,omitempty"`
	Message    string    `json:"message,omitempty"`
}

type apiPlacement struct {
	LaID     int `json:"laId"`
	Position struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
}

type apiGlobalLayer struct {
	LayerID         string         `json:"layerId"`
	GraphFolderID   string         `json:"graphFolderId"`
	LayerName       string         `json:"layerName"`
	GraphName       string         `json:"graphName"`
	Coordinates     []apiPlacement `json:"coordinates"`
}

type apiGlobalLayersResponse struct {
	Data       []apiGlobalLayer `json:"data"`
	StatusCode int              `json:"statusCode,omitempty"`
	Message    string           `json:"message,omitempty"`
}

// apiDo wraps the boilerplate for an authenticated HTTPS request to the
// Simulator Public API. Returns parsed JSON-friendly bytes on 2xx, otherwise
// an error containing the response body.
func apiDo(ctx context.Context, method, apiURL string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, reader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", globalApiConfig.Authorization)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}
	return respBody, nil
}

func getActorFull(ctx context.Context, actorID string) (apiActorResponse, error) {
	var out apiActorResponse
	b, err := apiDo(ctx, "GET", fmt.Sprintf("%s/actors/actor/%s", buildBaseURL(), actorID), nil)
	if err != nil {
		return out, fmt.Errorf("getActor: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("parse getActor: %w (%.200s)", err, b)
	}
	return out, nil
}

func createActorIn(ctx context.Context, formID int, body map[string]interface{}) (string, error) {
	bodyBytes, _ := json.Marshal(body)
	b, err := apiDo(ctx, "POST",
		fmt.Sprintf("%s/actors/actor/%d", buildBaseURL(), formID), bodyBytes)
	if err != nil {
		return "", fmt.Errorf("createActor: %w", err)
	}
	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return "", fmt.Errorf("parse createActor: %w (%.200s)", err, b)
	}
	if parsed.Data.ID != "" {
		return parsed.Data.ID, nil
	}
	return parsed.ID, nil
}

func deleteActor(ctx context.Context, formID int, actorID string) error {
	_, err := apiDo(ctx, "DELETE",
		fmt.Sprintf("%s/actors/actor/%d/%s", buildBaseURL(), formID, actorID), nil)
	if err != nil {
		return fmt.Errorf("deleteActor: %w", err)
	}
	return nil
}

func getActorLinks(ctx context.Context, actorID string) ([]apiLink, error) {
	b, err := apiDo(ctx, "GET",
		fmt.Sprintf("%s/actors/link/%s", buildBaseURL(), actorID), nil)
	if err != nil {
		return nil, fmt.Errorf("getActorLinks: %w", err)
	}
	var parsed apiLinksResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, fmt.Errorf("parse getActorLinks: %w (%.200s)", err, b)
	}
	return parsed.Data, nil
}

func deleteLink(ctx context.Context, linkID string) error {
	_, err := apiDo(ctx, "DELETE",
		fmt.Sprintf("%s/actors/link/%s", buildBaseURL(), linkID), nil)
	if err != nil {
		return fmt.Errorf("deleteLink: %w", err)
	}
	return nil
}

func createLink(ctx context.Context, source, target, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"source": source, "target": target, "name": name,
	})
	b, err := apiDo(ctx, "POST",
		fmt.Sprintf("%s/actors/link", buildBaseURL()), body)
	if err != nil {
		return "", fmt.Errorf("createLink: %w", err)
	}
	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return "", fmt.Errorf("parse createLink: %w (%.200s)", err, b)
	}
	return parsed.Data.ID, nil
}

func actorGlobalLayers(ctx context.Context, actorID string) ([]apiGlobalLayer, error) {
	b, err := apiDo(ctx, "GET",
		fmt.Sprintf("%s/actors/actor/%s/layers", buildBaseURL(), actorID), nil)
	if err != nil {
		return nil, fmt.Errorf("actorGlobalLayers: %w", err)
	}
	var parsed apiGlobalLayersResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, fmt.Errorf("parse layers: %w (%.200s)", err, b)
	}
	return parsed.Data, nil
}

// manageLayerCall issues POST /graph_layers/{layerId}/actors with a list of
// action items. Reuses the manageLayerItem type already declared in
// sync_graph.go.
func manageLayerCall(ctx context.Context, layerID string, items []manageLayerItem) error {
	body, _ := json.Marshal(map[string]interface{}{"items": items})
	_, err := apiDo(ctx, "POST",
		fmt.Sprintf("%s/graph_layers/%s/actors", buildBaseURL(),
			url.PathEscape(layerID)), body)
	if err != nil {
		return fmt.Errorf("manageLayer: %w", err)
	}
	return nil
}

// newPlacement builds a node-create/delete manageLayerItem with sane
// defaults — the inner Data struct uses typed fields, not a plain map.
func newPlacement(action, actorID string, laID, x, y int) manageLayerItem {
	var it manageLayerItem
	it.Action = action
	it.Data.Type = "node"
	it.Data.ID = actorID
	it.Data.LaID = laID
	it.Data.Position.X = x
	it.Data.Position.Y = y
	return it
}

// MoveActorResult describes what was done during a moveActorToForm call.
type MoveActorResult struct {
	OldActorID         string `json:"oldActorId"`
	NewActorID         string `json:"newActorId"`
	LinksRewired       int    `json:"linksRewired"`
	LinksFailed        int    `json:"linksFailed"`
	PlacementsRewired  int    `json:"placementsRewired"`
	PlacementsFailed   int    `json:"placementsFailed"`
	OldDeleted         bool   `json:"oldDeleted"`
	Warning            string `json:"warning,omitempty"`
}

// handleMoveActorToForm recreates an actor under a different formId while
// preserving title/color/picture/description/data/ref, all incoming and
// outgoing links, and every layer placement (laId/position) the actor had.
// On any non-fatal step (a single link or placement failing to replay) the
// tool continues and reports counts in the result rather than aborting —
// the caller can re-run to mop up.
func handleMoveActorToForm(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()
	actorID, _ := args["actorId"].(string)
	if actorID == "" {
		return mcp.NewToolResultError("[Error] actorId is required"), nil
	}
	targetFormID := toInt(args["targetFormId"])
	if targetFormID == 0 {
		return mcp.NewToolResultError("[Error] targetFormId is required"), nil
	}
	keepOld, _ := args["keepOld"].(bool)

	// 1) Snapshot the original actor.
	src, err := getActorFull(ctx, actorID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %v", err)), nil
	}
	if src.Data.FormID == targetFormID {
		return mcp.NewToolResultError(
			"[Error] source actor already lives on the target form"), nil
	}
	sourceFormID := src.Data.FormID

	// 2) Snapshot links + placements before mutation.
	links, lerr := getActorLinks(ctx, actorID)
	if lerr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %v", lerr)), nil
	}
	layers, gerr := actorGlobalLayers(ctx, actorID)
	if gerr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %v", gerr)), nil
	}

	// 3) Create the replacement actor.
	newBody := map[string]interface{}{
		"title":       src.Data.Title,
		"description": src.Data.Description,
		"picture":     src.Data.Picture,
		"color":       src.Data.Color,
		"data":        src.Data.Data,
	}
	if src.Data.Ref != "" {
		// Refs are unique within a form; only carry forward when moving between
		// different forms (target form is guaranteed different here).
		newBody["ref"] = src.Data.Ref
	}
	newID, err := createActorIn(ctx, targetFormID, newBody)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] createActor: %v", err)), nil
	}

	result := MoveActorResult{
		OldActorID: actorID,
		NewActorID: newID,
	}

	// 4) Replay links — delete the original, recreate against the new actorId.
	for _, l := range links {
		newSrc, newTgt := l.Source, l.Target
		if newSrc == actorID {
			newSrc = newID
		}
		if newTgt == actorID {
			newTgt = newID
		}
		// Skip self-loops introduced by the rewrite.
		if newSrc == newTgt {
			continue
		}
		if err := deleteLink(ctx, l.ID); err != nil {
			result.LinksFailed++
			continue
		}
		if _, err := createLink(ctx, newSrc, newTgt, l.Name); err != nil {
			result.LinksFailed++
			continue
		}
		result.LinksRewired++
	}

	// 5) Replay placements on every layer the old actor was pinned to.
	for _, layer := range layers {
		for _, p := range layer.Coordinates {
			delItem := newPlacement("delete", actorID, p.LaID, p.Position.X, p.Position.Y)
			if err := manageLayerCall(ctx, layer.LayerID, []manageLayerItem{delItem}); err != nil {
				result.PlacementsFailed++
				continue
			}
			createItem := newPlacement("create", newID, 0, p.Position.X, p.Position.Y)
			if err := manageLayerCall(ctx, layer.LayerID, []manageLayerItem{createItem}); err != nil {
				result.PlacementsFailed++
				continue
			}
			result.PlacementsRewired++
		}
	}

	// 6) Delete the original unless caller asked to keep it.
	if !keepOld {
		if err := deleteActor(ctx, sourceFormID, actorID); err != nil {
			result.Warning = fmt.Sprintf("old actor not deleted: %v (you may want to delete %s manually)", err, actorID)
		} else {
			result.OldDeleted = true
		}
	}

	// 7) Serialise result.
	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}
