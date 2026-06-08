package engines

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// uploadResponse mirrors the JSON returned by POST /upload/{accId}.
// Only the fields we care about — everything else is ignored.
type uploadResponse struct {
	Data struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Size     int    `json:"size"`
		FileName string `json:"fileName"`
	} `json:"data"`
	StatusCode int    `json:"statusCode,omitempty"`
	Message    string `json:"message,omitempty"`
}

// maxLocalImageBytes caps how large a file the `localPath` source will read
// from disk.
const maxLocalImageBytes = 25 << 20 // 25 MiB

// allowedImageExts is the extension allow-list for the `localPath` source.
var allowedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".ico": true, ".svg": true,
}

// readLocalImage reads an image referenced by a `localPath` tool argument,
// rejecting non-image extensions, directories, and oversized files. This
// narrows the arbitrary-file-read surface of the localPath source: a caller
// who can steer tool arguments (e.g. via prompt injection) cannot exfiltrate
// arbitrary local files (such as ~/.ssh/id_rsa) by uploading them as an actor
// picture — see security review.
func readLocalImage(p string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(p))
	if !allowedImageExts[ext] {
		return nil, fmt.Errorf("localPath must point to an image file; %q is not an allowed extension", ext)
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("localPath points to a directory, not a file")
	}
	if info.Size() > maxLocalImageBytes {
		return nil, fmt.Errorf("localPath file is too large (%d bytes; max %d)", info.Size(), maxLocalImageBytes)
	}
	return os.ReadFile(p)
}

// uploadFile sends a multipart POST to /upload/{accId} and returns the
// storage fileName (path under which the file is accessible via /download).
// `filename` becomes the multipart Content-Disposition filename and is also
// echoed back as `title` on the attachment record.
// `contentType` is the MIME content type sent with the file part — pick the
// real type (e.g. "image/png") so the storage backend tags the file correctly.
func uploadFile(ctx context.Context, accID string, filename string, contentType string, fileBytes []byte) (string, error) {
	if accID == "" {
		return "", fmt.Errorf("workspace ID (accId) is empty")
	}
	if len(fileBytes) == 0 {
		return "", fmt.Errorf("file is empty")
	}

	// Build multipart body
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	if contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(partHeader)
	if err != nil {
		return "", fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return "", fmt.Errorf("write file bytes: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	// POST request — `ttl=0` mirrors the UI request and the Postman collection
	// (`Simulator_public_API.postman_collection.json`).  Without `ttl=0` the
	// file is sometimes uploaded with a TTL that hides it from the graph UI.
	apiURL := fmt.Sprintf("%s/upload/%s?ttl=0", buildBaseURL(), accID)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &buf)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := apiHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upload failed: HTTP %d: %s", resp.StatusCode, body)
	}

	var ur uploadResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return "", fmt.Errorf("parse response: %w (body: %.200s)", err, body)
	}
	if ur.Data.FileName == "" {
		return "", fmt.Errorf("empty fileName in response: %.200s", body)
	}
	return ur.Data.FileName, nil
}

// setActorPicture issues PUT /actors/actor/{formId}/{actorId}?replaceEmpty=false
// with only the `picture` field updated. Other actor data is preserved.
func setActorPicture(ctx context.Context, formID int, actorID, picture string) error {
	body := map[string]string{"picture": picture}
	bodyBytes, _ := json.Marshal(body)

	apiURL := fmt.Sprintf("%s/actors/actor/%d/%s?replaceEmpty=false",
		buildBaseURL(), formID, actorID)
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")

	client := apiHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("updateActor failed: HTTP %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// fetchImageFromURL downloads bytes from a URL with a short timeout.
func fetchImageFromURL(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent",
		"simulator-ai-plugin/uploadActorPicture (+https://github.com/corezoid/simulator-ai-plugin)")

	client := apiHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB cap
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// decodeBase64Flexible decodes a base64 string accepting both standard and
// URL-safe alphabets, with or without padding. Data URIs, JWT-style payloads
// and many image tools emit URL-safe base64, which StdEncoding alone rejects.
func decodeBase64Flexible(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("invalid base64 (tried std/url, padded/raw)")
}

// guessContentType picks a reasonable MIME based on filename extension.
// Falls back to image/png when the extension is missing or unrecognised
// — the storage backend uses the multipart Content-Type to tag files,
// and image/png is the safest default for the graph UI.
func guessContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	}
	return "image/png"
}

// handleUploadActorPicture uploads an image and sets it as the actor's
// picture (graph avatar). The image source can be one of:
//   - imageUrl:   public URL — the plugin downloads it
//   - localPath:  absolute path on the machine running the MCP server
//   - base64:     raw base64 string (no data: prefix)
//
// One of the three is required. `filename` overrides the auto-derived name
// and is used both as multipart filename and to pick the Content-Type.
func handleUploadActorPicture(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()
	actorID, _ := args["actorId"].(string)
	if actorID == "" {
		return mcp.NewToolResultError("[Error] actorId is required"), nil
	}
	if r := requireUUID("actorId", actorID); r != nil {
		return r, nil
	}
	formID := toInt(args["formId"])
	if formID == 0 {
		return mcp.NewToolResultError("[Error] formId is required"), nil
	}

	accID := os.Getenv("WORKSPACE_ID")
	if accID == "" {
		return mcp.NewToolResultError("[Error] WORKSPACE_ID is not set"), nil
	}

	// Resolve image bytes from one of the three sources.
	var (
		fileBytes []byte
		filename  string
	)
	if u, ok := args["imageUrl"].(string); ok && u != "" {
		b, _, err := fetchImageFromURL(ctx, u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch imageUrl: %v", err)), nil
		}
		fileBytes = b
		filename = filepath.Base(u)
		if i := strings.IndexAny(filename, "?#"); i >= 0 {
			filename = filename[:i]
		}
	} else if p, ok := args["localPath"].(string); ok && p != "" {
		b, err := readLocalImage(p)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] read localPath: %v", err)), nil
		}
		fileBytes = b
		filename = filepath.Base(p)
	} else if b64, ok := args["base64"].(string); ok && b64 != "" {
		// strip optional `data:image/png;base64,` prefix
		if i := strings.Index(b64, "base64,"); i >= 0 {
			b64 = b64[i+len("base64,"):]
		}
		b, err := decodeBase64Flexible(b64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] decode base64: %v", err)), nil
		}
		fileBytes = b
	} else {
		return mcp.NewToolResultError(
			"[Error] one of imageUrl / localPath / base64 is required"), nil
	}

	// Override filename if explicitly provided
	if fn, ok := args["filename"].(string); ok && fn != "" {
		filename = fn
	}
	if filename == "" {
		filename = "picture.png"
	}

	// SVG inputs are auto-rasterised to PNG: the graph UI doesn't render SVG
	// from storage paths (multipart Content-Type comes back as
	// application/octet-stream). pngWidth / pngHeight / svgFillColor are
	// optional and only apply when the source is detected as SVG.
	pngW := toInt(args["pngWidth"])
	pngH := toInt(args["pngHeight"])
	fillColor, _ := args["svgFillColor"].(string)
	newBytes, newName, contentType, err := maybeConvertSVG(fileBytes, filename, pngW, pngH, fillColor)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] svg→png: %v", err)), nil
	}
	fileBytes, filename = newBytes, newName

	// Step 1 — upload to storage and capture (attachId, storage path).
	storagePath, err := uploadFile(ctx, accID, filename, contentType, fileBytes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] upload: %v", err)), nil
	}

	// Step 2 — set the picture on the actor.
	if err := setActorPicture(ctx, formID, actorID, storagePath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] set picture: %v", err)), nil
	}

	out := map[string]string{
		"actorId": actorID,
		"picture": storagePath,
	}
	resultBytes, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(resultBytes)), nil
}

// uploadBulkItemResult is the per-item result returned by uploadActorPictureBulk.
type uploadBulkItemResult struct {
	ActorID string `json:"actorId"`
	Picture string `json:"picture,omitempty"`
	Status  string `json:"status"` // "ok" | "error"
	Error   string `json:"error,omitempty"`
}

// handleUploadActorPictureBulk uploads pictures for many actors in one MCP call,
// deduplicating identical source images so we only pay for the bytes upload
// once per unique image. Useful when wiring icons across the whole graph.
//
// items[] entries support the same source options as uploadActorPicture
// (`imageUrl` / `localPath` / `base64`), plus an explicit `picture` shortcut
// that skips the upload and binds an already-known storage path to the actor.
func handleUploadActorPictureBulk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}

	args := req.GetArguments()
	rawItems, ok := args["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return mcp.NewToolResultError("[Error] items[] is required and must be non-empty"), nil
	}
	if len(rawItems) > 500 {
		return mcp.NewToolResultError("[Error] items[] capped at 500 per call"), nil
	}

	accID := os.Getenv("WORKSPACE_ID")
	if accID == "" {
		return mcp.NewToolResultError("[Error] WORKSPACE_ID is not set"), nil
	}

	defaultW := toInt(args["pngWidth"])
	defaultH := toInt(args["pngHeight"])
	defaultFill, _ := args["svgFillColor"].(string)

	// Cache uploads by raw-bytes SHA-256 so we only POST /upload once per
	// unique image even if 50 actors share the same icon.
	uploadCache := map[string]string{} // hash → storage path

	results := make([]uploadBulkItemResult, 0, len(rawItems))

	for i, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			results = append(results, uploadBulkItemResult{
				Status: "error",
				Error:  fmt.Sprintf("items[%d] is not an object", i),
			})
			continue
		}

		actorID, _ := item["actorId"].(string)
		formID := toInt(item["formId"])
		if actorID == "" || formID == 0 {
			results = append(results, uploadBulkItemResult{
				ActorID: actorID,
				Status:  "error",
				Error:   fmt.Sprintf("items[%d] missing actorId or formId", i),
			})
			continue
		}
		if !isUUID(actorID) {
			results = append(results, uploadBulkItemResult{
				ActorID: actorID,
				Status:  "error",
				Error:   fmt.Sprintf("items[%d] actorId is not a valid UUID", i),
			})
			continue
		}

		// Picture shortcut — bind an existing storage path directly.
		if p, ok := item["picture"].(string); ok && p != "" {
			if err := setActorPicture(ctx, formID, actorID, p); err != nil {
				results = append(results, uploadBulkItemResult{
					ActorID: actorID, Status: "error",
					Error: fmt.Sprintf("set picture: %v", err),
				})
				continue
			}
			results = append(results, uploadBulkItemResult{
				ActorID: actorID, Picture: p, Status: "ok",
			})
			continue
		}

		// Resolve source bytes.
		var (
			fileBytes []byte
			filename  string
		)
		if u, ok := item["imageUrl"].(string); ok && u != "" {
			b, _, err := fetchImageFromURL(ctx, u)
			if err != nil {
				results = append(results, uploadBulkItemResult{
					ActorID: actorID, Status: "error",
					Error: fmt.Sprintf("fetch imageUrl: %v", err),
				})
				continue
			}
			fileBytes = b
			filename = filepath.Base(u)
			if i := strings.IndexAny(filename, "?#"); i >= 0 {
				filename = filename[:i]
			}
		} else if p, ok := item["localPath"].(string); ok && p != "" {
			b, err := readLocalImage(p)
			if err != nil {
				results = append(results, uploadBulkItemResult{
					ActorID: actorID, Status: "error",
					Error: fmt.Sprintf("read localPath: %v", err),
				})
				continue
			}
			fileBytes = b
			filename = filepath.Base(p)
		} else if b64, ok := item["base64"].(string); ok && b64 != "" {
			if i := strings.Index(b64, "base64,"); i >= 0 {
				b64 = b64[i+len("base64,"):]
			}
			b, err := decodeBase64Flexible(b64)
			if err != nil {
				results = append(results, uploadBulkItemResult{
					ActorID: actorID, Status: "error",
					Error: fmt.Sprintf("decode base64: %v", err),
				})
				continue
			}
			fileBytes = b
		} else {
			results = append(results, uploadBulkItemResult{
				ActorID: actorID, Status: "error",
				Error: "one of imageUrl / localPath / base64 / picture is required",
			})
			continue
		}

		if fn, ok := item["filename"].(string); ok && fn != "" {
			filename = fn
		}
		if filename == "" {
			filename = "picture.png"
		}

		// Per-item override or default sizes.
		w := toInt(item["pngWidth"])
		if w == 0 {
			w = defaultW
		}
		h := toInt(item["pngHeight"])
		if h == 0 {
			h = defaultH
		}
		fill, _ := item["svgFillColor"].(string)
		if fill == "" {
			fill = defaultFill
		}

		newBytes, newName, contentType, err := maybeConvertSVG(fileBytes, filename, w, h, fill)
		if err != nil {
			results = append(results, uploadBulkItemResult{
				ActorID: actorID, Status: "error",
				Error: fmt.Sprintf("svg→png: %v", err),
			})
			continue
		}
		fileBytes, filename = newBytes, newName

		// Dedup uploads by content hash.
		hash := sha256.Sum256(fileBytes)
		key := fmt.Sprintf("%x", hash[:])
		storagePath, cached := uploadCache[key]
		if !cached {
			sp, err := uploadFile(ctx, accID, filename, contentType, fileBytes)
			if err != nil {
				results = append(results, uploadBulkItemResult{
					ActorID: actorID, Status: "error",
					Error: fmt.Sprintf("upload: %v", err),
				})
				continue
			}
			storagePath = sp
			uploadCache[key] = sp
		}

		if err := setActorPicture(ctx, formID, actorID, storagePath); err != nil {
			results = append(results, uploadBulkItemResult{
				ActorID: actorID, Status: "error",
				Error: fmt.Sprintf("set picture: %v", err),
			})
			continue
		}
		results = append(results, uploadBulkItemResult{
			ActorID: actorID, Picture: storagePath, Status: "ok",
		})
	}

	summary := struct {
		Total   int                    `json:"total"`
		Ok      int                    `json:"ok"`
		Errors  int                    `json:"errors"`
		Uploads int                    `json:"uploads"`
		Items   []uploadBulkItemResult `json:"items"`
	}{Total: len(results), Uploads: len(uploadCache), Items: results}
	for _, r := range results {
		if r.Status == "ok" {
			summary.Ok++
		} else {
			summary.Errors++
		}
	}
	out, _ := json.Marshal(summary)
	return mcp.NewToolResultText(string(out)), nil
}
