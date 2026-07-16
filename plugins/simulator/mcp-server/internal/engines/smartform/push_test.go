package smartform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestDefaultMimeType locks in the MIME-type derivation for every path
// pattern the Smart Form file tree can produce. The key regression case is
// pages/<page>/style, which was incorrectly typed as application/json before
// this fix and could not be corrected by a content-only PUT.
func TestDefaultMimeType(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// ── styles/ tree ─────────────────────────────────────────────────
		{"styles", "text/css"},
		{"styles/main.less", "text/css"},
		{"styles/components/button.less", "text/css"},

		// ── pages/<page>/style regression ────────────────────────────────
		// The file is named exactly "style" (no extension) but is Less/CSS.
		// Old code returned application/json here.
		{"pages/credit-application/style", "text/css"},
		{"pages/index/style", "text/css"},
		{"pages/my-page/style", "text/css"},

		// ── explicit .css extension ───────────────────────────────────────
		{"components/button.css", "text/css"},
		{"pages/landing/custom.css", "text/css"},

		// ── application/json (everything else) ───────────────────────────
		{"pages/credit-application/config", "application/json"},
		{"pages/credit-application/locale", "application/json"},
		{"pages/credit-application/viewModel", "application/json"},
		{"viewModel", "application/json"},
		{"locale", "application/json"},
		{"definitions/types", "application/json"},
		{"widgets/counter/config", "application/json"},
		// "style" at root (no pages/ prefix) is not a CSS file
		{"style", "application/json"},
		// "style" nested elsewhere is not CSS
		{"definitions/style", "application/json"},
	}

	for _, tc := range cases {
		got := defaultMimeType(tc.path)
		if got != tc.want {
			t.Errorf("defaultMimeType(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestDetectPullConflicts verifies that detectPullConflicts correctly
// identifies local files whose content has been modified since the last pull
// (hash differs from manifest) and ignores files that are unchanged or absent.
func TestDetectPullConflicts(t *testing.T) {
	dir := t.TempDir()

	// Helper: write file + update hash table
	writeFile := func(relPath, content string) {
		full := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	// Helper: write a manifest with given (path → hash) entries.
	writeManifest := func(hashes map[string]string) {
		files := make(map[string]manifestNode, len(hashes))
		for p, h := range hashes {
			files[p] = manifestNode{Hash: h}
		}
		m := smartFormManifest{Files: files}
		b, _ := json.MarshalIndent(m, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, manifestFileName), b, 0600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}

	t.Run("no manifest → no conflicts (first pull)", func(t *testing.T) {
		tmpDir := t.TempDir()
		if got := detectPullConflicts(tmpDir); len(got) != 0 {
			t.Errorf("expected nil/empty, got %v", got)
		}
	})

	t.Run("all files unchanged → no conflicts", func(t *testing.T) {
		content := "body { color: red; }"
		relPath := "pages/home/style"
		writeFile(relPath, content)
		writeManifest(map[string]string{
			relPath: hashSource(content),
		})
		if got := detectPullConflicts(dir); len(got) != 0 {
			t.Errorf("expected no conflicts, got %v", got)
		}
	})

	t.Run("modified file → flagged as conflict", func(t *testing.T) {
		relPath := "pages/home/config"
		writeFile(relPath, `{"title":"v1"}`)
		// Manifest records the original hash
		writeManifest(map[string]string{
			relPath: hashSource(`{"title":"original"}`),
		})
		got := detectPullConflicts(dir)
		if len(got) != 1 || got[0] != relPath {
			t.Errorf("expected [%q], got %v", relPath, got)
		}
	})

	t.Run("missing local file → not a conflict", func(t *testing.T) {
		// File is in manifest but not on disk — the pull will restore it;
		// that is expected behaviour, not a conflict.
		writeManifest(map[string]string{
			"pages/missing/config": hashSource("original"),
		})
		if got := detectPullConflicts(dir); len(got) != 0 {
			t.Errorf("expected no conflicts for missing file, got %v", got)
		}
	})

	t.Run("mixed: one modified + one unchanged → only modified listed", func(t *testing.T) {
		clean := "pages/clean/style"
		dirty := "pages/dirty/style"
		cleanContent := "p { margin: 0; }"
		writeFile(clean, cleanContent)
		writeFile(dirty, "p { margin: 8px; }") // edited locally
		writeManifest(map[string]string{
			clean: hashSource(cleanContent),
			dirty: hashSource("p { margin: 0; }"), // manifest has original
		})
		got := detectPullConflicts(dir)
		sort.Strings(got)
		if len(got) != 1 || got[0] != dirty {
			t.Errorf("expected [%q], got %v", dirty, got)
		}
	})
}
