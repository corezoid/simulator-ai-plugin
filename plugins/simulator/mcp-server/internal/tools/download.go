package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// readAttachment downloads a stored file (attachment) by its storage `fileName`
// and returns its content in a form the model can actually consume:
//   - textual files (text/*, JSON/XML/YAML/CSV/source code, SVG) → inline text;
//   - images (png/jpeg/gif/webp) → an image content block the model can see;
//   - PDFs and other binary → an embedded resource (base64 blob) the client renders.
//
// It is the read counterpart to getAttachments / getActorAttachments (which list
// metadata, including the `fileName`) and uploadBase64 (which stores files). The
// download goes through the PAPI download route (`GET {base}/download/{fileName}`),
// so it inherits the caller's per-actor access — the same files the user can see.
//
// Registered as a local (non-Operation) tool because the generic Operation handler
// only returns text; this one must emit image / embedded-resource content. Unlike
// buildLink / getBbcodeTags it IS exposed in actor mode (see actorBindings) so the
// reaction-triggered AI agent can read files attached to the reaction it handles.

const (
	// readAttachmentFetchCap bounds how many bytes we pull from the download route,
	// guarding against an unexpectedly huge file exhausting memory / the context.
	readAttachmentFetchCap = 24 << 20 // 24 MiB
	// readAttachmentTextCap bounds inline text returned to the model (keeps token
	// cost sane); larger text files are returned truncated with a note.
	readAttachmentTextCap = 256 << 10 // 256 KiB
	// readAttachmentImageCap bounds images sent as a vision content block (Claude
	// rejects oversized images); larger images fall back to a metadata note.
	readAttachmentImageCap = 5 << 20 // 5 MiB
)

// registerReadAttachment adds the readAttachment tool to s.
func registerReadAttachment(s *server.MCPServer, c *apiclient.Client) {
	tool := mcp.NewTool("readAttachment",
		mcp.WithDescription(
			"Read the content of a stored file (attachment) so you can analyze it. Pass `fileName` — "+
				"the storage name returned by getAttachments / getActorAttachments (the `fileName` field) or "+
				"by uploadBase64. To read files attached to a reaction/comment, first list them with "+
				"getActorAttachments using the reaction's id, then call this with each fileName. "+
				"Text files (txt, md, csv, json, xml, yaml, source code, svg) come back as text; "+
				"images (png, jpeg, gif, webp) as a viewable image; PDFs and other binaries as an embedded "+
				"resource. Access follows your own permissions — you can only read files you can already see."),
		mcp.WithString("fileName", mcp.Required(),
			mcp.Description("Storage file name of the attachment (the `fileName` field from "+
				"getAttachments / getActorAttachments / uploadBase64), e.g. \"a1b2c3d4.pdf\" or "+
				"\"bucket/a1b2c3d4.pdf\".")),
	)
	s.AddTool(tool, readAttachmentHandler(c))
}

// readAttachmentHandler fetches the file via the PAPI download route and adapts the
// bytes to the MCP content type that best lets the model read them. Split out for
// testability.
func readAttachmentHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		fileName, _ := args["fileName"].(string)
		fileName = strings.TrimSpace(fileName)
		if fileName == "" {
			return mcp.NewToolResultError("[Error] readAttachment: missing required parameter \"fileName\""), nil
		}

		reqPath := "/download/" + encodeFilePath(fileName)
		raw, err := c.DoRaw(ctx, "GET", reqPath, nil, nil, readAttachmentFetchCap)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] readAttachment %q: %v", fileName, err)), nil
		}

		display := path.Base(fileName)
		ct := baseMIME(raw.ContentType)

		switch {
		case isTextualContent(ct, fileName):
			return textAttachmentResult(raw, fileName), nil
		case imageMIME(ct, fileName) != "":
			return imageAttachmentResult(raw, fileName, display), nil
		default:
			return binaryAttachmentResult(raw, fileName, display, ct), nil
		}
	}
}

// textAttachmentResult returns the file as inline text, truncating to the text cap.
func textAttachmentResult(raw apiclient.RawResponse, fileName string) *mcp.CallToolResult {
	body := raw.Body
	truncated := raw.Truncated
	if len(body) > readAttachmentTextCap {
		body = body[:readAttachmentTextCap]
		truncated = true
	}
	text := string(body)
	if truncated {
		text += fmt.Sprintf("\n\n[…readAttachment: %q truncated — file is larger than the %d KiB read limit]",
			fileName, readAttachmentTextCap>>10)
	}
	return mcp.NewToolResultText(text)
}

// imageAttachmentResult returns the file as a viewable image content block, or a
// metadata note when it exceeds the vision size cap or was truncated mid-fetch.
func imageAttachmentResult(raw apiclient.RawResponse, fileName, display string) *mcp.CallToolResult {
	mime := imageMIME(baseMIME(raw.ContentType), fileName)
	if raw.Truncated || len(raw.Body) > readAttachmentImageCap {
		return mcp.NewToolResultText(fmt.Sprintf(
			"readAttachment: %q is an image (%s) larger than the %d MiB inline limit, so it was not loaded. "+
				"Ask the user to view it in Simulator, or to share a smaller version.",
			display, mime, readAttachmentImageCap>>20))
	}
	encoded := base64.StdEncoding.EncodeToString(raw.Body)
	note := fmt.Sprintf("Image attachment %q (%s, %d bytes).", display, mime, len(raw.Body))
	return mcp.NewToolResultImage(note, encoded, mime)
}

// binaryAttachmentResult returns PDFs and other binaries as an embedded resource
// blob the client can render, or a metadata note when too large to inline.
func binaryAttachmentResult(raw apiclient.RawResponse, fileName, display, ct string) *mcp.CallToolResult {
	mime := ct
	if mime == "" {
		mime = "application/octet-stream"
	}
	if raw.Truncated {
		return mcp.NewToolResultText(fmt.Sprintf(
			"readAttachment: %q (%s) is larger than the %d MiB read limit and was not loaded. "+
				"Ask the user to open it in Simulator.",
			display, mime, readAttachmentFetchCap>>20))
	}
	encoded := base64.StdEncoding.EncodeToString(raw.Body)
	note := fmt.Sprintf(
		"Binary attachment %q (%s, %d bytes) returned as an embedded resource. "+
			"If your client cannot render it, ask the user to open the file in Simulator.",
		display, mime, len(raw.Body))
	return mcp.NewToolResultResource(note, mcp.BlobResourceContents{
		URI:      "simulator://attachment/" + fileName,
		MIMEType: mime,
		Blob:     encoded,
	})
}

// encodeFilePath percent-escapes each path segment of a storage file name while
// keeping the slash separators — the download route matches a `/*` wildcard, so
// "bucket/uuid.ext" must stay a multi-segment path.
func encodeFilePath(fileName string) string {
	parts := strings.Split(fileName, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// baseMIME returns the lowercase media type from a Content-Type header, dropping
// any "; charset=…" parameters and surrounding whitespace.
func baseMIME(contentType string) string {
	ct := contentType
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// fileExt returns the lowercase extension (without the dot) of a file name, or "".
func fileExt(fileName string) string {
	ext := strings.ToLower(path.Ext(path.Base(fileName)))
	return strings.TrimPrefix(ext, ".")
}

// textualExts are file extensions we treat as plain text when the server gives us
// a generic or missing Content-Type.
var textualExts = map[string]bool{
	"txt": true, "text": true, "md": true, "markdown": true, "rst": true,
	"csv": true, "tsv": true, "json": true, "jsonl": true, "ndjson": true,
	"xml": true, "yaml": true, "yml": true, "toml": true, "ini": true, "conf": true,
	"cfg": true, "env": true, "properties": true, "log": true, "sql": true,
	"html": true, "htm": true, "css": true, "svg": true,
	"js": true, "mjs": true, "cjs": true, "ts": true, "tsx": true, "jsx": true,
	"go": true, "py": true, "rb": true, "php": true, "java": true, "kt": true,
	"c": true, "h": true, "cpp": true, "hpp": true, "cc": true, "cs": true,
	"rs": true, "swift": true, "sh": true, "bash": true, "zsh": true, "ps1": true,
}

// isTextualContent reports whether the file should be returned as inline text,
// deciding by media type and falling back to the extension for generic types.
func isTextualContent(ct, fileName string) bool {
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case strings.HasSuffix(ct, "+json"), strings.HasSuffix(ct, "+xml"):
		return true
	}
	switch ct {
	case "application/json", "application/ld+json", "application/xml",
		"application/javascript", "application/ecmascript",
		"application/yaml", "application/x-yaml",
		"application/csv", "application/sql", "application/toml",
		"image/svg+xml":
		return true
	case "", "application/octet-stream", "binary/octet-stream":
		return textualExts[fileExt(fileName)]
	}
	return false
}

// imageMIME returns the vision MIME type (png/jpeg/gif/webp) for a file the model
// can view, or "" when it is not a supported image, falling back to the extension
// for generic content types.
func imageMIME(ct, fileName string) string {
	switch ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	case "image/jpg":
		return "image/jpeg"
	case "", "application/octet-stream", "binary/octet-stream":
		switch fileExt(fileName) {
		case "png":
			return "image/png"
		case "jpg", "jpeg":
			return "image/jpeg"
		case "gif":
			return "image/gif"
		case "webp":
			return "image/webp"
		}
	}
	return ""
}
