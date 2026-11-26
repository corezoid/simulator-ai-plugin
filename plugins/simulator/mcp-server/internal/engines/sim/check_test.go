package sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func caseFiles(t *testing.T, name string) ([]byte, *Graph, []*OMap) {
	dir := filepath.Join("testdata", "conformance", name)
	model, _ := os.ReadFile(filepath.Join(dir, "model.yaml"))
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
	return model, g, raw
}

func TestConformanceModelsPassTheCheck(t *testing.T) {
	for _, name := range []string{"sample01", "warehouse", "process_flow", "edge_cases"} {
		m, g, sc := caseFiles(t, name)
		r := Check(m, g, sc)
		if !r.OK() || len(r.Warnings) > 0 {
			t.Errorf("%s: %v %v", name, r.Errors, r.Warnings)
		}
	}
}

func TestCheckFindsErrors(t *testing.T) {
	m, g, sc := caseFiles(t, "sample01")
	cases := map[string]string{
		"      - explode: {}\n":                                  "unknown action \"explode\"",
		"      - transfer: {amount: 1}\n":                        "missing field \"from\"",
		"      - set: {x: \"=params.pryce\"}\n":                  "params.pryce is not defined",
		"      - set: {x: \"=refs.boss\"}\n":                     "refs.boss is not defined",
		"      - set: {x: \"=1 +\"}\n":                           "syntax error",
		"      - set: {x: \"=magic(1)\"}\n":                      "unknown function \"magic\"",
		"      - add: {actor: self, account: cash, amount: 1}\n": "holds a conserved type",
		"      - schedule: {event: party, after: 1m}\n":          "\"party\" is emitted but no behavior handles it",
	}
	for insert, want := range cases {
		broken := strings.Replace(string(m), "      - release: {target: self}\n", "      - release: {target: self}\n"+insert, 1)
		r := Check([]byte(broken), g, sc)
		all := strings.Join(append(r.Errors, r.Warnings...), "\n")
		if !strings.Contains(all, want) {
			t.Errorf("inserting %q: want %q, got:\n%s", insert, want, all)
		}
	}
	bad := strings.Replace(string(m), "  company:  {title: Company}", "  company:  {title: Nobody}", 1)
	if r := Check([]byte(bad), g, sc); r.OK() || !strings.Contains(strings.Join(r.Errors, "\n"), "matches 0 actors") {
		t.Errorf("unresolved ref not reported: %v", r.Errors)
	}
}
