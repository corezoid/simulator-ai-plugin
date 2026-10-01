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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeChartConfig(&tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a %s error", err, tc.want)
			}
		})
	}
}

func TestNormalizeChartConfigDefaultsAndOrder(t *testing.T) {
	cfg := ChartConfig{Accounts: []ChartAccountEntry{{}}}
	if err := normalizeChartConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ChartType != "line" || cfg.CounterType != "amount" || cfg.Range != "lastHour" ||
		cfg.OrderValue != "default" || cfg.Top != 20 || cfg.Accounts[0].IncomeType != "total" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}

	// The UI ignores sorting for multi-series charts and always stores "default".
	line := ChartConfig{ChartType: "line", OrderValue: "desc"}
	bar := ChartConfig{ChartType: "bar", OrderValue: "desc"}
	if err := normalizeChartConfig(&line); err != nil {
		t.Fatal(err)
	}
	if err := normalizeChartConfig(&bar); err != nil {
		t.Fatal(err)
	}
	if line.OrderValue != "default" {
		t.Errorf("line orderValue = %q, want default", line.OrderValue)
	}
	if bar.OrderValue != "desc" {
		t.Errorf("bar orderValue = %q, want desc", bar.OrderValue)
	}
}

type chartRequest struct {
	Method, Path string
	Body         []byte
}

// chartStub serves every endpoint CreateChart touches and records the calls.
// The layer has no linked Graphs actor.
func chartStub(t *testing.T) (*httptest.Server, func() []chartRequest) {
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
			_, _ = w.Write([]byte(`{"data":[]}`))
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
	srv, calls := chartStub(t)
	cfg := ChartConfig{
		LayerID:     "11111111-1111-1111-1111-111111111111",
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

func TestCreateChartRejectsBeforeAnyRequest(t *testing.T) {
	srv, calls := chartStub(t)
	_, err := CreateChart(context.Background(), ChartConfig{CounterType: "turnover"}, "ws", "t", srv.URL)
	if err == nil {
		t.Fatal("expected an error for counterType turnover")
	}
	if n := len(calls()); n != 0 {
		t.Errorf("made %d requests before rejecting invalid config", n)
	}
}
