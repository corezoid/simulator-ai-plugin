package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// clearChartFormCache resets the package-global title→id cache so a test's mock
// server is actually consulted (other tests may have populated "Dashboards").
func clearChartFormCache() {
	chartFormCacheMu.Lock()
	chartFormCache = map[string]int{}
	chartFormCacheMu.Unlock()
}

func TestIsDashboardFormID(t *testing.T) {
	clearChartFormCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forms/templates/system/") {
			_, _ = w.Write([]byte(`{"data":[{"id":71872,"title":"Dashboards"},{"id":100,"title":"Clients"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newGraphSyncer(srv.URL, "Simulator t", t.Name())
	if !s.isDashboardFormID(context.Background(), 71872) {
		t.Error("71872 (system Dashboards) should be recognized as the dashboard form")
	}
	if s.isDashboardFormID(context.Background(), 100) {
		t.Error("100 (Clients) must not be recognized as the dashboard form")
	}
	if s.isDashboardFormID(context.Background(), 0) {
		t.Error("formID 0 must never match")
	}
}

func TestIsDashboardFormID_FailsOpen(t *testing.T) {
	clearChartFormCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	s := newGraphSyncer(srv.URL, "Simulator t", t.Name())
	if s.isDashboardFormID(context.Background(), 71872) {
		t.Error("unresolvable Dashboards form must fail open (false), not block")
	}
}

func TestDashboardSourceHasContent(t *testing.T) {
	empty := []map[string]interface{}{
		nil,
		{},
		{"source": nil},
		{"source": ""},
		{"source": "{}"},
		{"source": `""`},
		{"source": map[string]interface{}{}},
		{"title": "no source key"},
	}
	for _, d := range empty {
		if dashboardSourceHasContent(d) {
			t.Errorf("expected %#v to have no source content", d)
		}
	}
	full := []map[string]interface{}{
		{"source": `{"chartType":"line"}`},
		{"source": map[string]interface{}{"chartType": "bar"}},
	}
	for _, d := range full {
		if !dashboardSourceHasContent(d) {
			t.Errorf("expected %#v to have source content", d)
		}
	}
}
