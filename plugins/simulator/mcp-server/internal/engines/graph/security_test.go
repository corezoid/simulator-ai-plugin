package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestReadLocalImageAllowList confirms the localPath source only reads
// image-typed files, narrowing the arbitrary-file-read surface.
func TestReadLocalImageAllowList(t *testing.T) {
	for _, p := range []string{"/etc/passwd", "/home/u/.ssh/id_rsa", "secret.txt", "archive.zip", "noext"} {
		if _, err := readLocalImage(p); err == nil {
			t.Errorf("readLocalImage(%q) should reject a non-image extension", p)
		} else if !strings.Contains(err.Error(), "image file") {
			t.Errorf("readLocalImage(%q) rejected for the wrong reason: %v", p, err)
		}
	}
	// An allowed extension that does not exist on disk must fail at stat, not
	// at the extension check (proves the allow-list let it through).
	if _, err := readLocalImage("/nonexistent/path/avatar.png"); err == nil {
		t.Error("expected a stat error for a missing .png file")
	} else if strings.Contains(err.Error(), "image file") {
		t.Errorf(".png was rejected by the extension check: %v", err)
	}
}

// TestPushGraphFileRejectsTraversalLayerID is a focused regression test: the
// handler must reject a non-UUID layerId before any filesystem access.
func TestPushGraphFileRejectsTraversalLayerID(t *testing.T) {
	// ensureAuth requires a token; set one so we reach the layerId validation.
	t.Setenv("ACCESS_TOKEN", "test-token")
	ecore.Cfg.Authorization = "Simulator test-token"

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"layerId": "../../../../tmp/evil"}

	res, err := handlePushGraphFile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected an error result for a traversal layerId")
	}
}
