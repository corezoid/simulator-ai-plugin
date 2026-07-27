//go:build live_smoke

package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestLiveCloneGraphObjectsByURL(t *testing.T) {
	if os.Getenv("LIVE_SIMULATOR_CLONE") != "1" {
		t.Skip("set LIVE_SIMULATOR_CLONE=1 to run the live clone smoke test")
	}
	// This is intentionally opt-in: it creates imported graph objects in the
	// configured workspace. Run it only against a disposable/test workspace.
	workDir := os.Getenv("SIMULATOR_WORK_DIR")
	if workDir == "" {
		t.Fatal("SIMULATOR_WORK_DIR is required")
	}
	loadLiveDotEnv(t, filepath.Join(workDir, ".env"))

	sourceURL := os.Getenv("LIVE_SIMULATOR_SOURCE_URL")
	if sourceURL == "" {
		t.Fatal("LIVE_SIMULATOR_SOURCE_URL is required")
	}
	baseURL := os.Getenv("SIMULATOR_API_BASE_URL")
	workspaceID := os.Getenv("WORKSPACE_ID")
	if baseURL == "" || workspaceID == "" || os.Getenv("ACCESS_TOKEN") == "" {
		t.Fatal("SIMULATOR_API_BASE_URL, WORKSPACE_ID and ACCESS_TOKEN must be set")
	}

	ecore.Configure(baseURL, false)
	ecore.ResetAuth()

	prefix := "codex_clone_" + time.Now().UTC().Format("20060102_150405") + "_"
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sourceUrl":        sourceURL,
		"refReplacePrefix": prefix,
		"confirmClone":     true,
		"waitSec":          360,
	}
	res, err := handleCloneGraphObjects(context.Background(), req)
	if err != nil {
		t.Fatalf("cloneGraphObjects returned error: %v", err)
	}
	if res == nil {
		t.Fatal("cloneGraphObjects returned nil result")
	}
	text := textResult(t, res)
	t.Logf("clone result: %s", text)
	if res.IsError {
		t.Fatalf("cloneGraphObjects returned tool error: %s", text)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("parse clone result: %v", err)
	}
	if out["sourceAccId"] != workspaceID || out["targetAccId"] != workspaceID {
		t.Fatalf("expected source and target to default to current workspace %s, got source=%v target=%v", workspaceID, out["sourceAccId"], out["targetAccId"])
	}
	if out["sourceKind"] != "layer" {
		t.Fatalf("expected URL auto-selection to choose layer, got sourceKind=%v", out["sourceKind"])
	}
	if out["countersMatch"] != true {
		t.Fatalf("expected export/import counters to match, got %v", out["countersMatch"])
	}
	target, _ := out["target"].(map[string]any)
	targetURL, _ := target["url"].(string)
	if targetURL == "" {
		t.Fatalf("expected target.url, got target=%v warnings=%v", target, out["warnings"])
	}
	if !strings.Contains(targetURL, "/actors_graph/") || !strings.Contains(targetURL, "/graph/") || !strings.Contains(targetURL, "/layers") {
		t.Fatalf("target.url does not look like a graph layer URL: %s", targetURL)
	}
	fmt.Println("LIVE_CLONE_TARGET_URL=" + targetURL)
}

func loadLiveDotEnv(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if _, exists := os.LookupEnv(key); !exists {
			t.Setenv(key, val)
		}
	}
}
