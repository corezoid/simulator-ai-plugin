package graph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	updDashID    = "d0000000-0000-4000-8000-000000000001"
	updFilterID  = "f0000000-0000-4000-8000-000000000001"
	updFilter2ID = "f0000000-0000-4000-8000-000000000002"
	updLayerID   = "10000000-0000-4000-8000-000000000001"
	updNameID    = "a0000000-0000-4000-8000-000000000001"
)

// filterSourceJSON is an actorFilter source as the UI stores it, with keys the
// plugin does not model (foo, groupByFields content) that must survive.
const filterSourceJSON = `{"defaultAccount":null,"accounts":[],"counterType":"amount","chartType":"line",` +
	`"sourceType":"actorFilter","range":"lastWeek","rangeDates":{"from":null,"to":null},"showTotal":false,` +
	`"orderValue":"default","legend":{"actorTitle":false,"accountName":true,"currencyName":true},` +
	`"displayChartDataLabels":true,"chartViewMode":"default","foo":"keep-me",` +
	`"dynamicSource":{"id":"` + updFilterID + `","title":"Spenders","top":"20","groupByFields":["region"],` +
	`"filter":{"formId":501,"selectedForms":[501],"accountNameId":"` + updNameID + `","currencyId":7,"qFormId":"501"}}}`

// manualSourceJSON is a direct-accounts source written by createChart before
// v2.12.0 (sourceType "accounts").
const manualSourceJSON = `{"accounts":[` +
	`{"actorId":"` + chartTestActor1 + `","color":"#111111","incomeType":"total","currencyId":1,"nameId":"` + chartTestName1 + `"},` +
	`{"actorId":"` + chartTestActor2 + `","color":"#222222","incomeType":"total","currencyId":1,"nameId":"` + chartTestName1 + `"}],` +
	`"counterType":"amount","chartType":"bar","sourceType":"accounts","range":"lastWeek","orderValue":"desc",` +
	`"legend":{"actorTitle":true,"accountName":true,"currencyName":true}}`

type updStub struct {
	formTitle string // title of form 243 ("Dashboards" unless set)
	source    string // the chart's data.source
	edges     string // JSON array of layer edges
	mu        sync.Mutex
	calls     []chartRequest
}

func (s *updStub) serve(t *testing.T) *httptest.Server {
	t.Helper()
	if s.formTitle == "" {
		s.formTitle = "Dashboards"
	}
	if s.edges == "" {
		s.edges = "[]"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.calls = append(s.calls, chartRequest{r.Method, r.URL.Path, body})
		s.mu.Unlock()
		p := r.URL.Path
		switch {
		case p == "/actors/"+updDashID:
			src, _ := json.Marshal(s.source)
			_, _ = w.Write([]byte(`{"data":{"id":"` + updDashID + `","formId":243,"title":"Spend chart","data":{"source":` + string(src) + `}}}`))
		case p == "/actors/"+updFilter2ID:
			f, _ := json.Marshal(`{"formId":900,"accountNameId":"` + updNameID + `","currencyId":3}`)
			_, _ = w.Write([]byte(`{"data":{"title":"Other filter","data":{"filter":` + string(f) + `}}}`))
		case strings.HasPrefix(p, "/forms/templates/"):
			_, _ = w.Write([]byte(`{"data":[{"id":243,"title":"Dashboards"},{"id":244,"title":"ActorFilters"}]}`))
		case p == "/forms/243":
			_, _ = w.Write([]byte(`{"data":{"type":"system","title":"` + s.formTitle + `"}}`))
		case r.Method == "POST" && p == "/actors/actor/244":
			_, _ = w.Write([]byte(`{"data":{"id":"filter-new"}}`))
		case r.Method == "PUT" && p == "/actors/actor/243/"+updDashID:
			_, _ = w.Write([]byte(`{"data":{"id":"` + updDashID + `"}}`))
		case p == "/graph_layers/elements/"+updLayerID:
			_, _ = w.Write([]byte(`{"data":{"nodes":[{"id":"` + updDashID + `","laId":77,"position":{"x":120,"y":-40},` +
				`"layerSettings":{"expandType":"chart","expand":false,"pin":true,"offset":{"left":300,"right":300,"top":200,"bottom":200}}}],"edges":[]}}`))
		case p == "/graph_layers/paginated/"+updLayerID:
			if r.URL.Query().Get("offset") == "0" {
				_, _ = w.Write([]byte(`{"data":` + s.edges + `}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[]}`))
			}
		case r.Method == "POST" && p == "/graph_layers/actors/"+updLayerID:
			_, _ = w.Write([]byte(`{"data":{"nodesMap":[{"laId":88}]}}`))
		default:
			http.Error(w, "Route not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *updStub) writes() []chartRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []chartRequest
	for _, c := range s.calls {
		if c.Method != "GET" && !strings.HasPrefix(c.Path, "/graph_layers/elements/") {
			out = append(out, c)
		}
	}
	return out
}

// putSource returns the data.source the update wrote, or nil if there was no PUT.
func (s *updStub) putSource(t *testing.T) map[string]any {
	t.Helper()
	for _, c := range s.writes() {
		if c.Method != "PUT" {
			continue
		}
		var body struct {
			Data struct {
				Source string `json:"source"`
			} `json:"data"`
		}
		if err := json.Unmarshal(c.Body, &body); err != nil {
			t.Fatal(err)
		}
		var src map[string]any
		if err := json.Unmarshal([]byte(body.Data.Source), &src); err != nil {
			t.Fatalf("source is not a JSON string of an object: %v", err)
		}
		return src
	}
	return nil
}

func TestUpdateChartPatchesInPlaceAndKeepsUIKeys(t *testing.T) {
	stub := &updStub{source: filterSourceJSON}
	srv := stub.serve(t)
	res, err := UpdateChart(context.Background(), UpdateChartConfig{
		DashboardActorID: updDashID, ChartType: "bar", OrderValue: "desc", Top: 5,
	}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.DashboardActorID != updDashID || res.ChartType != "bar" || res.OrderValue != "desc" ||
		res.Range != "lastWeek" || res.SourceType != "actorFilter" || res.FilterActorID != updFilterID {
		t.Errorf("result = %+v", res)
	}
	if w := stub.writes(); len(w) != 1 {
		t.Fatalf("writes = %v, want exactly the PUT of the chart actor", w)
	}
	src := stub.putSource(t)
	if src["chartType"] != "bar" || src["orderValue"] != "desc" || src["counterType"] != "amount" || src["range"] != "lastWeek" {
		t.Errorf("source = %v", src)
	}
	if src["foo"] != "keep-me" || src["showTotal"] != false {
		t.Errorf("UI keys lost: foo=%v showTotal=%v", src["foo"], src["showTotal"])
	}
	if legend, _ := src["legend"].(map[string]any); legend["actorTitle"] != false {
		t.Errorf("legend = %v, want the stored legend kept", src["legend"])
	}
	ds, _ := src["dynamicSource"].(map[string]any)
	if ds["id"] != updFilterID || ds["top"] != "5" {
		t.Errorf("dynamicSource = %v", ds)
	}
	if g, _ := ds["groupByFields"].([]any); len(g) != 1 || g[0] != "region" {
		t.Errorf("groupByFields = %v, want kept", ds["groupByFields"])
	}
}

func TestUpdateChartValidatesLikeCreateChart(t *testing.T) {
	stub := &updStub{source: filterSourceJSON}
	srv := stub.serve(t)
	// line chart: realTime is stored as lineRealTime, a sort is dropped.
	res, err := UpdateChart(context.Background(), UpdateChartConfig{
		DashboardActorID: updDashID, Range: "realTime", OrderValue: "asc",
	}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.Range != "lineRealTime" || res.OrderValue != "default" || len(res.Warnings) != 2 {
		t.Errorf("result = %+v, want lineRealTime/default with two warnings", res)
	}

	stub2 := &updStub{source: filterSourceJSON}
	srv2 := stub2.serve(t)
	for _, cfg := range []UpdateChartConfig{
		{DashboardActorID: updDashID, CounterType: "turnover"},
		{DashboardActorID: updDashID, Range: "allTime"}, // still a line chart
		{DashboardActorID: updDashID, IncomeType: "credit"},
	} {
		if _, err := UpdateChart(context.Background(), cfg, "ws", "t", srv2.URL); err == nil {
			t.Errorf("%+v: expected an error", cfg)
		}
	}
	if w := stub2.writes(); len(w) != 0 {
		t.Errorf("rejected updates wrote %v", w)
	}
}

func TestUpdateChartDirectAccounts(t *testing.T) {
	stub := &updStub{source: manualSourceJSON}
	srv := stub.serve(t)
	res, err := UpdateChart(context.Background(), UpdateChartConfig{DashboardActorID: updDashID, IncomeType: "credit"}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceType != "manual" {
		t.Errorf("sourceType = %q, want the legacy \"accounts\" rewritten to manual", res.SourceType)
	}
	src := stub.putSource(t)
	accounts, _ := src["accounts"].([]any)
	if len(accounts) != 2 {
		t.Fatalf("accounts = %v", src["accounts"])
	}
	for i, want := range []string{"#111111", "#222222"} {
		a := accounts[i].(map[string]any)
		if a["incomeType"] != "credit" || a["color"] != want {
			t.Errorf("accounts[%d] = %v, want incomeType credit and its colour kept", i, a)
		}
	}
	if _, ok := src["dynamicSource"]; ok {
		t.Error("a direct-accounts source must not carry dynamicSource")
	}

	// Two series that become identical are rejected (the chart would show one).
	dup := &updStub{source: strings.Replace(manualSourceJSON, chartTestActor2, chartTestActor1, 1)}
	dupSrv := dup.serve(t)
	_, err = UpdateChart(context.Background(), UpdateChartConfig{DashboardActorID: updDashID, IncomeType: "credit"}, "ws", "t", dupSrv.URL)
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Errorf("err = %v, want the duplicate-series error", err)
	}
}

func TestUpdateChartSwitchesToAccounts(t *testing.T) {
	stub := &updStub{source: filterSourceJSON}
	srv := stub.serve(t)
	res, err := UpdateChart(context.Background(), UpdateChartConfig{
		DashboardActorID: updDashID,
		Accounts: []ChartAccountEntry{
			{ActorID: chartTestActor1, CurrencyID: 1, NameID: chartTestName1},
			{ActorID: chartTestActor1, CurrencyID: 1, NameID: chartTestName1, IncomeType: "debit"},
		},
		IncomeType: "credit",
	}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	src := stub.putSource(t)
	if src["sourceType"] != "manual" || src["dynamicSource"] != nil {
		t.Errorf("source = %v, want manual without dynamicSource", src)
	}
	a0 := src["accounts"].([]any)[0].(map[string]any)
	if a0["incomeType"] != "credit" {
		t.Errorf("accounts[0].incomeType = %v, want the incomeType default", a0["incomeType"])
	}
	if legend := src["legend"].(map[string]any); legend["incomeType"] != true {
		t.Errorf("legend = %v, want incomeType on for credit+debit series", legend)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], updFilterID) {
		t.Errorf("warnings = %v, want the orphaned-filter note", res.Warnings)
	}
}

func TestUpdateChartNewFilterGetsItsOwnActor(t *testing.T) {
	stub := &updStub{source: filterSourceJSON}
	srv := stub.serve(t)
	res, err := UpdateChart(context.Background(), UpdateChartConfig{DashboardActorID: updDashID, SourceFormID: 777}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilterActorID != "filter-new" {
		t.Errorf("filterActorId = %q", res.FilterActorID)
	}
	w := stub.writes()
	if len(w) != 2 || w[0].Path != "/actors/actor/244" || w[1].Method != "PUT" {
		t.Fatalf("writes = %v, want the new ActorFilters actor, then the PUT", w)
	}
	var filterBody struct {
		Title string `json:"title"`
		Data  struct {
			Filter string `json:"filter"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w[0].Body, &filterBody)
	if filterBody.Title != "Spenders" || !strings.Contains(filterBody.Data.Filter, `"formId":777`) ||
		!strings.Contains(filterBody.Data.Filter, updNameID) || !strings.Contains(filterBody.Data.Filter, `"currencyId":7`) {
		t.Errorf("filter actor = %s, want formId 777 with the current account name and currency", w[0].Body)
	}
	ds := stub.putSource(t)["dynamicSource"].(map[string]any)
	if ds["id"] != "filter-new" || ds["filter"].(map[string]any)["formId"] != float64(777) || ds["top"] != "20" {
		t.Errorf("dynamicSource = %v", ds)
	}
}

func TestUpdateChartRepointsAtExistingFilter(t *testing.T) {
	stub := &updStub{source: filterSourceJSON}
	srv := stub.serve(t)
	if _, err := UpdateChart(context.Background(), UpdateChartConfig{DashboardActorID: updDashID, FilterActorID: updFilter2ID}, "ws", "t", srv.URL); err != nil {
		t.Fatal(err)
	}
	ds := stub.putSource(t)["dynamicSource"].(map[string]any)
	if ds["id"] != updFilter2ID || ds["title"] != "Other filter" || ds["filter"].(map[string]any)["formId"] != float64(900) {
		t.Errorf("dynamicSource = %v", ds)
	}
}

func TestUpdateChartRefusesNonDashboard(t *testing.T) {
	stub := &updStub{source: filterSourceJSON, formTitle: "Clients"}
	srv := stub.serve(t)
	_, err := UpdateChart(context.Background(), UpdateChartConfig{DashboardActorID: updDashID, ChartType: "bar"}, "ws", "t", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "not a Dashboards") {
		t.Fatalf("err = %v", err)
	}
	if w := stub.writes(); len(w) != 0 {
		t.Errorf("wrote %v to a non-chart actor", w)
	}
}

func TestUpdateChartReplacesPlacement(t *testing.T) {
	stub := &updStub{source: filterSourceJSON, edges: `[{"laId":5,"id":"e1","laIdSource":12,"laIdTarget":13}]`}
	srv := stub.serve(t)
	res, err := UpdateChart(context.Background(), UpdateChartConfig{
		DashboardActorID: updDashID, LayerID: updLayerID, LaID: 77, Width: 750, Height: 550,
	}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.LaID != 88 || res.PreviousLaID != 77 {
		t.Errorf("result = %+v, want laId 88 replacing 77", res)
	}
	w := stub.writes()
	if len(w) != 1 || w[0].Path != "/graph_layers/actors/"+updLayerID {
		t.Fatalf("writes = %v, want only the re-placement (no source change)", w)
	}
	var ops []struct {
		Action string `json:"action"`
		Data   struct {
			ID            string         `json:"id"`
			LaID          int            `json:"laId"`
			Position      map[string]any `json:"position"`
			LayerSettings map[string]any `json:"layerSettings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w[0].Body, &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Action != "delete" || ops[0].Data.LaID != 77 || ops[1].Action != "create" || ops[1].Data.ID != updDashID {
		t.Fatalf("ops = %s", w[0].Body)
	}
	ls := ops[1].Data.LayerSettings
	off, _ := ls["offset"].(map[string]any)
	if ops[1].Data.Position["x"] != float64(120) || ls["expand"] != true || ls["expandType"] != "chart" || ls["pin"] != true ||
		ls["width"] != float64(750) || off["left"] != float64(350) || off["top"] != float64(250) {
		t.Errorf("create = %+v", ops[1].Data)
	}
}

func TestUpdateChartRefusesPlacementWithEdges(t *testing.T) {
	stub := &updStub{source: filterSourceJSON, edges: `[{"laId":5,"id":"e1","laIdSource":77,"laIdTarget":13}]`}
	srv := stub.serve(t)
	_, err := UpdateChart(context.Background(), UpdateChartConfig{
		DashboardActorID: updDashID, ChartType: "bar", LayerID: updLayerID, LaID: 77,
	}, "ws", "t", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "edges") {
		t.Fatalf("err = %v, want the layer-edges refusal", err)
	}
	if w := stub.writes(); len(w) != 0 {
		t.Errorf("wrote %v before refusing — every check must run before the first write", w)
	}
}

func TestUpdateChartConfigValidate(t *testing.T) {
	yes := true
	for _, tc := range []struct {
		name string
		cfg  UpdateChartConfig
		want string
	}{
		{"no id", UpdateChartConfig{ChartType: "bar"}, "dashboardActorId"},
		{"nothing", UpdateChartConfig{DashboardActorID: updDashID}, "nothing to change"},
		{"accounts+top", UpdateChartConfig{DashboardActorID: updDashID, Top: 3, Accounts: []ChartAccountEntry{{}}}, "cannot be combined"},
		{"expand without laId", UpdateChartConfig{DashboardActorID: updDashID, Expand: &yes}, "laId"},
		{"laId without layer", UpdateChartConfig{DashboardActorID: updDashID, LaID: 1}, "layerId"},
		{"width alone", UpdateChartConfig{DashboardActorID: updDashID, LayerID: updLayerID, LaID: 1, Width: 700}, "together"},
		{"too small", UpdateChartConfig{DashboardActorID: updDashID, LayerID: updLayerID, LaID: 1, Width: 300, Height: 300}, "minimum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
