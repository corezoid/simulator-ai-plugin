package mcpserver

import (
	"bytes"
	"context"
	"crypto/tls"
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
	"time"

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
	req.Header.Set("Authorization", globalApiConfig.Authorization)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
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
	req.Header.Set("Authorization", globalApiConfig.Authorization)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
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

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
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
		b, err := os.ReadFile(p)
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
		b, err := base64.StdEncoding.DecodeString(b64)
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
	contentType := guessContentType(filename)

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
