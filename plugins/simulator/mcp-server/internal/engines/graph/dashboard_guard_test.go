package graph

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// clearChartFormCache resets the package-global system form id cache so a test's
// mock server is actually consulted.
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

// systemFormsServer serves GET /forms/templates/system/{ws} with Dashboards →
// ids[ws]; any other workspace or route is a 404.
func systemFormsServer(t *testing.T, ids map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, ok := strings.CutPrefix(r.URL.Path, "/forms/templates/system/")
		if id, known := ids[ws]; ok && known {
			_, _ = fmt.Fprintf(w, `{"data":[{"id":%d,"title":"Dashboards","type":"system"}]}`, id)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLookupSystemFormIDScopedPerBackend: after set-environment the same
// workspace id can live on another backend with a different Dashboards id; the
// first backend's cached id must not leak to the second.
func TestLookupSystemFormIDScopedPerBackend(t *testing.T) {
	clearChartFormCache()
	ctx := context.Background()
	a := systemFormsServer(t, map[string]int{"ws": 101})
	b := systemFormsServer(t, map[string]int{"ws": 202})

	for _, step := range []struct {
		srv  *httptest.Server
		want int
	}{{a, 101}, {b, 202}, {a, 101}, {b, 202}} { // second round is served from the cache
		got, err := lookupSystemFormID(ctx, "Dashboards", "ws", "t", step.srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if got != step.want {
			t.Errorf("Dashboards on %s = %d, want %d", step.srv.URL, got, step.want)
		}
	}
}

// TestLookupSystemFormIDScopedPerWorkspace: set-workspace keeps the backend but
// every workspace has its own Dashboards form.
func TestLookupSystemFormIDScopedPerWorkspace(t *testing.T) {
	clearChartFormCache()
	ctx := context.Background()
	srv := systemFormsServer(t, map[string]int{"wsA": 101, "wsB": 202})

	for _, step := range []struct {
		ws   string
		want int
	}{{"wsA", 101}, {"wsB", 202}, {"wsA", 101}} {
		got, err := lookupSystemFormID(ctx, "Dashboards", step.ws, "t", srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if got != step.want {
			t.Errorf("Dashboards in %s = %d, want %d", step.ws, got, step.want)
		}
	}
}

// TestIsDashboardFormIDAfterWorkspaceSwitch guards the push-guard bypass: once
// workspace A's Dashboards id is cached, a push into workspace B must still
// block B's Dashboards form (and not treat A's id as B's).
func TestIsDashboardFormIDAfterWorkspaceSwitch(t *testing.T) {
	clearChartFormCache()
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		urlA, urlB string
		wsA, wsB   string
		idA, idB   int
	}{
		{name: "set-workspace", wsA: "wsA", wsB: "wsB", idA: 101, idB: 202},
		{name: "set-environment", wsA: "ws", wsB: "ws", idA: 303, idB: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wsA == tc.wsB {
				tc.urlA = systemFormsServer(t, map[string]int{tc.wsA: tc.idA}).URL
				tc.urlB = systemFormsServer(t, map[string]int{tc.wsB: tc.idB}).URL
			} else {
				srv := systemFormsServer(t, map[string]int{tc.wsA: tc.idA, tc.wsB: tc.idB})
				tc.urlA, tc.urlB = srv.URL, srv.URL
			}
			sA := newGraphSyncer(tc.urlA, "t", tc.wsA)
			if !sA.isDashboardFormID(ctx, tc.idA) {
				t.Fatalf("%d should be workspace A's Dashboards form", tc.idA)
			}
			sB := newGraphSyncer(tc.urlB, "t", tc.wsB)
			if !sB.isDashboardFormID(ctx, tc.idB) {
				t.Errorf("%d (B's Dashboards) passed the guard after A's id was cached", tc.idB)
			}
			if sB.isDashboardFormID(ctx, tc.idA) {
				t.Errorf("%d (A's Dashboards) must not count as B's Dashboards form", tc.idA)
			}
		})
	}
}

// TestLookupSystemFormIDFallbackIgnoresCustomForms: the all-forms fallback also
// returns custom templates (type null); one titled "Dashboards" must neither be
// picked nor overwrite the cached system id.
func TestLookupSystemFormIDFallbackIgnoresCustomForms(t *testing.T) {
	clearChartFormCache()
	ctx := context.Background()
	var systemDown atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/forms/templates/system/"):
			if systemDown.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":71872,"title":"Dashboards","type":"system"}]}`))
		case strings.HasPrefix(r.URL.Path, "/forms/templates/"):
			// formTypes=all lists custom forms first (type NULLS FIRST).
			_, _ = w.Write([]byte(`{"data":[` +
				`{"id":900,"title":"Dashboards","type":null},` +
				`{"id":71872,"title":"Dashboards","type":"system"},` +
				`{"id":55,"title":"ActorFilters","type":"system"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// 1. System id cached via the system endpoint, then a lookup of another
	//    title goes through the fallback, which also lists the custom form.
	if id, err := lookupSystemFormID(ctx, "Dashboards", "ws", "t", srv.URL); err != nil || id != 71872 {
		t.Fatalf("Dashboards = %d, %v; want 71872", id, err)
	}
	systemDown.Store(true)
	if id, err := lookupSystemFormID(ctx, "ActorFilters", "ws", "t", srv.URL); err != nil || id != 55 {
		t.Fatalf("ActorFilters via fallback = %d, %v; want 55", id, err)
	}
	if id, _ := lookupSystemFormID(ctx, "Dashboards", "ws", "t", srv.URL); id != 71872 {
		t.Errorf("Dashboards after fallback = %d, want 71872 — the custom form overwrote the system id", id)
	}

	// 2. Cold cache, fallback only: the custom form is never picked.
	clearChartFormCache()
	if id, err := lookupSystemFormID(ctx, "Dashboards", "ws", "t", srv.URL); err != nil || id != 71872 {
		t.Errorf("Dashboards via fallback = %d, %v; want 71872 (not the custom 900)", id, err)
	}
	s := newGraphSyncer(srv.URL, "t", "ws")
	if s.isDashboardFormID(ctx, 900) {
		t.Error("the custom form titled Dashboards must not trip the push guard")
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
