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

// formByIDServer serves GET /forms/{id} from a table of id → (type, title); any
// other id or route is a 404.
func formByIDServer(t *testing.T, forms map[int][2]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/forms/%d", &id); err == nil {
			if f, ok := forms[id]; ok {
				_, _ = fmt.Fprintf(w, `{"data":{"type":%q,"title":%q}}`, f[0], f[1])
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestIsDashboardFormID(t *testing.T) {
	srv := formByIDServer(t, map[int][2]string{
		71872: {"system", "Dashboards"},
		100:   {"system", "Clients"},
		900:   {"custom", "Dashboards"},
	})
	s := newGraphSyncer(srv.URL, "Simulator t", t.Name())
	if !s.isDashboardFormID(context.Background(), 71872) {
		t.Error("71872 (system Dashboards) should be recognized as the dashboard form")
	}
	if s.isDashboardFormID(context.Background(), 100) {
		t.Error("100 (Clients) must not be recognized as the dashboard form")
	}
	if s.isDashboardFormID(context.Background(), 900) {
		t.Error("900 (custom form titled Dashboards) must not trip the push guard")
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

// TestIsDashboardFormIDScopedPerBackend: form ids are only unique within one
// backend, so after set-environment the same id can name another form — the
// first backend's cached verdict must not leak to the second.
func TestIsDashboardFormIDScopedPerBackend(t *testing.T) {
	ctx := context.Background()
	a := formByIDServer(t, map[int][2]string{303: {"system", "Dashboards"}})
	b := formByIDServer(t, map[int][2]string{303: {"custom", "Clients"}})
	sA, sB := newGraphSyncer(a.URL, "t", "ws"), newGraphSyncer(b.URL, "t", "ws")
	for range 2 { // second round is served from the cache
		if !sA.isDashboardFormID(ctx, 303) {
			t.Error("303 is backend A's Dashboards form")
		}
		if sB.isDashboardFormID(ctx, 303) {
			t.Error("303 on backend B is Clients, not Dashboards")
		}
	}
}

// TestLookupSystemFormIDFallbackIgnoresCustomForms: should the fallback listing
// ever include custom templates (type null), one titled "Dashboards" must
// neither be picked nor overwrite the cached system id.
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
			// A listing that (unexpectedly) also carries custom forms, first.
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
}

// TestLookupSystemFormIDNoFallbackOnAuthError: both routes share the access
// check, so a 403 is returned as is instead of being retried on the fallback.
func TestLookupSystemFormIDNoFallbackOnAuthError(t *testing.T) {
	clearChartFormCache()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	_, err := lookupSystemFormID(context.Background(), "Dashboards", "ws", "t", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want the original 403", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("made %d requests, want 1 (no fallback on 403)", n)
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
