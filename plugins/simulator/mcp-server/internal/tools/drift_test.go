package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecDrift validates every curated operation against the backend's
// papi-openapi.json — the spec dumped by pong-server (`yarn dump-openapi`).
//
// It is the drift gate connecting the plugin's hand-declared operations to the
// backend contract: when the spec is dropped at testdata/papi-openapi.json, each
// curated op's (method, path, operationId) is checked against it. Until the spec
// is committed, the test skips (so it never blocks a fresh checkout).
//
// Path note: op.Path is relative to the /papi/1.0 base; the spec uses full paths
// with {param} placeholders and lowercase HTTP methods.
func TestSpecDrift(t *testing.T) {
	path := filepath.Join("testdata", "papi-openapi.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no %s — drop the backend spec here to enable the drift gate "+
			"(generate it in pong-server with `yarn dump-openapi`)", path)
	}

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	for _, op := range allOps() {
		full := "/papi/1.0" + op.Path
		methods, ok := spec.Paths[full]
		if !ok {
			t.Errorf("%s: path %q not found in backend spec", op.Name, full)
			continue
		}
		entry, ok := methods[strings.ToLower(op.Method)]
		if !ok {
			t.Errorf("%s: %s %s not present in backend spec", op.Name, op.Method, full)
			continue
		}
		if entry.OperationID != op.Name {
			t.Errorf("%s: backend operationId for %s %s is %q, want %q",
				op.Name, op.Method, full, entry.OperationID, op.Name)
		}
	}
}
