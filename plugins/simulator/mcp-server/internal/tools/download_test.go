package tools

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestBaseMIME(t *testing.T) {
	cases := map[string]string{
		"text/plain; charset=utf-8": "text/plain",
		"Application/JSON":          "application/json",
		"  image/png  ":             "image/png",
		"":                          "",
	}
	for in, want := range cases {
		if got := baseMIME(in); got != want {
			t.Errorf("baseMIME(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTextualContent(t *testing.T) {
	textual := []struct{ ct, name string }{
		{"text/plain", "a.txt"},
		{"text/csv", "a.csv"},
		{"application/json", "a.json"},
		{"application/vnd.api+json", "a"},
		{"application/atom+xml", "a"},
		{"image/svg+xml", "a.svg"},
		{"application/octet-stream", "notes.md"}, // generic ct → decide by extension
		{"", "main.go"},                          // missing ct → decide by extension
	}
	for _, c := range textual {
		if !isTextualContent(c.ct, c.name) {
			t.Errorf("isTextualContent(%q, %q) = false, want true", c.ct, c.name)
		}
	}
	binary := []struct{ ct, name string }{
		{"application/pdf", "a.pdf"},
		{"image/png", "a.png"},
		{"application/octet-stream", "blob.bin"}, // unknown extension stays binary
	}
	for _, c := range binary {
		if isTextualContent(c.ct, c.name) {
			t.Errorf("isTextualContent(%q, %q) = true, want false", c.ct, c.name)
		}
	}
}

func TestImageMIME(t *testing.T) {
	cases := []struct{ ct, name, want string }{
		{"image/png", "a.png", "image/png"},
		{"image/jpg", "a.jpg", "image/jpeg"},                 // normalised
		{"application/octet-stream", "a.jpeg", "image/jpeg"}, // generic ct → by extension
		{"", "a.webp", "image/webp"},
		{"application/pdf", "a.pdf", ""}, // not an image
		{"text/plain", "a.txt", ""},
	}
	for _, c := range cases {
		if got := imageMIME(c.ct, c.name); got != c.want {
			t.Errorf("imageMIME(%q, %q) = %q, want %q", c.ct, c.name, got, c.want)
		}
	}
}

func TestEncodeFilePath(t *testing.T) {
	cases := map[string]string{
		"a1b2c3d4.pdf":          "a1b2c3d4.pdf",
		"bucket/a1b2c3d4.pdf":   "bucket/a1b2c3d4.pdf", // slashes preserved
		"my report (1).txt":     "my%20report%20%281%29.txt",
		"bucket/space file.png": "bucket/space%20file.png",
	}
	for in, want := range cases {
		if got := encodeFilePath(in); got != want {
			t.Errorf("encodeFilePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// newDownloadServer serves fixed bytes with a content type and records the path.
func newDownloadServer(t *testing.T, contentType string, body []byte, gotPath *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func callReadAttachment(t *testing.T, srvURL, fileName string) *mcp.CallToolResult {
	t.Helper()
	c := apiclient.New(srvURL, "ws", func() (string, error) { return "Simulator t", nil }, false)
	var req mcp.CallToolRequest
	req.Params.Name = "readAttachment"
	req.Params.Arguments = map[string]any{"fileName": fileName}
	res, err := readAttachmentHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("readAttachmentHandler error: %v", err)
	}
	return res
}

func TestReadAttachmentText(t *testing.T) {
	var path string
	srv := newDownloadServer(t, "text/plain; charset=utf-8", []byte("hello world"), &path)
	defer srv.Close()

	res := callReadAttachment(t, srv.URL, "bucket/notes.txt")
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	if want := "/download/bucket/notes.txt"; path != want {
		t.Errorf("request path = %q, want %q", path, want)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not text: %+v", res.Content[0])
	}
	if tc.Text != "hello world" {
		t.Errorf("text = %q, want %q", tc.Text, "hello world")
	}
}

func TestReadAttachmentImage(t *testing.T) {
	var path string
	raw := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	srv := newDownloadServer(t, "image/png", raw, &path)
	defer srv.Close()

	res := callReadAttachment(t, srv.URL, "logo.png")
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	var img mcp.ImageContent
	var found bool
	for _, ct := range res.Content {
		if ic, ok := ct.(mcp.ImageContent); ok {
			img, found = ic, true
		}
	}
	if !found {
		t.Fatalf("no image content in result: %+v", res.Content)
	}
	if img.MIMEType != "image/png" {
		t.Errorf("image mime = %q, want image/png", img.MIMEType)
	}
	if img.Data != base64.StdEncoding.EncodeToString(raw) {
		t.Errorf("image data not the base64 of the body")
	}
}

func TestReadAttachmentBinaryResource(t *testing.T) {
	var path string
	raw := []byte("%PDF-1.4 fake")
	srv := newDownloadServer(t, "application/pdf", raw, &path)
	defer srv.Close()

	res := callReadAttachment(t, srv.URL, "doc.pdf")
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	var blob mcp.BlobResourceContents
	var found bool
	for _, ct := range res.Content {
		if er, ok := ct.(mcp.EmbeddedResource); ok {
			if b, ok := er.Resource.(mcp.BlobResourceContents); ok {
				blob, found = b, true
			}
		}
	}
	if !found {
		t.Fatalf("no embedded blob resource in result: %+v", res.Content)
	}
	if blob.MIMEType != "application/pdf" {
		t.Errorf("blob mime = %q, want application/pdf", blob.MIMEType)
	}
	if blob.Blob != base64.StdEncoding.EncodeToString(raw) {
		t.Errorf("blob data not the base64 of the body")
	}
}

func TestReadAttachmentMissingFileName(t *testing.T) {
	c := apiclient.New("http://unused.example", "ws", func() (string, error) { return "", nil }, false)
	var req mcp.CallToolRequest
	req.Params.Name = "readAttachment"
	req.Params.Arguments = map[string]any{"fileName": "  "}
	res, err := readAttachmentHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result for blank fileName, got: %+v", res.Content)
	}
	tc, _ := res.Content[0].(mcp.TextContent)
	if !strings.Contains(tc.Text, "fileName") {
		t.Errorf("error text = %q, want mention of fileName", tc.Text)
	}
}
