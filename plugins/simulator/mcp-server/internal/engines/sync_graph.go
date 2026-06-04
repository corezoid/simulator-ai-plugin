package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// ---- YAML data model ----

type GraphFile struct {
	LayerID string       `yaml:"layerId"`
	Actors  []GraphActor `yaml:"actors"`
	Edges   []GraphEdge  `yaml:"edges"`
}

type GraphActor struct {
	ID          string                 `yaml:"id"`
	Action      string                 `yaml:"action,omitempty"`
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description,omitempty"`
	FormID      int                    `yaml:"formId,omitempty"`
	FormName    string                 `yaml:"formName,omitempty"`
	Picture     string                 `yaml:"picture,omitempty"`
	Color       string                 `yaml:"color,omitempty"`
	Data        map[string]interface{} `yaml:"data,omitempty"`
	Position    struct {
		X int `yaml:"x"`
		Y int `yaml:"y"`
	} `yaml:"position"`
}

type GraphEdge struct {
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	Action      string `yaml:"action,omitempty"`
	SourceTitle string `yaml:"sourceTitle,omitempty"`
	TargetTitle string `yaml:"targetTitle,omitempty"`
}

// ---- Server response types ----

type layerActor struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Color       string                 `json:"color"`
	FormID      int                    `json:"formId"`
	Picture     string                 `json:"picture"`
	LaID        int                    `json:"laId"`
	Data        map[string]interface{} `json:"data"`
	Position    struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
}

// formIDFromLayerActor returns the formId to use for API calls.
// Prefers the id found in data keys like "__form__408962:view" over the top-level formId.
func formIDFromLayerActor(sa layerActor) int {
	for key := range sa.Data {
		if strings.HasPrefix(key, "__form__") {
			rest := strings.TrimPrefix(key, "__form__")
			if idx := strings.Index(rest, ":"); idx > 0 {
				if id, err := strconv.Atoi(rest[:idx]); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	return sa.FormID
}

type layerEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	LaID       int    `json:"laId"`
	LaIDSource int    `json:"laIdSource"`
	LaIDTarget int    `json:"laIdTarget"`
}

// ---- Helpers ----

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(s string) bool {
	return uuidRe.MatchString(s)
}

// requireUUID returns an MCP error result when v is not a well-formed UUID, or
// nil when it is. It guards tool arguments that are interpolated into file
// paths (pull/pushGraphFile write <layerId>.yaml) and API URLs — without it a
// value like "../../etc/x" or "id?admin=1" would traverse the filesystem or
// inject into the request (see security review).
func requireUUID(name, v string) *mcp.CallToolResult {
	if !isUUID(v) {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %s must be a valid UUID, got %q", name, v))
	}
	return nil
}

// seg escapes a value for safe interpolation as a single URL path segment.
// IDs that originate from a graph file (e.g. graph.LayerID, actor UUIDs) are
// not boundary-validated, so escaping here prevents path/query injection if
// such an ID contains "/", "?" or "#".
func seg(s string) string { return url.PathEscape(s) }

// buildBaseURL returns the same base URL used by all other MCP tools.
func buildBaseURL() string {
	switch {
	case Cfg.Url != "":
		return strings.TrimSuffix(Cfg.Url, "/")
	case Cfg.BaseUrl != "":
		return strings.TrimSuffix(Cfg.BaseUrl, "/")
	default:
		return "https://api.simulator.company/v/1.0"
	}
}

func papiGET(apiURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", Cfg.Authorization)
	resp, err := apiHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Check status AFTER reading: a 401/500 returns an error body that is not a
	// {"data":[...]} payload; without this guard the caller would silently parse
	// it as an empty result (e.g. an empty layer export). Matches GraphSyncer.get.
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

func fetchLayerActors(layerID string) ([]layerActor, error) {
	base := buildBaseURL()
	var all []layerActor
	limit, offset := 50, 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=nodes&limit=%d&offset=%d", base, seg(layerID), limit, offset)
		body, err := papiGET(u)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []layerActor `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parse layer actors: %w (body: %.200s)", err, body)
		}
		all = append(all, page.Data...)
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}
	return all, nil
}

func fetchLayerEdges(layerID string) ([]layerEdge, error) {
	base := buildBaseURL()
	var all []layerEdge
	limit, offset := 50, 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=edges&limit=%d&offset=%d", base, seg(layerID), limit, offset)
		body, err := papiGET(u)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []layerEdge `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parse layer edges: %w (body: %.200s)", err, body)
		}
		all = append(all, page.Data...)
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}
	return all, nil
}

// manageLayerItem is a single create/delete action for the manageLayer API.
type manageLayerItem struct {
	Action string `json:"action"`
	Data   struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		LaID     int    `json:"laId,omitempty"`
		LaIDSrc  int    `json:"laIdSource,omitempty"`
		LaIDTgt  int    `json:"laIdTarget,omitempty"`
		Position struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"position"`
	} `json:"data"`
}

// ---- Main handlers ----

func handlePushGraphFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	filePath := layerID + ".yaml"

	rawData, err := os.ReadFile(filePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] cannot read file %s: %v", filePath, err)), nil
	}

	var graph GraphFile
	if parseErr := yaml.Unmarshal(rawData, &graph); parseErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] cannot parse YAML %s: %v", filePath, parseErr)), nil
	}

	result, syncErr := PushGraphFile(graph, os.Getenv("WORKSPACE_ID"), layerID, Cfg.Authorization, buildBaseURL())
	if syncErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %v", syncErr)), nil
	}

	updatedYAML, marshalErr := yaml.Marshal(&result.UpdatedGraph)
	if marshalErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal YAML: %v", marshalErr)), nil
	}
	if writeErr := os.WriteFile(filePath, updatedYAML, 0600); writeErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] write YAML: %v", writeErr)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"layerId": result.LayerID,
		"actors": map[string]int{
			"created":   result.ActorsCreated,
			"updated":   result.ActorsUpdated,
			"unchanged": result.ActorsUnchanged,
			"deleted":   result.ActorsDeleted,
			"recreated": result.ActorsRecreated,
		},
		"edges": map[string]int{
			"created": result.EdgesCreated,
			"deleted": result.EdgesDeleted,
		},
		"fileUpdated": true,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// handlePullGraphFile fetches all actors and edges from a layer and writes
// them to <layerId>.yaml in the current working directory.
func handlePullGraphFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	filePath := layerID + ".yaml"

	// Fetch actors
	serverActors, err := fetchLayerActors(layerID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch layer actors: %v", err)), nil
	}

	// Fetch edges
	serverEdges, err := fetchLayerEdges(layerID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch layer edges: %v", err)), nil
	}

	// Build actor title lookup for edge sourceTitle / targetTitle
	titleByUUID := make(map[string]string, len(serverActors))
	for _, a := range serverActors {
		titleByUUID[a.ID] = a.Title
	}

	// Build GraphFile
	graph := GraphFile{LayerID: layerID}

	// Pre-load sys forms so resolveFormIDToName works (best-effort, non-fatal).
	if _, err := loadSysForms(); err != nil {
		log.Printf("Warning: exportGraph loadSysForms: %v", err)
	}

	for _, sa := range serverActors {
		var ga GraphActor
		ga.ID = sa.ID
		ga.Title = sa.Title
		ga.Description = sa.Description
		ga.Color = sa.Color
		ga.Picture = sa.Picture
		ga.FormID = formIDFromLayerActor(sa)
		if name := resolveFormIDToName(ga.FormID); name != "" {
			ga.FormName = name
		}
		ga.Position.X = sa.Position.X
		ga.Position.Y = sa.Position.Y
		graph.Actors = append(graph.Actors, ga)
	}

	for _, se := range serverEdges {
		graph.Edges = append(graph.Edges, GraphEdge{
			Source:      se.Source,
			Target:      se.Target,
			SourceTitle: titleByUUID[se.Source],
			TargetTitle: titleByUUID[se.Target],
		})
	}

	data, err := yaml.Marshal(&graph)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] marshal YAML: %v", err)), nil
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] write file: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"layerId":  layerID,
		"filePath": filePath,
		"actors":   len(graph.Actors),
		"edges":    len(graph.Edges),
	})
	return mcp.NewToolResultText(string(out)), nil
}
