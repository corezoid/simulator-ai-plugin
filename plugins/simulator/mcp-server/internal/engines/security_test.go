package engines

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestRequireUUIDRejectsTraversal locks in the guard that stops path-traversal
// and URL-injection via tool arguments that are interpolated into file paths
// (<layerId>.yaml) and API URLs.
func TestRequireUUIDRejectsTraversal(t *testing.T) {
	good := "8e86c99d-ffda-40a4-acd9-eef94418d4a9"
	if r := requireUUID("layerId", good); r != nil {
		t.Fatalf("valid UUID %q was rejected", good)
	}
	bad := []string{
		"../../../../etc/passwd",
		"../../secret",
		"id?admin=1",
		"id/../../x",
		"foo#frag",
		"",
		"not-a-uuid",
	}
	for _, v := range bad {
		if r := requireUUID("layerId", v); r == nil {
			t.Errorf("malicious/invalid value %q was accepted", v)
		}
	}
}

// TestSegEscapesPathSeparators verifies that IDs from a graph file are escaped
// before being placed in a URL path segment, so a "/" or "?" cannot alter the
// request path or inject query parameters.
func TestSegEscapesPathSeparators(t *testing.T) {
	cases := map[string]bool{
		"8e86c99d-ffda-40a4-acd9-eef94418d4a9": false, // a real UUID must be unchanged
		"a/b":      true,
		"a?x=1":    true,
		"a#frag":   true,
		"../../x":  true,
	}
	for in, mustChange := range cases {
		out := seg(in)
		if mustChange && out == in {
			t.Errorf("seg(%q) = %q: separators not escaped", in, out)
		}
		if !mustChange && out != in {
			t.Errorf("seg(%q) = %q: a plain UUID should be unchanged", in, out)
		}
		if strings.ContainsAny(out, "/?#") {
			t.Errorf("seg(%q) = %q still contains a path/query metacharacter", in, out)
		}
	}
}

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
	Cfg.Authorization = "Simulator test-token"

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
