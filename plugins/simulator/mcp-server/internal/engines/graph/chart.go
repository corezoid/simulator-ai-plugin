package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---- Types ----

// ChartConfig holds all parameters for creating a chart dashboard on a layer.
type ChartConfig struct {
	LayerID     string
	Title       string
	Description string
	ChartType   string // "line" | "bar" | "area" — default "line"
	CounterType string // "amount" | "turnover" — default "amount"
	Range       string // "lastHour" | "lastDay" | "lastWeek" | "lastMonth" — default "lastHour"
	PositionX   int
	PositionY   int

	// actorFilter mode — dynamic source via an ActorFilters actor
	FilterActorID string // if set, reuse existing filter (skip creation)
	FilterTitle   string // title for the new filter actor (defaults to Title)
	SourceFormID  int    // numeric formId whose actors are charted
	AccountNameID string // UUID of account name
	CurrencyID    int    // numeric currency ID
	Top           int    // top-N actors shown, default 20

	// direct accounts mode — explicit per-actor series
	Accounts []ChartAccountEntry
}

// ChartAccountEntry is one data series in a direct-accounts chart.
type ChartAccountEntry struct {
	ActorID    string `json:"actorId"`
	CurrencyID int    `json:"currencyId"`
	NameID     string `json:"nameId"`
	Color      string `json:"color,omitempty"`
	IncomeType string `json:"incomeType,omitempty"`
}

// CreateChartResult is returned by CreateChart.
type CreateChartResult struct {
	DashboardActorID string `json:"dashboardActorId"`
	FilterActorID    string `json:"filterActorId,omitempty"`
	LaID             int    `json:"laId"`
}

// ---- System form ID cache for chart forms ----

var (
	chartFormCacheMu sync.RWMutex
	chartFormCache   = map[string]int{} // title → formId
)

// lookupSystemFormID resolves a system form title to its numeric ID.
// Checks the server-wide form name cache first; falls back to a direct API call.
func lookupSystemFormID(ctx context.Context, title, workspaceID, auth, baseURL string) (int, error) {
	// 1. Try the global name→id cache (populated from sys-forms.yaml after sim-init)
	if id := resolveFormNameToID(title); id != 0 {
		return id, nil
	}

	// 2. Try our local chart form cache
	chartFormCacheMu.RLock()
	id := chartFormCache[title]
	chartFormCacheMu.RUnlock()
	if id != 0 {
		return id, nil
	}

	// 3. Fetch from the system forms endpoint
	url := fmt.Sprintf("%s/forms/templates/system/%s?formTypes=system&limit=200&offset=0", baseURL, workspaceID)
	data, err := chartHTTPGet(ctx, url, auth)
	if err != nil {
		// Fall back to all-forms endpoint
		url2 := fmt.Sprintf("%s/forms/templates/%s?formTypes=all&limit=200&offset=0&withDefault=false", baseURL, workspaceID)
		data, err = chartHTTPGet(ctx, url2, auth)
		if err != nil {
			return 0, fmt.Errorf("fetch forms to resolve %q: %w", title, err)
		}
	}

	var resp struct {
		Data []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("parse forms response: %w", err)
	}

	chartFormCacheMu.Lock()
	for _, f := range resp.Data {
		if f.Title != "" && f.ID != 0 {
			chartFormCache[f.Title] = f.ID
		}
	}
	chartFormCacheMu.Unlock()

	chartFormCacheMu.RLock()
	id = chartFormCache[title]
	chartFormCacheMu.RUnlock()
	if id == 0 {
		return 0, fmt.Errorf("form %q not found in workspace %s", title, workspaceID)
	}
	return id, nil
}

// ---- HTTP helpers ----

func chartHTTPGet(ctx context.Context, apiURL, auth string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	resp, err := ecore.APIHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %.300s", apiURL, resp.StatusCode, data)
	}
	return data, nil
}

func chartHTTPJSON(ctx context.Context, method, apiURL, auth string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ecore.APIHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: HTTP %d: %.300s", method, apiURL, resp.StatusCode, data)
	}
	return data, nil
}

// chartJSONString marshals v to a JSON string; panics only on marshal error (never in practice).
func chartJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---- Graph parent lookup ----

// findGraphActorForLayer returns the ID of the Graphs actor that owns the given layer.
// Returns "" if not found — the caller must handle the absence gracefully.
func findGraphActorForLayer(ctx context.Context, layerID, auth, baseURL string) string {
	url := fmt.Sprintf("%s/graph/linked_actors/%s", baseURL, layerID)
	data, err := chartHTTPGet(ctx, url, auth)
	if err != nil {
		return ""
	}
	var resp struct {
		Data []struct {
			ID        string `json:"id"`
			FormTitle string `json:"formTitle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ""
	}
	for _, a := range resp.Data {
		if a.FormTitle == "Graphs" {
			return a.ID
		}
	}
	return ""
}

// ---- Core logic ----

// CreateChart orchestrates the full chart creation flow:
//  1. Create ActorFilters actor (or reuse existing filterActorId).
//  2. Create Dashboards actor with chart config.
//  3. Place dashboard on the layer (returns laId).
//  4. Set account inheritance (best-effort).
//  5. Set expandType=chart on the placement.
func CreateChart(ctx context.Context, cfg ChartConfig, workspaceID, auth, baseURL string) (CreateChartResult, error) {
	var result CreateChartResult

	// Apply defaults
	if cfg.ChartType == "" {
		cfg.ChartType = "line"
	}
	if cfg.CounterType == "" {
		cfg.CounterType = "amount"
	}
	if cfg.Range == "" {
		cfg.Range = "lastHour"
	}
	if cfg.Top <= 0 {
		cfg.Top = 20
	}

	isActorFilterMode := len(cfg.Accounts) == 0

	// ---- Step 1: resolve form IDs ----
	dashboardsFormID, err := lookupSystemFormID(ctx, "Dashboards", workspaceID, auth, baseURL)
	if err != nil {
		return result, fmt.Errorf("resolve Dashboards form: %w", err)
	}

	// ---- Step 2: filter actor (actorFilter mode only) ----
	var filterActorID, filterTitle string

	if isActorFilterMode {
		if cfg.FilterActorID != "" {
			// Reuse existing filter — fetch its data to populate dynamicSource.filter
			filterActorID = cfg.FilterActorID
			filterTitle = cfg.FilterTitle
			if filterTitle == "" {
				// Fetch actor title from API
				actorURL := fmt.Sprintf("%s/actors/%s?attachments=false&meetingParticipants=false&lastTranscription=false&streams=false&webhooks=false&executionState=false&triggers=false", baseURL, filterActorID)
				actorData, err := chartHTTPGet(ctx, actorURL, auth)
				if err == nil {
					var ar struct {
						Data struct {
							Title string                 `json:"title"`
							Data  map[string]interface{} `json:"data"`
						} `json:"data"`
					}
					if json.Unmarshal(actorData, &ar) == nil {
						filterTitle = ar.Data.Title
						// If caller didn't provide filter params, extract from actor data
						if cfg.SourceFormID == 0 {
							if raw, ok := ar.Data.Data["filter"].(string); ok {
								var fd struct {
									FormID        int    `json:"formId"`
									AccountNameID string `json:"accountNameId"`
									CurrencyID    int    `json:"currencyId"`
								}
								if json.Unmarshal([]byte(raw), &fd) == nil {
									cfg.SourceFormID = fd.FormID
									if cfg.AccountNameID == "" {
										cfg.AccountNameID = fd.AccountNameID
									}
									if cfg.CurrencyID == 0 {
										cfg.CurrencyID = fd.CurrencyID
									}
								}
							}
						}
					}
				}
			}
		} else {
			// Create a new ActorFilters actor
			actorFiltersFormID, err := lookupSystemFormID(ctx, "ActorFilters", workspaceID, auth, baseURL)
			if err != nil {
				return result, fmt.Errorf("resolve ActorFilters form: %w", err)
			}

			filterTitle = cfg.FilterTitle
			if filterTitle == "" {
				filterTitle = cfg.Title
			}

			filterJSON := chartJSONString(map[string]interface{}{
				"formId":        cfg.SourceFormID,
				"selectedForms": []int{cfg.SourceFormID},
				"accountNameId": cfg.AccountNameID,
				"currencyId":    cfg.CurrencyID,
			})

			createFilterURL := fmt.Sprintf("%s/actors/actor/%d?contextLayerId=%s", baseURL, actorFiltersFormID, cfg.LayerID)
			filterBody := map[string]interface{}{
				"title": filterTitle,
				"data": map[string]interface{}{
					"filter":                filterJSON,
					"fields":                "[]",
					"defaultFields":         `["title","id","ref","owner","createdAt","updatedAt","balance"]`,
					"formFieldExportFormat": "idAndTitle",
					"cacheFilterResult":     false,
				},
				"hole": false,
			}

			respData, err := chartHTTPJSON(ctx, "POST", createFilterURL, auth, filterBody)
			if err != nil {
				return result, fmt.Errorf("create ActorFilters actor: %w", err)
			}

			var fr struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(respData, &fr); err != nil {
				return result, fmt.Errorf("parse ActorFilters response: %w", err)
			}
			filterActorID = fr.Data.ID
			result.FilterActorID = filterActorID
		}
	}

	// ---- Step 3: build dashboard source config ----
	var sourceConfig map[string]interface{}

	if isActorFilterMode {
		dynamicSource := map[string]interface{}{
			"id":            filterActorID,
			"title":         filterTitle,
			"top":           fmt.Sprintf("%d", cfg.Top),
			"groupByFields": []interface{}{},
		}
		if cfg.SourceFormID != 0 {
			dynamicSource["filter"] = map[string]interface{}{
				"formId":        cfg.SourceFormID,
				"selectedForms": []int{cfg.SourceFormID},
				"accountNameId": cfg.AccountNameID,
				"currencyId":    cfg.CurrencyID,
				"qFormId":       fmt.Sprintf("%d", cfg.SourceFormID),
				"isUat":         false,
			}
		}

		sourceConfig = map[string]interface{}{
			"defaultAccount":         nil,
			"accounts":               []interface{}{},
			"counterType":            cfg.CounterType,
			"chartType":              cfg.ChartType,
			"sourceType":             "actorFilter",
			"range":                  cfg.Range,
			"rangeDates":             map[string]interface{}{"from": nil, "to": nil},
			"showTotal":              true,
			"orderValue":             "default",
			"legend":                 map[string]interface{}{"actorTitle": true, "accountName": true, "currencyName": true},
			"displayChartDataLabels": true,
			"dynamicSource":          dynamicSource,
			"chartViewMode":          "default",
		}
	} else {
		// Direct accounts mode
		palette := []string{
			"#499894", "#59A14F", "#D37295", "#4E79A7", "#FF9D9A",
			"#B07AA1", "#B6992D", "#F1CE63", "#BAB0AC", "#79706E",
		}
		accounts := make([]interface{}, len(cfg.Accounts))
		for i, a := range cfg.Accounts {
			inc := a.IncomeType
			if inc == "" {
				inc = "total"
			}
			col := a.Color
			if col == "" {
				col = palette[i%len(palette)]
			}
			accounts[i] = map[string]interface{}{
				"actorId":    a.ActorID,
				"color":      col,
				"incomeType": inc,
				"currencyId": a.CurrencyID,
				"nameId":     a.NameID,
			}
		}
		sourceConfig = map[string]interface{}{
			"defaultAccount":         nil,
			"accounts":               accounts,
			"counterType":            cfg.CounterType,
			"chartType":              cfg.ChartType,
			"sourceType":             "accounts",
			"range":                  cfg.Range,
			"rangeDates":             map[string]interface{}{"from": nil, "to": nil},
			"showTotal":              true,
			"orderValue":             "default",
			"legend":                 map[string]interface{}{"actorTitle": true, "accountName": true, "currencyName": true},
			"displayChartDataLabels": true,
			"chartViewMode":          "default",
		}
	}

	// ---- Step 4: create Dashboards actor ----
	createDashURL := fmt.Sprintf("%s/actors/actor/%d?contextLayerId=%s", baseURL, dashboardsFormID, cfg.LayerID)
	dashBody := map[string]interface{}{
		"title":       cfg.Title,
		"description": cfg.Description,
		"data": map[string]interface{}{
			"source": chartJSONString(sourceConfig),
		},
		"hole": false,
	}

	respData, err := chartHTTPJSON(ctx, "POST", createDashURL, auth, dashBody)
	if err != nil {
		return result, fmt.Errorf("create Dashboards actor: %w", err)
	}

	var dr struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respData, &dr); err != nil {
		return result, fmt.Errorf("parse Dashboards actor response: %w", err)
	}
	dashboardActorID := dr.Data.ID
	result.DashboardActorID = dashboardActorID

	// ---- Step 5: place dashboard on the layer ----
	addToLayerURL := fmt.Sprintf("%s/graph_layers/actors/%s", baseURL, cfg.LayerID)
	addToLayerBody := []map[string]interface{}{
		{
			"action": "create",
			"data": map[string]interface{}{
				"id":       dashboardActorID,
				"type":     "node",
				"position": map[string]int{"x": cfg.PositionX, "y": cfg.PositionY},
			},
		},
	}

	respData, err = chartHTTPJSON(ctx, "POST", addToLayerURL, auth, addToLayerBody)
	if err != nil {
		return result, fmt.Errorf("place dashboard on layer: %w", err)
	}

	var lr struct {
		Data struct {
			NodesMap []struct {
				LaID int `json:"laId"`
			} `json:"nodesMap"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respData, &lr); err != nil {
		return result, fmt.Errorf("parse add-to-layer response: %w", err)
	}
	var laID int
	if len(lr.Data.NodesMap) > 0 {
		laID = lr.Data.NodesMap[0].LaID
	}
	result.LaID = laID

	// ---- Step 6: set account inheritance (best-effort) ----
	parents := []string{cfg.LayerID}
	if graphID := findGraphActorForLayer(ctx, cfg.LayerID, auth, baseURL); graphID != "" {
		parents = []string{graphID, cfg.LayerID}
	}
	inheritURL := fmt.Sprintf("%s/accounts/inherit/%s", baseURL, workspaceID)
	inheritBody := map[string]interface{}{
		"action":   "create",
		"parents":  parents,
		"children": []string{dashboardActorID},
	}
	if _, inheritErr := chartHTTPJSON(ctx, "POST", inheritURL, auth, inheritBody); inheritErr != nil {
		// Non-fatal: chart may still display if accounts are already accessible
		_ = inheritErr
	}

	// ---- Step 7: mark placement as chart expand type ----
	if laID > 0 {
		expandURL := fmt.Sprintf("%s/graph_layers/actor_settings/%d", baseURL, laID)
		expandBody := map[string]interface{}{
			"layerSettings": map[string]interface{}{
				"expandType": "chart",
				"offset":     map[string]int{"left": 300, "right": 300, "top": 200, "bottom": 200},
				"expand":     true,
			},
		}
		if _, expandErr := chartHTTPJSON(ctx, "PUT", expandURL, auth, expandBody); expandErr != nil {
			_ = expandErr
		}
	}

	return result, nil
}

// ---- MCP handler ----

func handleCreateChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()

	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		return mcp.NewToolResultError("[Error] layerId is required"), nil
	}
	if r := ecore.RequireUUID("layerId", layerID); r != nil {
		return r, nil
	}

	title, _ := args["title"].(string)
	if title == "" {
		return mcp.NewToolResultError("[Error] title is required"), nil
	}

	description, _ := args["description"].(string)
	chartType, _ := args["chartType"].(string)
	counterType, _ := args["counterType"].(string)
	timeRange, _ := args["range"].(string)
	filterActorID, _ := args["filterActorId"].(string)
	filterTitle, _ := args["filterTitle"].(string)
	accountNameID, _ := args["accountNameId"].(string)
	if filterActorID != "" {
		if r := ecore.RequireUUID("filterActorId", filterActorID); r != nil {
			return r, nil
		}
	}
	if accountNameID != "" {
		if r := ecore.RequireUUID("accountNameId", accountNameID); r != nil {
			return r, nil
		}
	}

	var sourceFormID, currencyID, top int
	if v, ok := args["sourceFormId"].(float64); ok {
		sourceFormID = int(v)
	}
	if v, ok := args["currencyId"].(float64); ok {
		currencyID = int(v)
	}
	if v, ok := args["top"].(float64); ok {
		top = int(v)
	}

	var posX, posY int
	if v, ok := args["positionX"].(float64); ok {
		posX = int(v)
	}
	if v, ok := args["positionY"].(float64); ok {
		posY = int(v)
	}

	// Parse direct accounts array
	var accounts []ChartAccountEntry
	if raw, ok := args["accounts"]; ok && raw != nil {
		if arr, ok := raw.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					a := ChartAccountEntry{}
					if v, ok := m["actorId"].(string); ok {
						a.ActorID = v
					}
					if v, ok := m["nameId"].(string); ok {
						a.NameID = v
					}
					if v, ok := m["color"].(string); ok {
						a.Color = v
					}
					if v, ok := m["incomeType"].(string); ok {
						a.IncomeType = v
					}
					if v, ok := m["currencyId"].(float64); ok {
						a.CurrencyID = int(v)
					}
					accounts = append(accounts, a)
				}
			}
		}
	}

	// Validate: actorFilter mode needs either filterActorId or all three filter params
	isActorFilterMode := len(accounts) == 0
	if isActorFilterMode && filterActorID == "" {
		if sourceFormID == 0 || accountNameID == "" || currencyID == 0 {
			return mcp.NewToolResultError(
				"[Error] actorFilter mode requires sourceFormId, accountNameId, and currencyId " +
					"(or provide filterActorId to reuse an existing filter, " +
					"or provide accounts array for direct-accounts mode)",
			), nil
		}
	}

	cfg := ChartConfig{
		LayerID:       layerID,
		Title:         title,
		Description:   description,
		ChartType:     chartType,
		CounterType:   counterType,
		Range:         timeRange,
		PositionX:     posX,
		PositionY:     posY,
		FilterActorID: filterActorID,
		FilterTitle:   filterTitle,
		SourceFormID:  sourceFormID,
		AccountNameID: accountNameID,
		CurrencyID:    currencyID,
		Top:           top,
		Accounts:      accounts,
	}

	result, err := CreateChart(ctx, cfg, os.Getenv("WORKSPACE_ID"), ecore.AuthHeader(), ecore.BuildBaseURL())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] createChart: %v", err)), nil
	}

	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}
