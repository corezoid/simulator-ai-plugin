package sim

import (
	"strings"
	"testing"
)

// A call with arguments the function does not take fails the run with an error, as in the
// reference engine; it must not panic or pass silently.
func TestWrongCallFailsTheRun(t *testing.T) {
	g, err := LoadGraph([]byte("layerId: x\nactors:\n  - {id: s, title: Shop, sim: {type: shop}}\nedges: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel([]byte("horizon: 0\nmetrics: {x: \"actor(type='wallet')\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := RunScenario(g, m, ScenarioFrom(NewOMap()), nil, RunOptions{})
	if r.Status != "failed" || !strings.Contains(r.Error, "metrics") {
		t.Errorf("status %s error %q, want a failed run with a metrics error", r.Status, r.Error)
	}
}
