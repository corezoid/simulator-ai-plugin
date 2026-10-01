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

// TestNormalizeChartConfigRejectsValuesTheUICannotRender guards issue #109: a
// counterType/chartType/range the Simulator UI does not know is stored fine but
// the chart renders "Something went wrong".
func TestNormalizeChartConfigRejectsValuesTheUICannotRender(t *testing.T) {
	cases := []struct {
		name string
		cfg  ChartConfig
		want string
	}{
		{"turnover", ChartConfig{CounterType: "turnover"}, "counterType"},
		{"area", ChartConfig{ChartType: "area"}, "chartType"},
		{"lastDay", ChartConfig{Range: "lastDay"}, "range"},
		{"orderValue", ChartConfig{OrderValue: "DESC"}, "orderValue"},
		{"incomeType", ChartConfig{Accounts: []ChartAccountEntry{{IncomeType: "in"}}}, "accounts[0].incomeType"},
		// The UI hides allTime for multi-series charts and resets it to null.
		{"allTime on line", ChartConfig{Range: "allTime"}, "allTime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeChartConfig(&tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a %s error", err, tc.want)
			}
		})
	}
}

func TestNormalizeChartConfigDefaults(t *testing.T) {
	accounts := []ChartAccountEntry{{}}
	cfg := ChartConfig{Accounts: accounts}
	warnings, err := normalizeChartConfig(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChartType != "line" || cfg.CounterType != "amount" || cfg.Range != "lastHour" ||
		cfg.OrderValue != "default" || cfg.Top != 20 || cfg.Accounts[0].IncomeType != "total" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if len(warnings) != 0 {
		t.Errorf("defaults produced warnings: %v", warnings)
	}
	if accounts[0].IncomeType != "" {
		t.Errorf("caller's Accounts slice was mutated: %+v", accounts[0])
	}
}

// TestNormalizeChartConfigPerTypeRules mirrors the UI's per-chartType rewrites
// (rangeUtils.adjustRangeForChartType, buildPayload orderValue) and checks each
// rewrite is reported rather than silent.
func TestNormalizeChartConfigPerTypeRules(t *testing.T) {
	cases := []struct {
		name               string
		cfg                ChartConfig
		wantRange, wantOrd string
		wantWarn           string
	}{
		{"line ignores sorting", ChartConfig{ChartType: "line", OrderValue: "desc"}, "lastHour", "default", "orderValue desc ignored"},
		{"bar keeps sorting", ChartConfig{ChartType: "bar", OrderValue: "desc"}, "lastHour", "desc", ""},
		{"line realTime", ChartConfig{ChartType: "line", Range: "realTime"}, "lineRealTime", "default", "stored as lineRealTime"},
		{"bar lineRealTime", ChartConfig{ChartType: "bar", Range: "lineRealTime"}, "realTime", "default", "stored as realTime"},
		{"bar allTime", ChartConfig{ChartType: "bar", Range: "allTime"}, "allTime", "default", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings, err := normalizeChartConfig(&tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if tc.cfg.Range != tc.wantRange || tc.cfg.OrderValue != tc.wantOrd {
				t.Errorf("range=%q orderValue=%q, want %q %q", tc.cfg.Range, tc.cfg.OrderValue, tc.wantRange, tc.wantOrd)
			}
			got := strings.Join(warnings, "; ")
			if (tc.wantWarn == "") != (got == "") || !strings.Contains(got, tc.wantWarn) {
				t.Errorf("warnings = %q, want %q", got, tc.wantWarn)
			}
		})
	}
}

type chartRequest struct {
	Method, Path string
	Body         []byte
}

// linkedActors* are GET /graph/linked_actors/{layer} bodies in the real
// {data: {nodes, edges}} shape; nodes include the layer itself.
const (
	chartTestLayerID      = "11111111-1111-1111-1111-111111111111"
	linkedActorsNoGraph   = `{"data":{"nodes":[{"id":"` + chartTestLayerID + `","formTitle":"Layers"}],"edges":[]}}`
	linkedActorsWithGraph = `{"data":{"nodes":[{"id":"` + chartTestLayerID + `","formTitle":"Layers"},` +
		`{"id":"graph-1","formTitle":"Graphs"}],"edges":[{"source":"graph-1","target":"` + chartTestLayerID + `"}]}}`
)

// chartStub serves every endpoint CreateChart touches and records the calls.
// linkedActors is the linked_actors body; "" makes that endpoint fail with 500.
func chartStub(t *testing.T, linkedActors string) (*httptest.Server, func() []chartRequest) {
	t.Helper()
	var mu sync.Mutex
	var calls []chartRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, chartRequest{r.Method, r.URL.Path, body})
		mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/forms/templates/"):
			_, _ = w.Write([]byte(`{"data":[{"id":243,"title":"Dashboards"}]}`))
		case strings.HasPrefix(r.URL.Path, "/actors/actor/"):
			_, _ = w.Write([]byte(`{"data":{"id":"dash-1"}}`))
		case strings.HasPrefix(r.URL.Path, "/graph_layers/actors/"):
			_, _ = w.Write([]byte(`{"data":{"nodesMap":[{"laId":77}]}}`))
		case strings.HasPrefix(r.URL.Path, "/graph/linked_actors/"):
			if linkedActors == "" {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(linkedActors))
		case strings.HasPrefix(r.URL.Path, "/accounts/inherit/"):
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			http.Error(w, "Route not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []chartRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]chartRequest(nil), calls...)
	}
}

func TestCreateChartWritesUIValuesAndExpandsOnPlacement(t *testing.T) {
	srv, calls := chartStub(t, linkedActorsNoGraph)
	cfg := ChartConfig{
		LayerID:     chartTestLayerID,
		Title:       "Incoming per day",
		ChartType:   "bar",
		CounterType: "count",
		Range:       "lastWeek",
		OrderValue:  "desc",
		Accounts:    []ChartAccountEntry{{ActorID: "a1", CurrencyID: 1, NameID: "n1", IncomeType: "credit"}},
	}
	res, err := CreateChart(context.Background(), cfg, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.DashboardActorID != "dash-1" || res.LaID != 77 {
		t.Errorf("result = %+v", res)
	}

	var sawDashboard, sawPlacement bool
	for _, c := range calls() {
		if strings.Contains(c.Path, "actor_settings") {
			t.Errorf("unexpected %s %s — /papi has no actor_settings route", c.Method, c.Path)
		}
		switch {
		case c.Method == "POST" && strings.HasPrefix(c.Path, "/actors/actor/243"):
			sawDashboard = true
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
				t.Fatal(err)
			}
			if src["counterType"] != "count" || src["orderValue"] != "desc" || src["chartType"] != "bar" {
				t.Errorf("source = %v", src)
			}
		case c.Method == "POST" && strings.HasPrefix(c.Path, "/graph_layers/actors/"):
			sawPlacement = true
			var body []struct {
				Data struct {
					LayerSettings map[string]any `json:"layerSettings"`
				} `json:"data"`
			}
			if err := json.Unmarshal(c.Body, &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body[0].Data.LayerSettings["expandType"] != "chart" || body[0].Data.LayerSettings["expand"] != true {
				t.Errorf("placement body = %s", c.Body)
			}
		}
	}
	if !sawDashboard || !sawPlacement {
		t.Errorf("dashboard POST seen=%v, placement POST seen=%v", sawDashboard, sawPlacement)
	}

	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not linked to a Graphs actor") {
		t.Errorf("warnings = %v, want the unlinked-graph warning", res.Warnings)
	}
}

// inheritParents returns the parents sent to POST /accounts/inherit.
func inheritParents(t *testing.T, calls []chartRequest) []string {
	t.Helper()
	for _, c := range calls {
		if strings.HasPrefix(c.Path, "/accounts/inherit/") {
			var body struct {
				Parents []string `json:"parents"`
			}
			if err := json.Unmarshal(c.Body, &body); err != nil {
				t.Fatal(err)
			}
			return body.Parents
		}
	}
	t.Fatal("no /accounts/inherit call")
	return nil
}

func TestCreateChartInheritsFromLinkedGraph(t *testing.T) {
	srv, calls := chartStub(t, linkedActorsWithGraph)
	res, err := CreateChart(context.Background(), ChartConfig{LayerID: chartTestLayerID, Title: "t",
		Accounts: []ChartAccountEntry{{ActorID: "a1", CurrencyID: 1, NameID: "n1"}}}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := inheritParents(t, calls()); strings.Join(got, ",") != "graph-1,"+chartTestLayerID {
		t.Errorf("inherit parents = %v, want [graph-1 layer]", got)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", res.Warnings)
	}
}

func TestCreateChartReportsGraphLookupFailure(t *testing.T) {
	srv, calls := chartStub(t, "")
	res, err := CreateChart(context.Background(), ChartConfig{LayerID: chartTestLayerID, Title: "t",
		Accounts: []ChartAccountEntry{{ActorID: "a1", CurrencyID: 1, NameID: "n1"}}}, "ws", "t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := inheritParents(t, calls()); strings.Join(got, ",") != chartTestLayerID {
		t.Errorf("inherit parents = %v, want [layer]", got)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "could not look up the layer's graph") {
		t.Errorf("warnings = %v, want the lookup-failure warning (not \"not linked\")", res.Warnings)
	}
}

func TestCreateChartRejectsBeforeAnyRequest(t *testing.T) {
	srv, calls := chartStub(t, linkedActorsNoGraph)
	_, err := CreateChart(context.Background(), ChartConfig{CounterType: "turnover"}, "ws", "t", srv.URL)
	if err == nil {
		t.Fatal("expected an error for counterType turnover")
	}
	if n := len(calls()); n != 0 {
		t.Errorf("made %d requests before rejecting invalid config", n)
	}
}
