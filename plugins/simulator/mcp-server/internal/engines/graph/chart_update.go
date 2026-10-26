package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// UpdateChartConfig reconfigures an existing Dashboards (chart) actor in place.
// A zero value (""/0/nil) keeps what the chart has now.
type UpdateChartConfig struct {
	DashboardActorID string

	ChartType   string
	CounterType string
	Range       string
	OrderValue  string
	// IncomeType (direct-accounts mode) is applied to every series, or — with
	// Accounts — to each new series that names none.
	IncomeType string

	// actorFilter mode
	Top           int
	FilterActorID string // repoint the chart at this existing ActorFilters actor
	FilterTitle   string // title of a new ActorFilters actor
	SourceFormID  int    // any of these three builds a new ActorFilters actor
	AccountNameID string
	CurrencyID    int

	// Accounts replaces every series and switches the chart to direct-accounts mode.
	Accounts []ChartAccountEntry

	// Placement: LaID (on LayerID) is re-placed with chart layerSettings.
	LayerID string
	LaID    int
	Expand  *bool // default true
	Width   int   // px, with Height; 0 keeps the current size
	Height  int
}

// UpdateChartResult is returned by UpdateChart.
type UpdateChartResult struct {
	DashboardActorID string `json:"dashboardActorId"`
	SourceType       string `json:"sourceType"`
	ChartType        string `json:"chartType"`
	CounterType      string `json:"counterType"`
	Range            string `json:"range"`
	OrderValue       string `json:"orderValue"`
	FilterActorID    string `json:"filterActorId,omitempty"`
	// LaID is the chart's placement after a re-placement; PreviousLaID the one it replaced.
	LaID         int      `json:"laId,omitempty"`
	PreviousLaID int      `json:"previousLaId,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// The UI's chart size floor (pong-front-end EXP_NODE.size.chart: 13×9 cells of
// GRAPH_NODE_SIZE 50px); its offset is the size minus one cell, split evenly —
// the 650×450 default is createChart's 300/300/200/200.
const (
	chartCellSize  = 50
	chartMinWidth  = 650
	chartMinHeight = 450
)

// dashboardDisplayGroupBy is the UI's DASHBOARD_DISPLAY_MODE.GROUP_BY: the
// chart groups the filtered actors by groupByFields instead of showing the top N.
const dashboardDisplayGroupBy = "groupBy"

func (c UpdateChartConfig) changesSource() bool {
	return c.ChartType != "" || c.CounterType != "" || c.Range != "" || c.OrderValue != "" ||
		c.IncomeType != "" || c.Top > 0 || c.FilterActorID != "" || c.FilterTitle != "" ||
		c.SourceFormID != 0 || c.AccountNameID != "" || c.CurrencyID != 0 || len(c.Accounts) > 0
}

func (c UpdateChartConfig) changesFilter() bool {
	return c.SourceFormID != 0 || c.AccountNameID != "" || c.CurrencyID != 0
}

func (c UpdateChartConfig) validate() error {
	switch {
	case !ecore.IsUUID(c.DashboardActorID):
		return fmt.Errorf("dashboardActorId must be the chart actor's UUID, got %q", c.DashboardActorID)
	case c.Top < 0 || c.SourceFormID < 0 || c.CurrencyID < 0 || c.LaID < 0:
		return errors.New("top, sourceFormId, currencyId and laId must be positive (0 or omitted keeps the current value)")
	case !c.changesSource() && c.LaID == 0:
		return errors.New("nothing to change — pass a chart setting (chartType, range, counterType, orderValue, " +
			"incomeType, top, the filter or accounts) and/or laId for the placement")
	case len(c.Accounts) > 0 && (c.FilterActorID != "" || c.changesFilter() || c.Top > 0 || c.FilterTitle != ""):
		return errors.New("accounts (direct-accounts mode) cannot be combined with the actorFilter params " +
			"filterActorId, filterTitle, sourceFormId, accountNameId, currencyId or top")
	case c.FilterActorID != "" && !ecore.IsUUID(c.FilterActorID):
		return fmt.Errorf("filterActorId must be an ActorFilters actor UUID, got %q", c.FilterActorID)
	case c.AccountNameID != "" && !ecore.IsUUID(c.AccountNameID):
		return fmt.Errorf("accountNameId must be the account name UUID, got %q", c.AccountNameID)
	case c.LaID == 0 && (c.LayerID != "" || c.Expand != nil || c.Width != 0 || c.Height != 0):
		return errors.New("layerId, expand, width and height change the chart's placement — pass its laId too")
	case c.LaID != 0 && !ecore.IsUUID(c.LayerID):
		return fmt.Errorf("laId needs layerId, the UUID of the layer it is on (got %q)", c.LayerID)
	case (c.Width == 0) != (c.Height == 0):
		return errors.New("pass width and height together")
	case c.Width != 0 && (c.Width < chartMinWidth || c.Height < chartMinHeight):
		return fmt.Errorf("width×height %d×%d is below the chart minimum %d×%d", c.Width, c.Height, chartMinWidth, chartMinHeight)
	}
	return nil
}

// dashboardActor is the part of a Dashboards actor updateChart reads.
type dashboardActor struct {
	FormID int    `json:"formId"`
	Title  string `json:"title"`
	Data   struct {
		Source json.RawMessage `json:"source"`
	} `json:"data"`
}

// sourceView is the part of data.source updateChart reads. It is deliberately
// looser than dashboardSource (what createChart writes): the UI wizard,
// createChart and older plugin versions store the same keys with different
// JSON types (top "20" or 20, range null), and every key not listed here is
// kept verbatim in the raw map that gets written back.
type sourceView struct {
	SourceType    string             `json:"sourceType"`
	ChartType     string             `json:"chartType"`
	CounterType   string             `json:"counterType"`
	Range         string             `json:"range"`
	OrderValue    string             `json:"orderValue"`
	Accounts      []accountView      `json:"accounts"`
	DynamicSource *dynamicSourceView `json:"dynamicSource"`
}

type accountView struct {
	ActorID    string  `json:"actorId"`
	NameID     string  `json:"nameId"`
	CurrencyID flexInt `json:"currencyId"`
	Color      string  `json:"color"`
	IncomeType string  `json:"incomeType"`
}

type dynamicSourceView struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Top         flexInt `json:"top"`
	DisplayMode string  `json:"displayMode"`
	Filter      *struct {
		FormID        flexInt `json:"formId"`
		AccountNameID string  `json:"accountNameId"`
		CurrencyID    flexInt `json:"currencyId"`
	} `json:"filter"`
}

// parseDashboardSource reads data.source, stored as a JSON string by the UI and
// createChart (a hand-written object is read too). The view is for reading;
// the raw map is what gets written back, so keys this plugin does not model
// (the UI owns them) survive the update.
func parseDashboardSource(raw json.RawMessage) (sourceView, map[string]json.RawMessage, error) {
	var src sourceView
	var s string
	if json.Unmarshal(raw, &s) == nil {
		raw = json.RawMessage(s)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil || len(keys) == 0 {
		return src, nil, errors.New("the actor has no chart config (data.source) — create the chart with createChart instead")
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		return src, nil, fmt.Errorf("parse data.source: %w", err)
	}
	return src, keys, nil
}

// setJSON stores v under key, marshalled.
func setJSON(m map[string]json.RawMessage, key string, v any) {
	b, _ := json.Marshal(v)
	m[key] = b
}

// rawObject decodes a JSON object into a key map; nil or a non-object gives an
// empty map.
func rawObject(raw json.RawMessage) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m
}

// sourcePlan is the computed new data.source plus what has to be written first.
type sourcePlan struct {
	keys          map[string]json.RawMessage
	newFilter     bool // create an ActorFilters actor first and point the source at it
	filterTitle   string
	filter        filterActorFilter
	top           int
	prevFilterID  string
	filterActorID string // set when the chart keeps or is repointed to an existing filter
	// grouping holds the displayMode/groupByFields a rebuilt dynamicSource
	// carries over from a "group by" chart (nil: show the top N).
	grouping   map[string]json.RawMessage
	sourceType string
	cfg        ChartConfig
}

// dynamicSource builds the dynamicSource for filter id, with the grouping
// carried over when there is one.
func (p sourcePlan) dynamicSource(id, title string, f filterActorFilter) map[string]json.RawMessage {
	b, _ := json.Marshal(newDynamicSource(id, title, p.top, f.FormID, f.AccountNameID, f.CurrencyID))
	ds := rawObject(b)
	if p.grouping != nil {
		for k, v := range p.grouping {
			ds[k] = v
		}
		delete(ds, "top") // the UI keeps no top in group-by mode
	}
	return ds
}

// planSource merges cfg onto the chart's current source and validates the
// result with createChart's normalizeChartConfig. It makes no write.
func planSource(ctx context.Context, cfg UpdateChartConfig, actor dashboardActor, auth, baseURL string) (sourcePlan, []string, error) {
	var p sourcePlan
	cur, keys, err := parseDashboardSource(actor.Data.Source)
	if err != nil {
		return p, nil, err
	}
	p.keys = keys

	// Values createChart cannot build are kept only if the caller replaces them.
	if cur.ChartType == "kpiCard" && cfg.ChartType == "" {
		return p, nil, errors.New("this is a kpiCard chart, which updateChart does not build — pass another chartType or edit it in the UI")
	}
	if cur.Range == "custom" && cfg.Range == "" {
		return p, nil, errors.New("this chart uses a custom date range, which updateChart does not build — pass another range or edit it in the UI")
	}

	// Mode: accounts → direct accounts; a filter param → actorFilter; else as now.
	// createChart wrote sourceType "accounts" before v2.12.0 — that is "manual".
	curFilterMode := cur.SourceType == dashboardSourceActorFilter ||
		(cur.SourceType != dashboardSourceManual && cur.SourceType != "accounts" && cur.DynamicSource != nil)
	curDS := cur.DynamicSource
	if !curFilterMode {
		curDS = nil
	}
	filterMode := curFilterMode
	switch {
	case len(cfg.Accounts) > 0:
		filterMode = false
	case cfg.FilterActorID != "" || cfg.changesFilter():
		filterMode = true
	case !curFilterMode && (cfg.Top > 0 || cfg.FilterTitle != ""):
		return p, nil, errors.New("top and filterTitle apply to actorFilter charts — this chart lists its accounts directly")
	}
	if filterMode && cfg.IncomeType != "" {
		return p, nil, errors.New("incomeType applies to direct-accounts charts — an actorFilter chart has no per-series incomeType")
	}

	c := ChartConfig{
		ChartType:   firstNonEmpty(cfg.ChartType, cur.ChartType),
		CounterType: firstNonEmpty(cfg.CounterType, cur.CounterType),
		Range:       firstNonEmpty(cfg.Range, cur.Range),
		OrderValue:  firstNonEmpty(cfg.OrderValue, cur.OrderValue),
	}
	// A key the chart has no value for gets normalizeChartConfig's default; say so.
	var defaulted []string
	for name, v := range map[string]string{"chartType": c.ChartType, "counterType": c.CounterType, "range": c.Range, "orderValue": c.OrderValue} {
		if v == "" {
			defaulted = append(defaulted, name)
		}
	}
	slices.Sort(defaulted)
	if !filterMode {
		if len(cfg.Accounts) > 0 {
			c.Accounts = slices.Clone(cfg.Accounts)
			for i := range c.Accounts {
				if c.Accounts[i].IncomeType == "" {
					c.Accounts[i].IncomeType = cfg.IncomeType
				}
			}
		} else {
			if len(cur.Accounts) == 0 {
				return p, nil, errors.New("this chart has no series — pass accounts, or switch it to actorFilter mode with filterActorId or sourceFormId+accountNameId+currencyId")
			}
			for _, a := range cur.Accounts {
				e := ChartAccountEntry{ActorID: a.ActorID, CurrencyID: int(a.CurrencyID), NameID: a.NameID, Color: a.Color, IncomeType: a.IncomeType}
				if cfg.IncomeType != "" {
					e.IncomeType = cfg.IncomeType
				}
				c.Accounts = append(c.Accounts, e)
			}
		}
	} else {
		c.Top = cfg.Top
		if c.Top == 0 && curDS != nil {
			c.Top = int(curDS.Top)
		}
	}
	warnings, err := normalizeChartConfig(&c)
	if err != nil {
		return p, nil, err
	}
	final := map[string]string{"chartType": c.ChartType, "counterType": c.CounterType, "range": c.Range, "orderValue": c.OrderValue}
	for _, name := range defaulted {
		warnings = append(warnings, fmt.Sprintf("the chart had no %s — stored the default %s", name, final[name]))
	}
	p.cfg = c
	p.top = c.Top
	if curDS != nil {
		p.prevFilterID = curDS.ID
	}

	setJSON(keys, "chartType", c.ChartType)
	setJSON(keys, "counterType", c.CounterType)
	setJSON(keys, "range", c.Range)
	setJSON(keys, "orderValue", c.OrderValue)
	if cur.Range == "custom" {
		// What the UI stores for every range but custom (buildDashboardSource).
		setJSON(keys, "rangeDates", dashboardRangeDates{})
	}
	legend := rawObject(keys["legend"])
	if len(legend) == 0 {
		setJSON(legend, "actorTitle", true)
		setJSON(legend, "accountName", true)
		setJSON(legend, "currencyName", true)
	}

	if !filterMode {
		p.sourceType = dashboardSourceManual
		setJSON(keys, "sourceType", dashboardSourceManual)
		if len(cfg.Accounts) > 0 {
			setJSON(keys, "accounts", dashboardAccounts(c.Accounts))
		} else {
			// Kept series are patched, not rebuilt: the UI stores more per series
			// (accountType plan/fact, customName, showTrendLine, account, actor).
			var series []map[string]json.RawMessage
			if json.Unmarshal(keys["accounts"], &series) != nil || len(series) != len(c.Accounts) {
				return p, nil, errors.New("parse data.source accounts: not a list of series objects")
			}
			for i := range series {
				if series[i] == nil {
					series[i] = map[string]json.RawMessage{}
				}
				setJSON(series[i], "incomeType", c.Accounts[i].IncomeType)
				if cur.Accounts[i].Color == "" {
					setJSON(series[i], "color", chartSeriesPalette[i%len(chartSeriesPalette)])
				}
			}
			setJSON(keys, "accounts", series)
		}
		delete(keys, "dynamicSource")
		if legendNeedsIncomeType(c.Accounts) {
			setJSON(legend, "incomeType", true)
		} else {
			delete(legend, "incomeType")
		}
		setJSON(keys, "legend", legend)
		return p, warnings, nil
	}

	p.sourceType = dashboardSourceActorFilter
	setJSON(keys, "sourceType", dashboardSourceActorFilter)
	setJSON(keys, "accounts", []dashboardAccount{})
	setJSON(keys, "legend", legend)
	var curFilter filterActorFilter
	if curDS != nil && curDS.Filter != nil {
		f := curDS.Filter
		curFilter = filterActorFilter{FormID: int(f.FormID), AccountNameID: f.AccountNameID, CurrencyID: int(f.CurrencyID)}
	}
	groupBy := curDS != nil && curDS.DisplayMode == dashboardDisplayGroupBy
	// keepGrouping decides whether a rebuilt dynamicSource stays "group by":
	// groupByFields are fields of the source form, so only while it is unchanged.
	keepGrouping := func(formID int) error {
		if !groupBy {
			return nil
		}
		if formID == 0 || formID != curFilter.FormID {
			warnings = append(warnings, fmt.Sprintf("the chart grouped actors by fields of form %d; form %d has other fields, "+
				"so it now shows the top %d actors — set the grouping again in the UI", curFilter.FormID, formID, c.Top))
			return nil
		}
		if cfg.Top > 0 {
			return errors.New("this chart groups its actors by fields (display mode \"group by\"), so it has no top — switch it to top-N in the UI first")
		}
		ds := rawObject(keys["dynamicSource"])
		p.grouping = map[string]json.RawMessage{"displayMode": ds["displayMode"], "groupByFields": ds["groupByFields"]}
		return nil
	}
	switch {
	case cfg.FilterActorID != "":
		// Repoint at an existing filter; explicit params override what it holds.
		fa, err := readFilterActor(ctx, cfg.FilterActorID, auth, baseURL)
		if err != nil {
			return p, nil, fmt.Errorf("read ActorFilters actor %s: %w", cfg.FilterActorID, err)
		}
		f := filterActorFilter{}
		if fa.Filter != nil {
			f = *fa.Filter
		}
		f = overrideFilter(f, cfg)
		if err := keepGrouping(f.FormID); err != nil {
			return p, nil, err
		}
		p.filterActorID = cfg.FilterActorID
		setJSON(keys, "dynamicSource", p.dynamicSource(cfg.FilterActorID, firstNonEmpty(cfg.FilterTitle, fa.Title), f))
	case cfg.changesFilter():
		// A filter actor can back several charts (createChart's filterActorId), so
		// a new filter gets a new actor instead of editing the shared one.
		f := overrideFilter(curFilter, cfg)
		if f.FormID == 0 || f.AccountNameID == "" || f.CurrencyID == 0 {
			return p, nil, errors.New("a new actorFilter source needs sourceFormId, accountNameId and currencyId " +
				"(the ones not passed are taken from the chart's current filter) — or pass filterActorId to reuse a filter")
		}
		if err := keepGrouping(f.FormID); err != nil {
			return p, nil, err
		}
		p.newFilter = true
		p.filter = f
		title := cfg.FilterTitle
		if title == "" && curDS != nil {
			title = curDS.Title
		}
		p.filterTitle = firstNonEmpty(title, actor.Title)
	default:
		// Same filter: patch top (and the title) in place so the UI's own keys survive.
		if curDS == nil {
			return p, nil, errors.New("no filter to keep — pass filterActorId, or sourceFormId+accountNameId+currencyId")
		}
		ds := rawObject(keys["dynamicSource"])
		if groupBy {
			if cfg.Top > 0 {
				return p, nil, errors.New("this chart groups its actors by fields (display mode \"group by\"), so it has no top — switch it to top-N in the UI first")
			}
		} else {
			setJSON(ds, "top", strconv.Itoa(c.Top))
		}
		if cfg.FilterTitle != "" {
			setJSON(ds, "title", cfg.FilterTitle)
		}
		setJSON(keys, "dynamicSource", ds)
		p.filterActorID = curDS.ID
	}
	return p, warnings, nil
}

func overrideFilter(f filterActorFilter, cfg UpdateChartConfig) filterActorFilter {
	if cfg.SourceFormID != 0 {
		f.FormID = cfg.SourceFormID
	}
	if cfg.AccountNameID != "" {
		f.AccountNameID = cfg.AccountNameID
	}
	if cfg.CurrencyID != 0 {
		f.CurrencyID = cfg.CurrencyID
	}
	return f
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// chartPlacement is a layer element as POST /graph_layers/elements returns it.
type chartPlacement struct {
	ID            string                     `json:"id"`
	LaID          int                        `json:"laId"`
	Position      json.RawMessage            `json:"position"`
	LayerSettings map[string]json.RawMessage `json:"layerSettings"`
}

// readChartPlacement loads placement laID on layerID and checks that it holds
// the chart and that no layer edge is attached to it.
func readChartPlacement(ctx context.Context, layerID string, laID int, actorID, auth, baseURL string) (chartPlacement, error) {
	var pl chartPlacement
	data, err := chartHTTPJSON(ctx, "POST", fmt.Sprintf("%s/graph_layers/elements/%s", baseURL, layerID), auth,
		map[string][]int{"nodes": {laID}, "edges": {}})
	if err != nil {
		return pl, fmt.Errorf("read placement %d: %w", laID, err)
	}
	var resp struct {
		Data struct {
			Nodes []chartPlacement `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return pl, fmt.Errorf("parse placement %d: %w", laID, err)
	}
	if len(resp.Data.Nodes) == 0 {
		return pl, fmt.Errorf("laId %d is not on layer %s (or not visible to you) — getAllLayerPlacements lists the chart's laId", laID, layerID)
	}
	pl = resp.Data.Nodes[0]
	if pl.ID != actorID {
		return pl, fmt.Errorf("laId %d places actor %s, not the chart %s", laID, pl.ID, actorID)
	}
	if len(pl.Position) == 0 || string(pl.Position) == "null" {
		return pl, fmt.Errorf("placement %d has no position", laID)
	}

	// /papi cannot edit a placement's layerSettings in place, only delete and
	// re-create it — which would drop the layer edges drawn to the old laId.
	for offset := 0; ; offset += maxLayerPageLimit {
		page, err := chartHTTPGet(ctx, fmt.Sprintf("%s/graph_layers/paginated/%s?type=edges&limit=%d&offset=%d",
			baseURL, layerID, maxLayerPageLimit, offset), auth)
		if err != nil {
			return pl, fmt.Errorf("list layer edges: %w", err)
		}
		var edges struct {
			Data []struct {
				LaIDSource jsonInt `json:"laIdSource"`
				LaIDTarget jsonInt `json:"laIdTarget"`
			} `json:"data"`
		}
		if err := json.Unmarshal(page, &edges); err != nil {
			return pl, fmt.Errorf("parse layer edges: %w", err)
		}
		// a short page is not the last: the server drops deleted elements after LIMIT
		if len(edges.Data) == 0 {
			break
		}
		for _, e := range edges.Data {
			if int(e.LaIDSource) == laID || int(e.LaIDTarget) == laID {
				return pl, fmt.Errorf("placement %d has edges drawn on the layer; re-placing it (the only way /papi can change "+
					"its layerSettings) would drop them — remove those edges first, or resize the chart in the UI", laID)
			}
		}
	}
	return pl, nil
}

// chartLayerSettingsFor merges the chart expand/size onto a placement's current
// layerSettings, keeping every other key the UI wrote.
func chartLayerSettingsFor(cur map[string]json.RawMessage, expand bool, width, height int) map[string]json.RawMessage {
	ls := make(map[string]json.RawMessage, len(cur)+5)
	for k, v := range cur {
		ls[k] = v
	}
	setJSON(ls, "expandType", "chart")
	setJSON(ls, "expand", expand)
	if width != 0 {
		setJSON(ls, "width", width)
		setJSON(ls, "height", height)
		// An odd remainder goes right/bottom so the offsets add up to the size.
		left, top := (width-chartCellSize)/2, (height-chartCellSize)/2
		setJSON(ls, "offset", chartLayerOffsets{
			Left: left, Right: width - chartCellSize - left,
			Top: top, Bottom: height - chartCellSize - top,
		})
	} else if _, ok := ls["offset"]; !ok {
		setJSON(ls, "offset", newChartLayerSettings().Offset)
	}
	return ls
}

// UpdateChart reconfigures an existing chart without changing its actor id:
//  1. Read the Dashboards actor and merge cfg onto its data.source, validated by
//     normalizeChartConfig (and, for a placement change, read the placement).
//  2. Create a new ActorFilters actor when the filter changes.
//  3. PUT the new data.source (pong-server re-links the dashboard's actors).
//  4. Re-place laId with chart layerSettings (new laId, same position).
//
// Every validation and read happens before the first write. Step 4 takes two
// requests — pong rejects a create on a cell the old placement still holds,
// even when the same request deletes it — so its errors say what was written.
func UpdateChart(ctx context.Context, cfg UpdateChartConfig, workspaceID, auth, baseURL string) (UpdateChartResult, error) {
	result := UpdateChartResult{DashboardActorID: cfg.DashboardActorID}
	if err := cfg.validate(); err != nil {
		return result, err
	}

	data, err := chartHTTPGet(ctx, fmt.Sprintf("%s/actors/%s?attachments=false&meetingParticipants=false&lastTranscription=false&streams=false&webhooks=false&executionState=false&triggers=false",
		baseURL, cfg.DashboardActorID), auth)
	if err != nil {
		return result, fmt.Errorf("read chart actor: %w", err)
	}
	var ar struct {
		Data dashboardActor `json:"data"`
	}
	if err := json.Unmarshal(data, &ar); err != nil {
		return result, fmt.Errorf("parse chart actor: %w", err)
	}
	actor := ar.Data
	// Fail closed: this tool writes data.source, which is only safe on a chart.
	isDash, err := ecore.IsDashboardForm(ctx, baseURL, actor.FormID, func(ctx context.Context, path string, q url.Values) ([]byte, error) {
		return chartHTTPGet(ctx, baseURL+path+"?"+q.Encode(), auth)
	})
	if err != nil {
		return result, fmt.Errorf("check the actor's form %d: %w", actor.FormID, err)
	}
	if !isDash {
		return result, fmt.Errorf("actor %s is not a Dashboards (chart) actor", cfg.DashboardActorID)
	}

	if cfg.changesSource() {
		plan, warnings, err := planSource(ctx, cfg, actor, auth, baseURL)
		if err != nil {
			return result, err
		}
		result.Warnings = append(result.Warnings, warnings...)
		if plan.newFilter && workspaceID == "" {
			return result, errors.New("a new actorFilter source needs a workspace to create its ActorFilters actor — pass accId or run set-workspace")
		}
		result.SourceType, result.ChartType, result.CounterType, result.Range, result.OrderValue =
			plan.sourceType, plan.cfg.ChartType, plan.cfg.CounterType, plan.cfg.Range, plan.cfg.OrderValue

		var placement chartPlacement
		if cfg.LaID != 0 {
			if placement, err = readChartPlacement(ctx, cfg.LayerID, cfg.LaID, cfg.DashboardActorID, auth, baseURL); err != nil {
				return result, err
			}
		}

		if plan.newFilter {
			id, err := createFilterActor(ctx, plan.filterTitle, plan.filter.FormID, plan.filter.AccountNameID, plan.filter.CurrencyID,
				cfg.LayerID, workspaceID, auth, baseURL)
			if err != nil {
				return result, err
			}
			plan.filterActorID = id
			setJSON(plan.keys, "dynamicSource", plan.dynamicSource(id, plan.filterTitle, plan.filter))
		}
		body := map[string]any{"data": map[string]any{"source": chartJSONString(plan.keys)}}
		if _, err := chartHTTPJSON(ctx, "PUT", fmt.Sprintf("%s/actors/actor/%d/%s", baseURL, actor.FormID, cfg.DashboardActorID), auth, body); err != nil {
			if plan.newFilter {
				return result, fmt.Errorf("update chart source: %w (the new ActorFilters actor %s is unused — delete it)", err, plan.filterActorID)
			}
			return result, fmt.Errorf("update chart source: %w", err)
		}
		if plan.prevFilterID != "" && plan.filterActorID != plan.prevFilterID {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"the chart no longer uses ActorFilters actor %s — delete it if no other chart does", plan.prevFilterID))
		}
		result.FilterActorID = plan.filterActorID

		if cfg.LaID != 0 {
			if err := replaceChartPlacement(ctx, cfg, placement, auth, baseURL, &result); err != nil {
				// The source write above stands; say so, with what the caller must still know.
				note := "the chart config itself was saved"
				if len(result.Warnings) > 0 {
					note += "; " + strings.Join(result.Warnings, "; ")
				}
				return result, fmt.Errorf("%w (%s)", err, note)
			}
		}
		return result, nil
	}

	// Placement only: the actor must still hold a chart config.
	cur, _, err := parseDashboardSource(actor.Data.Source)
	if err != nil {
		return result, err
	}
	result.SourceType, result.ChartType, result.CounterType, result.Range, result.OrderValue =
		cur.SourceType, cur.ChartType, cur.CounterType, cur.Range, cur.OrderValue
	if cur.DynamicSource != nil {
		result.FilterActorID = cur.DynamicSource.ID
	}
	placement, err := readChartPlacement(ctx, cfg.LayerID, cfg.LaID, cfg.DashboardActorID, auth, baseURL)
	if err != nil {
		return result, err
	}
	if err := replaceChartPlacement(ctx, cfg, placement, auth, baseURL, &result); err != nil {
		return result, err
	}
	return result, nil
}

// replaceChartPlacement deletes placement cfg.LaID and creates the chart again
// at the same position with chart layerSettings, recording the new laId.
func replaceChartPlacement(ctx context.Context, cfg UpdateChartConfig, placement chartPlacement, auth, baseURL string, result *UpdateChartResult) error {
	expand := true
	if cfg.Expand != nil {
		expand = *cfg.Expand
	}
	layerURL := fmt.Sprintf("%s/graph_layers/actors/%s", baseURL, cfg.LayerID)
	remove := []map[string]any{{"action": "delete", "data": map[string]any{"id": cfg.DashboardActorID, "type": "node", "laId": cfg.LaID}}}
	if _, err := chartHTTPJSON(ctx, "POST", layerURL, auth, remove); err != nil {
		return fmt.Errorf("take placement %d off the layer: %w", cfg.LaID, err)
	}
	create := []map[string]any{{"action": "create", "data": map[string]any{
		"id": cfg.DashboardActorID, "type": "node", "position": placement.Position,
		"layerSettings": chartLayerSettingsFor(placement.LayerSettings, expand, cfg.Width, cfg.Height),
	}}}
	data, err := chartHTTPJSON(ctx, "POST", layerURL, auth, create)
	if err != nil {
		restore, _ := json.Marshal([]map[string]any{{"action": "create", "data": map[string]any{
			"id": cfg.DashboardActorID, "type": "node", "position": placement.Position, "layerSettings": placement.LayerSettings,
		}}})
		return fmt.Errorf("took placement %d off layer %s but placing the chart again failed: %w — put it back with "+
			"manageLayerActors(actorId=%q, items=%s)", cfg.LaID, cfg.LayerID, err, cfg.LayerID, restore)
	}
	var lr struct {
		Data struct {
			NodesMap []struct {
				LaID int `json:"laId"`
			} `json:"nodesMap"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &lr); err != nil || len(lr.Data.NodesMap) == 0 {
		return fmt.Errorf("re-placed the chart but could not read its new laId (%.200s) — getAllLayerPlacements lists it", data)
	}
	result.LaID, result.PreviousLaID = lr.Data.NodesMap[0].LaID, cfg.LaID
	return nil
}

// ---- MCP handler ----

func handleUpdateChart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	str := func(k string) string { v, _ := args[k].(string); return strings.TrimSpace(v) }
	num := func(k string) int { v, _ := args[k].(float64); return int(v) }

	cfg := UpdateChartConfig{
		DashboardActorID: str("dashboardActorId"),
		ChartType:        str("chartType"),
		CounterType:      str("counterType"),
		Range:            str("range"),
		OrderValue:       str("orderValue"),
		IncomeType:       str("incomeType"),
		Top:              num("top"),
		FilterActorID:    str("filterActorId"),
		FilterTitle:      str("filterTitle"),
		SourceFormID:     num("sourceFormId"),
		AccountNameID:    str("accountNameId"),
		CurrencyID:       num("currencyId"),
		LayerID:          str("layerId"),
		LaID:             num("laId"),
		Width:            num("width"),
		Height:           num("height"),
	}
	if v, ok := args["expand"].(bool); ok {
		cfg.Expand = &v
	}
	if arr, ok := args["accounts"].([]interface{}); ok {
		for _, item := range arr {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			a := ChartAccountEntry{}
			a.ActorID, _ = m["actorId"].(string)
			a.NameID, _ = m["nameId"].(string)
			a.Color, _ = m["color"].(string)
			a.IncomeType, _ = m["incomeType"].(string)
			if v, ok := m["currencyId"].(float64); ok {
				a.CurrencyID = int(v)
			}
			cfg.Accounts = append(cfg.Accounts, a)
		}
	}

	result, err := UpdateChart(ctx, cfg, ecore.WorkspaceIDForContext(ctx), ecore.AuthHeaderForContext(ctx), ecore.BuildBaseURLForContext(ctx))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] updateChart: %v", err)), nil
	}
	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}
