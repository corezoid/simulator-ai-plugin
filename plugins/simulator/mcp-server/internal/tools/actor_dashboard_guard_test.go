package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// guardClient builds a client pointed at a mock backend. Each mock gets a unique
// httptest URL, and both package-global caches (formTitleCache,
// dashboardFormCache) are keyed by base URL, so entries never leak between tests.
func guardClient(t *testing.T, h http.HandlerFunc) *apiclient.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return apiclient.New(srv.URL, "WS", func() (string, error) { return "t", nil }, false)
}

// formHandler serves GET /forms/{id} from a table of id→(type,title), plus
// GET /forms/templates/{accId} (title→id) for formName resolution. An id absent
// from the table (or formFail) returns 500 so the fail-open path can be exercised.
func formHandler(byID map[int][2]string, byTitle map[string]int, failForm int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/forms/templates/"):
			var b strings.Builder
			b.WriteString(`{"data":[`)
			first := true
			for title, id := range byTitle {
				if !first {
					b.WriteString(",")
				}
				first = false
				fmt.Fprintf(&b, `{"id":%d,"title":%q}`, id, title)
			}
			b.WriteString(`]}`)
			_, _ = w.Write([]byte(b.String()))
		case strings.HasPrefix(r.URL.Path, "/forms/"):
			var id int
			fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/forms/"), "%d", &id)
			tt, ok := byID[id]
			if !ok || id == failForm {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
				return
			}
			fmt.Fprintf(w, `{"data":{"type":%q,"title":%q}}`, tt[0], tt[1])
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}
}

// ---- createActor ----

func TestGuardActorCreate_BlocksDashboardsForm(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{243: {"system", "Dashboards"}}, nil, 0))
	err := guardActorCreate(context.Background(), map[string]any{"formId": float64(243), "data": map[string]any{}}, c)
	if err == nil || !strings.Contains(err.Error(), "createChart") {
		t.Fatalf("expected Dashboards create blocked with a createChart hint, got: %v", err)
	}
}

func TestGuardActorCreate_AllowsOrdinaryForm(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{100: {"custom", "Clients"}}, nil, 0))
	if err := guardActorCreate(context.Background(), map[string]any{"formId": float64(100), "data": map[string]any{}}, c); err != nil {
		t.Fatalf("ordinary form create must pass, got: %v", err)
	}
}

// Point 1: a custom form that merely shares the title "Dashboards" must NOT trip
// the guard — only the system form does.
func TestGuardActorCreate_AllowsCustomFormNamedDashboards(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{900: {"custom", "Dashboards"}}, nil, 0))
	if err := guardActorCreate(context.Background(), map[string]any{"formId": float64(900), "data": map[string]any{}}, c); err != nil {
		t.Fatalf("custom form titled Dashboards must pass (not the system form), got: %v", err)
	}
}

func TestGuardActorCreate_BlocksDashboardsByName(t *testing.T) {
	c := guardClient(t, formHandler(
		map[int][2]string{243: {"system", "Dashboards"}},
		map[string]int{"Dashboards": 243},
		0,
	))
	if err := guardActorCreate(context.Background(), map[string]any{"formName": "Dashboards", "data": map[string]any{}}, c); err == nil {
		t.Fatal("expected createActor with formName=Dashboards to be blocked")
	}
}

func TestGuardActorCreate_FailsOpenWhenFormUnreadable(t *testing.T) {
	c := guardClient(t, formHandler(nil, nil, 243)) // GET /forms/243 -> 500
	if err := guardActorCreate(context.Background(), map[string]any{"formId": float64(243), "data": map[string]any{}}, c); err != nil {
		t.Fatalf("unresolvable form must fail open, got: %v", err)
	}
}

// The same formId names different forms on different backends (dev/pre/prod keep
// separate id sequences), so a cached verdict must not survive set-environment.
func TestGuardActorCreate_CacheKeyedByBaseURL(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{243: {"system", "Dashboards"}}, nil, 0))
	args := func() map[string]any { return map[string]any{"formId": float64(243), "data": map[string]any{}} }
	if err := guardActorCreate(context.Background(), args(), c); err == nil {
		t.Fatal("expected Dashboards create blocked on the first backend")
	}

	other := httptest.NewServer(formHandler(map[int][2]string{243: {"custom", "Clients"}}, nil, 0))
	t.Cleanup(other.Close)
	c.SetBaseURL(other.URL) // what set-environment does
	if err := guardActorCreate(context.Background(), args(), c); err != nil {
		t.Fatalf("formId 243 is an ordinary form on the second backend, must pass, got: %v", err)
	}
}

// ---- updateActor ----

func TestGuardActorUpdate_AllowsMetadataOnlyEditOnDashboard(t *testing.T) {
	// No source in the payload → allowed even on a Dashboards actor (fixes the
	// U6 lock-out), and no form fetch is even needed.
	c := guardClient(t, formHandler(map[int][2]string{243: {"system", "Dashboards"}}, nil, 0))
	args := map[string]any{"formId": float64(243), "actorId": "a1", "title": "renamed"}
	if err := guardActorUpdate(context.Background(), args, c); err != nil {
		t.Fatalf("metadata-only edit on a dashboard must pass, got: %v", err)
	}
}

func TestGuardActorUpdate_AllowsClearingSourceOnDashboard(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{243: {"system", "Dashboards"}}, nil, 0))
	for _, empty := range []any{nil, "", "   ", "{}", `""`, "[]", map[string]any{}, []any{}} {
		args := map[string]any{"formId": float64(243), "actorId": "a1", "data": map[string]any{"source": empty}}
		if err := guardActorUpdate(context.Background(), args, c); err != nil {
			t.Fatalf("clearing source (%#v) on a dashboard must pass, got: %v", empty, err)
		}
	}
}

func TestGuardActorUpdate_BlocksWritingSourceOnDashboard(t *testing.T) {
	c := guardClient(t, formHandler(map[int][2]string{243: {"system", "Dashboards"}}, nil, 0))
	// Both the JSON-string form createChart uses and a raw object must be blocked.
	for _, src := range []any{`{"chartType":"line","sourceType":"actorFilter"}`, map[string]any{"chartType": "bar"}} {
		args := map[string]any{"formId": float64(243), "actorId": "a1", "data": map[string]any{"source": src}}
		if err := guardActorUpdate(context.Background(), args, c); err == nil {
			t.Fatalf("writing a non-empty source (%#v) on a dashboard must be blocked", src)
		}
	}
}

func TestGuardActorUpdate_AllowsSourceOnOrdinaryForm(t *testing.T) {
	// A non-Dashboards form may have its own field literally named "source".
	c := guardClient(t, formHandler(map[int][2]string{100: {"custom", "Clients"}}, nil, 0))
	args := map[string]any{"formId": float64(100), "actorId": "a1", "data": map[string]any{"source": `{"x":1}`}}
	if err := guardActorUpdate(context.Background(), args, c); err != nil {
		t.Fatalf("writing source on an ordinary form must pass, got: %v", err)
	}
}

func TestGuardActorUpdate_FailsOpenWhenFormUnreadable(t *testing.T) {
	c := guardClient(t, formHandler(nil, nil, 243))
	args := map[string]any{"formId": float64(243), "actorId": "a1", "data": map[string]any{"source": `{"chartType":"line"}`}}
	if err := guardActorUpdate(context.Background(), args, c); err != nil {
		t.Fatalf("unresolvable form must fail open, got: %v", err)
	}
}

// ---- source-content helper ----

func TestValueHasContent(t *testing.T) {
	empty := []any{nil, "", "   ", "{}", "[]", `""`, "null", `"{}"`, map[string]any{}, []any{}}
	for _, v := range empty {
		if valueHasContent(v) {
			t.Errorf("expected %#v to be empty", v)
		}
	}
	full := []any{`{"chartType":"line"}`, "plain text", map[string]any{"a": 1}, []any{1}, 42, true}
	for _, v := range full {
		if !valueHasContent(v) {
			t.Errorf("expected %#v to have content", v)
		}
	}
}
