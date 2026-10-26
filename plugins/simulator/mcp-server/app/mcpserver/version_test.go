package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is the repository root, relative to this package dir
// (plugins/simulator/mcp-server/app/mcpserver).
const repoRoot = "../../../../.."

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

// TestEveryVersionFileInLockstep checks every file scripts/release.sh bumps,
// not only the canonical manifest: the v2.11.0 and v2.12.0 releases (cut by
// sim-deploy) left the root plugin.json at 2.10.0 and nothing failed. The list
// is read from release.sh itself so the two cannot diverge.
func TestEveryVersionFileInLockstep(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)VERSION_FILES="(.*?)"`).FindSubmatch(script)
	if block == nil {
		t.Fatal("VERSION_FILES not found in scripts/release.sh")
	}
	files := strings.Fields(string(block[1]))
	if len(files) < 7 {
		t.Fatalf("release.sh lists only %d version files: %v", len(files), files)
	}
	jsonVersion := regexp.MustCompile(`"version"\s*:\s*"([^"]+)"`)
	mdVersion := regexp.MustCompile(`(?m)^version:\s*(\S+)`)
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(repoRoot, f))
		if err != nil {
			t.Errorf("read %s: %v", f, err)
			continue
		}
		re := jsonVersion
		if strings.HasSuffix(f, ".md") {
			re = mdVersion
		}
		m := re.FindSubmatch(raw)
		if m == nil {
			t.Errorf("%s carries no version", f)
			continue
		}
		if v := string(m[1]); v != DefaultVersion {
			t.Errorf("%s is at %q but DefaultVersion is %q — every file in release.sh VERSION_FILES must move together", f, v, DefaultVersion)
		}
	}
}
