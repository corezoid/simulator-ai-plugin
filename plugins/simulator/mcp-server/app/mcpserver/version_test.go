package mcpserver

import (
	"encoding/json"
	"os"
	"testing"
)

// TestDefaultVersionMatchesManifest guards against the Go server reporting a
// serverInfo.version that has drifted from the plugin manifests (issue #89).
// scripts/release.sh bumps DefaultVersion in lockstep with the manifests; this
// test fails CI if a release — or a manual edit — ever leaves the const behind.
func TestDefaultVersionMatchesManifest(t *testing.T) {
	// Canonical version source, relative to this package dir
	// (plugins/simulator/mcp-server/app/mcpserver).
	const manifest = "../../../.claude-plugin/plugin.json"
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read %s: %v", manifest, err)
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse %s: %v", manifest, err)
	}
	if m.Version == "" {
		t.Fatalf("%s has no version field", manifest)
	}
	if DefaultVersion != m.Version {
		t.Errorf("DefaultVersion = %q but %s = %q — bump mcpserver.DefaultVersion "+
			"(scripts/release.sh does this at release) so serverInfo.version matches the manifests",
			DefaultVersion, manifest, m.Version)
	}
}
