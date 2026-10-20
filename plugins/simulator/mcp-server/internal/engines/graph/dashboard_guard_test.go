package graph

import (
	"context"
	"fmt"
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

func formsServer(dashID int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forms/templates/system/") {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"id":%d,"title":"Dashboards"}]}`, dashID)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestLookupSystemFormID_PerBackend(t *testing.T) {
	clearChartFormCache()
	a, b := formsServer(111), formsServer(222)
	defer a.Close()
	defer b.Close()
	ctx := context.Background()

	idA, err := lookupSystemFormID(ctx, "Dashboards", "ws", "auth", a.URL)
	if err != nil || idA != 111 {
		t.Fatalf("A: got %d, %v", idA, err)
	}
	idB, err := lookupSystemFormID(ctx, "Dashboards", "ws", "auth", b.URL)
	if err != nil || idB != 222 {
		t.Fatalf("B: got %d, %v", idB, err)
	}
	// same backend, different workspace
	idC, err := lookupSystemFormID(ctx, "Dashboards", "ws2", "auth", a.URL)
	if err != nil || idC != 111 {
		t.Fatalf("ws2: got %d, %v", idC, err)
	}

	sb := newGraphSyncer(b.URL, "Simulator t", "ws")
	if !sb.isDashboardFormID(ctx, 222) {
		t.Error("B's Dashboards id must be blocked after A's id was cached")
	}
	if sb.isDashboardFormID(ctx, 111) {
		t.Error("A's id must not be treated as B's Dashboards form")
	}
}

func TestLookupSystemFormID_FallbackDoesNotOverwrite(t *testing.T) {
	clearChartFormCache()
	systemOK := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forms/templates/system/") && systemOK:
			_, _ = w.Write([]byte(`{"data":[{"id":111,"title":"Dashboards"}]}`))
		case strings.Contains(r.URL.Path, "/forms/templates/") && !strings.Contains(r.URL.Path, "/system/"):
			_, _ = w.Write([]byte(`{"data":[{"id":999,"title":"Dashboards"},{"id":5,"title":"ActorFilters"}]}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if id, err := lookupSystemFormID(ctx, "Dashboards", "ws", "auth", srv.URL); err != nil || id != 111 {
		t.Fatalf("got %d, %v", id, err)
	}
	systemOK = false
	// ActorFilters is unknown → fallback endpoint is consulted; it also lists a
	// custom "Dashboards" (999) that must not replace the system id.
	if id, err := lookupSystemFormID(ctx, "ActorFilters", "ws", "auth", srv.URL); err != nil || id != 5 {
		t.Fatalf("ActorFilters: got %d, %v", id, err)
	}
	if id, _ := lookupSystemFormID(ctx, "Dashboards", "ws", "auth", srv.URL); id != 111 {
		t.Errorf("custom Dashboards form overwrote the system id: got %d", id)
	}
}

func TestValidateChartAccounts(t *testing.T) {
	ok := ChartAccountEntry{ActorID: "a", NameID: "n", CurrencyID: 1, IncomeType: "total"}
	if err := validateChartAccounts([]ChartAccountEntry{ok}); err != nil {
		t.Errorf("valid: %v", err)
	}
	bad := [][]ChartAccountEntry{
		{{NameID: "n", CurrencyID: 1}},
		{{ActorID: "a", CurrencyID: 1}},
		{{ActorID: "a", NameID: "n"}},
		{ok, ok},
	}
	for i, b := range bad {
		if validateChartAccounts(b) == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
