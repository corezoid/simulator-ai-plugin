package sim

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The first production use of the tools (2026-10-22, an agent modelling a game economy on the
// hosted server) found models that checked "ok" and ran "completed" with wrong numbers. Each
// case below is one of its calls, cut down.

func TestMain(m *testing.M) {
	os.Setenv("SIMULATOR_SIMULATION_LOG", "off") // quiet tests; the log test turns it on
	os.Exit(m.Run())
}

func checkText(t *testing.T, model string, g *Graph) *CheckReport {
	t.Helper()
	return Check([]byte(model), g, nil)
}

func has(list []string, part string) bool {
	return strings.Contains(strings.Join(list, "\n"), part)
}

func TestNotAModelIsAnErrorWithAHint(t *testing.T) {
	for _, src := range []string{"help", "", "42", "- a\n- b\n"} {
		r := checkText(t, src, nil)
		if r.OK() || !has(r.Errors, "not a model") || !has(r.Errors, FormatURL) {
			t.Errorf("model %q: errors %v, want 'not a model' with the format link", src, r.Errors)
		}
	}
}

func TestNestedUnknownKeysAreReported(t *testing.T) {
	r := checkText(t, `
horizon: 1d
actors:
  - {title: Farm, type: farm, behaviour: {every: 1d}}
initial_events:
  - {event: grow, target: {title: Farm}, evry: 1d}
value_types: {seeds: {kind: integer, conservd: true}}
behaviors:
  farm:
    grow:
      - add: {account: seeds, amount: 1, value_type: seeds, every: 1d}
metrics: {seeds: "actor(type='farm')"}
`, nil)
	for _, want := range []string{`actors[0]: unknown field "behaviour"`, `initial_events[0]: unknown field "evry"`,
		`value_types.seeds: unknown field "conservd"`, `unknown field "every" (ignored)`} {
		if !has(r.Warnings, want) {
			t.Errorf("no warning %q in %v", want, r.Warnings)
		}
	}
	if r := checkText(t, "horizon: 1d\nactors: {farm: {behaviour: {frobnicate: 1}}}\n", nil); !has(r.Errors, "actors: must be a list") {
		t.Errorf("actors as a mapping: errors %v", r.Errors)
	}
}

func TestExpressionAccountNameIsAnError(t *testing.T) {
	r := checkText(t, `
horizon: 1d
initial_events: [{event: learn, target: {title: Farm}}]
behaviors:
  farm:
    learn:
      - add: {account: "='k_' + self.condition", amount: 1}
metrics: {k: "actor(type='farm')"}
`, nil)
	if !has(r.Errors, "account names are literal") {
		t.Errorf("errors %v, want the literal account name error", r.Errors)
	}
}

func TestReadOfAnAccountNobodyHasIsAWarning(t *testing.T) {
	g, err := LoadGraph([]byte("layerId: x\nactors:\n  - {id: f, title: Farm, sim: {type: farm}}\nedges: []\n" +
		"sim: {valueTypes: {USD: {kind: decimal, scale: 2}}, accounts: [{actor_id: f, name: cash, value_type: USD, value: '5'}]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := checkText(t, `
horizon: 1d
refs: {farm: {title: Farm}}
initial_events: [{event: sell, target: farm}]
behaviors:
  farm:
    sell:
      - add: {account: sales, amount: 1, value_type: USD}
metrics:
  cash: "actor(refs.farm).acc('cash')"
  sales: "actor(refs.farm).acc('sales')"
  knowledge: "total('knowledge_total')"
`, g)
	if !has(r.Warnings, `account "knowledge_total" is read but no actor has it`) {
		t.Errorf("warnings %v, want one about knowledge_total", r.Warnings)
	}
	if has(r.Warnings, `"cash" is read`) || has(r.Warnings, `"sales" is read`) {
		t.Errorf("false warning for an existing or created account: %v", r.Warnings)
	}
}

// The published examples must stay free of the new warnings.
func TestDocExamplesPassTheCheck(t *testing.T) {
	dirs, _ := filepath.Glob("../../../../docs/simulation/examples/*/model.yaml")
	if len(dirs) == 0 {
		t.Skip("docs examples not found")
	}
	for _, m := range dirs {
		dir := filepath.Dir(m)
		model, _ := os.ReadFile(m)
		gsrc, _ := os.ReadFile(filepath.Join(dir, "graph.yaml"))
		g, err := LoadGraph(gsrc)
		if err != nil {
			t.Fatal(err)
		}
		ssrc, _ := os.ReadFile(filepath.Join(dir, "scenarios.yaml"))
		_, raw, err := LoadScenarios(ssrc)
		if err != nil {
			t.Fatal(err)
		}
		if r := Check(model, g, raw); !r.OK() || len(r.Warnings) > 0 {
			t.Errorf("%s: errors %v warnings %v", dir, r.Errors, r.Warnings)
		}
	}
}

// 2026-10-22: a model made only of helper actors could be checked but not run without a layer.
func TestModelOnlyRunNeedsNoLayer(t *testing.T) {
	out, isErr := callTool(t, handleRun, map[string]any{"model": `
value_types: {coins: {kind: integer}}
horizon: 3d
actors:
  - {title: Farm, type: farm, accounts: [{name: coins, value_type: coins, value: 0}]}
initial_events: [{event: harvest, target: farm, every: 1d}]
behaviors:
  farm:
    harvest:
      - add: {account: coins, amount: 2}
refs: {farm: {title: Farm}}
metrics: {coins: "actor(refs.farm).acc('coins')"}
`})
	if isErr || out["ok"] != true || !strings.Contains(fmt.Sprint(out["table"]), "| 8 |") {
		t.Fatalf("model-only run: %v", out)
	}
}

func TestRunLogLineHasNoModelContent(t *testing.T) {
	t.Setenv("SIMULATOR_SIMULATION_LOG", "on")
	var buf strings.Builder
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	callTool(t, handleRun, map[string]any{"model": "horizon: 1d\nparams: {secret_price: 12345}\nmetrics: {p: params.secret_price}\n"})
	line := buf.String()
	if !strings.Contains(line, `simulation {`) || !strings.Contains(line, `"outcome":"ok"`) || strings.Contains(line, "12345") {
		t.Errorf("log line %q: want outcome and no model values", line)
	}
}
