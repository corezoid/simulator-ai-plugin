package sim

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// Review of v2.9.0: a saved snapshot wrote numbers in actor data as YAML strings, so after
// LoadGraph `self.price + self.cost` concatenated ("1020") and `count(price=10)` found nothing.
func TestGraphFileKeepsNumbersInData(t *testing.T) {
	g := layerGraph("L", []layerNode{{ID: "a", Title: "Shop", FormID: "7", FormTitle: "shop",
		Data: map[string]any{"price": json.Number("10"), "cost": json.Number("20.5"), "code": "10"}}}, nil)
	b, err := GraphFile(g, "")
	if err != nil {
		t.Fatal(err)
	}
	g2, err := LoadGraph(b)
	if err != nil {
		t.Fatal(err)
	}
	data := g2.actorIdx["a"].Data
	for _, k := range []string{"price", "cost", "_form_id"} {
		if v, _ := data.Get(k); !isRat(v) {
			t.Errorf("%s = %#v after round trip, want a number", k, v)
		}
	}
	if v, _ := data.Get("code"); v != "10" {
		t.Errorf("code = %#v, want the string \"10\"", v)
	}
	m, err := LoadModel([]byte("horizon: 0\nmetrics: {sum: self.price + self.cost, n: count(price=10)}\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := RunScenario(g2, m, ScenarioFrom(NewOMap()), nil, RunOptions{})
	if r.Status != "completed" || MetricText(r.Metrics["sum"]) != "30.5" || MetricText(r.Metrics["n"]) != "1" {
		t.Errorf("status %s sum %v n %v: %s", r.Status, r.Metrics["sum"], r.Metrics["n"], r.Error)
	}
}

func isRat(v any) bool { _, ok := v.(*big.Rat); return ok }

// Review of v2.9.0: a run stopped before the horizon fed metrics and goals and showed as a
// successful run (1/1, goal 100%) although no run completed.
func TestStoppedRunsAreNotStatistics(t *testing.T) {
	goals := NewOMap()
	goals.Set("few", "ticks < 5")
	s := newSummary("base", 3, goals)
	s.record(&RunResult{Status: "stopped_by_time", Error: "stopped at step 7: time limit 1s reached", Metrics: map[string]any{"ticks": ratInt(1)}})
	s.record(&RunResult{Status: "stopped_by_limit", Error: "max_steps 10 reached", Metrics: map[string]any{"ticks": ratInt(2)}})
	s.record(&RunResult{Status: "completed", Metrics: map[string]any{"ticks": ratInt(9)}})
	if s.Completed != 1 || s.Stopped != 2 || s.Failed != 0 {
		t.Fatalf("completed %d stopped %d failed %d", s.Completed, s.Stopped, s.Failed)
	}
	if v := s.values["ticks"]; len(v) != 1 || Canon(v[0]) != "9" {
		t.Errorf("ticks values %v, want only the completed run", v)
	}
	if s.Goals["few"] != [2]int{0, 1} {
		t.Errorf("goal few = %v, want [0 1]", s.Goals["few"])
	}
	if len(s.Errors) != 2 || !strings.Contains(s.Errors[0], "time limit") {
		t.Errorf("errors %v, want why the runs stopped", s.Errors)
	}
	if table := SummaryTable([]*Summary{s}); !strings.Contains(table, "| 1/3 (2 stopped) |") {
		t.Errorf("table:\n%s", table)
	}
}

// A goal that no run could evaluate shows as "no data", not as a missing column.
func TestGoalWithoutCompletedRunShowsNoData(t *testing.T) {
	goals := NewOMap()
	goals.Set("few", "ticks < 5")
	s := newSummary("base", 1, goals)
	s.record(&RunResult{Status: "stopped_by_time", Error: "stopped at step 0", Metrics: map[string]any{}})
	if table := SummaryTable([]*Summary{s}); !strings.Contains(table, "goal: few") || !strings.Contains(table, "| 0/1 (1 stopped) | no data |") {
		t.Errorf("table:\n%s", table)
	}
}

// A snapshot written by v2.9.0 holds its numbers as text; check says so instead of
// letting the run concatenate them.
func TestCheckWarnsAboutTextNumberSnapshot(t *testing.T) {
	model := []byte("horizon: 1\nmetrics: {n: count()}\n")
	old, err := LoadGraph([]byte("layerId: L\nactors:\n  - {id: a, title: A, data: {price: \"10\"}, sim: {type: t}}\nedges: []\n" +
		"sim: {format: sim-morrow/1, source: {kind: simulator, layer: L, taken_at: \"1790000000\"}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if w := strings.Join(Check(model, old, nil).Warnings, "\n"); !strings.Contains(w, "written by v2.9.0") {
		t.Errorf("no warning for a v2.9.0 snapshot: %s", w)
	}
	g := layerGraph("L", []layerNode{{ID: "a", Title: "A", FormTitle: "t", Data: map[string]any{"price": json.Number("10")}}}, nil)
	b, err := GraphFile(g, "")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := LoadGraph(b)
	if err != nil {
		t.Fatal(err)
	}
	if w := strings.Join(Check(model, fresh, nil).Warnings, "\n"); strings.Contains(w, "v2.9.0") {
		t.Errorf("warning for a current snapshot: %s", w)
	}
}

// Review of v2.9.0: opening the first account of a conserved type at 0 failed the step
// ("conserved totals changed within step:") although the total stayed 0.
func TestFirstZeroAccountOfConservedTypeKeepsTotal(t *testing.T) {
	g, err := LoadGraph([]byte("layerId: x\nactors:\n  - {id: s, title: Shop, sim: {type: shop}}\nedges: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel([]byte(`
value_types: {USD: {kind: decimal, scale: 2, conserved: true}}
horizon: 0
initial_events:
  - {event: open, target: {title: Shop}}
behaviors:
  shop:
    open:
      - {create: {type: wallet, title: W, accounts: [{name: cash, value_type: USD}]}}
metrics: {wallets: count(type='wallet')}
`))
	if err != nil {
		t.Fatal(err)
	}
	r := RunScenario(g, m, ScenarioFrom(NewOMap()), nil, RunOptions{})
	if r.Status != "completed" || MetricText(r.Metrics["wallets"]) != "1" {
		t.Errorf("status %s wallets %v: %s", r.Status, r.Metrics["wallets"], r.Error)
	}
}

func TestTotalsDiffTreatsMissingTypeAsZero(t *testing.T) {
	none, zero, one := map[string]*big.Rat{}, map[string]*big.Rat{"USD": new(big.Rat)}, map[string]*big.Rat{"USD": ratOne}
	if totalsDiff(none, zero) != "" || totalsDiff(zero, none) != "" {
		t.Error("missing type and 0 must be the same total")
	}
	if d := totalsDiff(one, none); d != " USD: -1" {
		t.Errorf("an account removed with its value: diff %q, want \" USD: -1\"", d)
	}
	if totalsDiff(none, one) == "" || totalsDiff(zero, one) == "" {
		t.Error("a changed total must be caught")
	}
}
