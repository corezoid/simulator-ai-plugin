package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// evalScenario is one natural-language task and the tool sequence a correct run
// is expected to use (at minimum).
type evalScenario struct {
	Name   string   `json:"name"`
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
}

// knownToolNames is the full set of tool names the server registers: curated
// API operations + the engine tools + the auth helpers. Engine/auth names are
// listed explicitly because they are registered outside allOps().
func knownToolNames() map[string]bool {
	m := map[string]bool{"login": true, "set-workspace": true}
	for _, op := range allOps() {
		m[op.Name] = true
	}
	for _, n := range []string{
		"pullGraphFile", "pushGraphFile", "getAllLayerPlacements",
		"compactGraphLayout", "pruneLongEdges", "createChart",
		"uploadActorPicture", "uploadActorPictureBulk",
	} {
		m[n] = true
	}
	return m
}

// TestEvalScenariosReferenceRealTools is the structural half of the eval harness:
// it guarantees every tool named in the golden scenarios actually exists in the
// registered tool set. If a tool is renamed or dropped, the affected scenario
// fails here — so the eval corpus can never drift away from the real surface.
//
// The behavioural half (drive a model through each prompt against a throwaway
// workspace and assert it issues at least these tools) runs with an LLM + a live
// server and is intentionally out of this unit test.
func TestEvalScenariosReferenceRealTools(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "eval-scenarios.json"))
	if err != nil {
		t.Fatalf("read eval scenarios: %v", err)
	}
	var scenarios []evalScenario
	if err := json.Unmarshal(data, &scenarios); err != nil {
		t.Fatalf("parse eval scenarios: %v", err)
	}
	if len(scenarios) == 0 {
		t.Fatal("no eval scenarios defined")
	}

	known := knownToolNames()
	for _, s := range scenarios {
		if len(s.Tools) == 0 {
			t.Errorf("scenario %q lists no tools", s.Name)
		}
		for _, tool := range s.Tools {
			if !known[tool] {
				t.Errorf("scenario %q references unknown tool %q", s.Name, tool)
			}
		}
	}
}
