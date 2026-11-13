package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

const tickModel = `
horizon: 30d
initial_events:
  - {event: tick, target: {title: Clock}, every: 1s}
behaviors:
  clock:
    tick:
      - {add: {account: ticks, amount: 1}}
metrics:
  ticks: self_acc_placeholder
`

func tickGraph(t *testing.T) *Graph {
	t.Helper()
	g, err := LoadGraph([]byte(`layerId: x
actors:
  - {id: c, title: Clock, sim: {type: clock}}
edges: []
sim:
  valueTypes: {n: {kind: integer}}
  accounts:
    - {actor_id: c, name: ticks, value_type: n, value: "0"}
`))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func tickModelFor(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel([]byte(strings.Replace(tickModel, "self_acc_placeholder", `"actor(refs.clock).acc('ticks')"`, 1) +
		"refs: {clock: {title: Clock}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Review of #111: `every: 1s` over 30 days queued 2.6M events up front. The queue must
// hold only the next occurrence, while pending_events still counts the whole rest.
func TestRecurringEventQueueStaysBounded(t *testing.T) {
	g, m := tickGraph(t), tickModelFor(t)
	e, err := NewEngine(g.clone(), m, ScenarioFrom(NewOMap()), nil)
	if err != nil {
		t.Fatal(err)
	}
	e.MaxSteps = 1000
	r := e.Run()
	if r.Status != "stopped_by_limit" || r.Steps != 1000 {
		t.Fatalf("status %s steps %d: %s", r.Status, r.Steps, r.Error)
	}
	if e.queue.Len() > 1 {
		t.Errorf("queue holds %d events, want 1", e.queue.Len())
	}
	if want := 30*86400 + 1 - 1000; r.PendingEvents != want {
		t.Errorf("pending_events = %d, want %d", r.PendingEvents, want)
	}
	if got := MetricText(r.Metrics["ticks"]); got != "1000" {
		t.Errorf("ticks = %s, want 1000", got)
	}
	src := []byte(strings.Replace(tickModel, "self_acc_placeholder", `"actor(refs.clock).acc('ticks')"`, 1) + "refs: {clock: {title: Clock}}\n")
	rep := Check(src, g, nil)
	if len(rep.Warnings) == 0 || !strings.Contains(strings.Join(rep.Warnings, "\n"), "2,592,001 occurrences") {
		t.Errorf("check should warn about the step limit, got %v", rep.Warnings)
	}
}

func TestRunStopsWhenContextIsDone(t *testing.T) {
	g, m := tickGraph(t), tickModelFor(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("time limit 1s reached"))
	r := RunScenario(g, m, ScenarioFrom(NewOMap()), nil, RunOptions{Ctx: ctx})
	if r.Status != "stopped_by_time" || r.Steps != 0 || !strings.Contains(r.Error, "time limit 1s reached") {
		t.Errorf("status %s steps %d error %q", r.Status, r.Steps, r.Error)
	}
	s := RunMany(ctx, g, m, ScenarioFrom(NewOMap()), 50, nil, NewOMap())
	if s.Runs != 0 || s.Requested != 50 || !strings.Contains(s.Note, "stopped after 0 of 50 runs") {
		t.Errorf("runs %d/%d note %q", s.Runs, s.Requested, s.Note)
	}
}

func TestRunLogIsCapped(t *testing.T) {
	g, m := tickGraph(t), tickModelFor(t)
	sc := ScenarioFrom(NewOMap())
	sc.Horizon = ratInt(100)
	r := RunScenario(g, m, sc, nil, RunOptions{LogLimit: 5})
	if r.Steps != 101 || len(r.Log) != 5 {
		t.Errorf("steps %d log %d, want 101 and 5", r.Steps, len(r.Log))
	}
}

// Review of #111: the layer endpoints apply LIMIT before dropping deleted elements, so a
// short page is not the last one; an actor whose accounts are not readable is skipped.
func TestSnapshotReadsPastShortPageAndSkipsUnreadable(t *testing.T) {
	node := func(i int) map[string]any {
		return map[string]any{"id": fmt.Sprintf("n%03d", i), "title": fmt.Sprint("N", i), "formTitle": "t"}
	}
	var page1, page2 []map[string]any
	for i := 0; i < 49; i++ { // 50 asked, one deleted actor filtered out by the server
		page1 = append(page1, node(i))
	}
	for i := 49; i < 60; i++ {
		page2 = append(page2, node(i))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var data any = []any{}
		switch {
		case strings.HasPrefix(r.URL.Path, "/graph_layers/paginated/") && q.Get("type") == "nodes":
			switch q.Get("offset") {
			case "0":
				data = page1
			case "50":
				data = page2
			}
		case strings.HasPrefix(r.URL.Path, "/accounts/"):
			if q.Get("highPrecision") != "true" {
				t.Errorf("accounts read without highPrecision: %s", r.URL)
			}
			if strings.HasSuffix(r.URL.Path, "/n007") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			if q.Get("offset") == "0" && strings.HasSuffix(r.URL.Path, "/n001") {
				data = []any{map[string]any{"accountName": "cash", "currencyName": "UAH", "currencyId": 1, "nameId": "a",
					"amount": "1234567890123456.12345678", "incomeType": "credit", "type": "fact"}}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	ctx := apiclient.WithAuthorization(apiclient.WithBaseURL(context.Background(), srv.URL), "Simulator test")
	g, err := Snapshot(ctx, "layer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.actors) != 60 {
		t.Errorf("actors = %d, want 60 (the short first page is not the last)", len(g.actors))
	}
	if v, _ := g.Source.Get("accounts_not_readable"); fmt.Sprint(v) != "[N7]" {
		t.Errorf("accounts_not_readable = %v, want [N7]", v)
	}
	if v := g.accIdx[accKey{"n001", "cash"}].Value; Canon(v) != "1234567890123456.12345678" {
		t.Errorf("cash = %s, want the exact high-precision amount", Canon(v))
	}
}

// Review of #111: aliases are expanded by hand, so yaml.v3's alias-bomb guard never runs.
func TestYAMLAliasBombIsRefused(t *testing.T) {
	src := "a: &a [x, x, x, x, x, x, x, x, x, x]\n"
	prev := "a"
	for _, n := range []string{"b", "c", "d", "e", "f"} {
		src += fmt.Sprintf("%s: &%s [*%s, *%s, *%s, *%s, *%s, *%s, *%s, *%s, *%s, *%s]\n", n, n, prev, prev, prev, prev, prev, prev, prev, prev, prev, prev)
		prev = n
	}
	if _, err := parseYAML([]byte(src)); err == nil || !strings.Contains(err.Error(), "aliases expand to too many nodes") {
		t.Fatalf("err = %v, want the alias limit", err)
	}
	if _, err := parseYAML([]byte("base: &b {x: 1}\nuse: [*b, *b]\n")); err != nil {
		t.Errorf("a small alias must still work: %v", err)
	}
}

// Review of #115: a server that ignored offset would make "read until an empty page" loop.
func TestPagedReadStopsAtThePageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "same"}}})
	}))
	defer srv.Close()
	prev := maxPages
	maxPages = 5
	defer func() { maxPages = prev }()
	ctx := apiclient.WithAuthorization(apiclient.WithBaseURL(context.Background(), srv.URL), "Simulator test")
	if _, err := readPaged[layerNode](ctx, "/graph_layers/paginated/x", nil, 1, true); err == nil || !strings.Contains(err.Error(), "more than 5 pages") {
		t.Errorf("err = %v, want the page cap", err)
	}
}
