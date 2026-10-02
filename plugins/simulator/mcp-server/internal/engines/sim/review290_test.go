package sim

import (
	"context"
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

// errAfter reports no error for the first n calls to Err, then context.Canceled.
type errAfter struct {
	context.Context
	n int
}

func (c *errAfter) Err() error {
	if c.n--; c.n >= 0 {
		return nil
	}
	return context.Canceled
}

// Review of v2.9.0: a run stopped before the horizon fed metrics and goals and showed as a
// successful run (1/1, goal 100%) although no run completed.
func TestStoppedRunsAreNotStatistics(t *testing.T) {
	g := tickGraph(t)
	m, err := LoadModel([]byte(strings.Replace(tickModel, "self_acc_placeholder", `"actor(refs.clock).acc('ticks')"`, 1) +
		"refs: {clock: {title: Clock}}\ngoals: {few: ticks < 5}\n"))
	if err != nil {
		t.Fatal(err)
	}
	// RunMany's check passes, the engine stops at step 0, the next RunMany check ends the loop
	s := RunMany(&errAfter{context.Background(), 1}, g, m, ScenarioFrom(NewOMap()), 5, nil, NewOMap())
	if s.Runs != 1 || s.Completed != 0 || s.Stopped != 1 || s.Failed != 0 {
		t.Fatalf("runs %d completed %d stopped %d failed %d", s.Runs, s.Completed, s.Stopped, s.Failed)
	}
	if len(s.Metrics) != 0 || s.Goals["few"] != [2]int{} {
		t.Errorf("stopped run counted: metrics %v goals %v", s.Metrics, s.Goals)
	}
	if len(s.Errors) == 0 || !strings.Contains(s.Errors[0], "stopped at step 0") {
		t.Errorf("errors %v, want why the run stopped", s.Errors)
	}
	table := SummaryTable([]*Summary{s})
	if !strings.Contains(table, "| 0/1 (1 stopped) |") {
		t.Errorf("table:\n%s", table)
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

func TestSameTotalsTreatsMissingTypeAsZero(t *testing.T) {
	none, zero, one := map[string]*big.Rat{}, map[string]*big.Rat{"USD": new(big.Rat)}, map[string]*big.Rat{"USD": ratOne}
	if !sameTotals(none, zero) || !sameTotals(zero, none) {
		t.Error("missing type and 0 must be the same total")
	}
	if sameTotals(none, one) || sameTotals(one, none) || sameTotals(zero, one) {
		t.Error("a changed total must be caught")
	}
}
