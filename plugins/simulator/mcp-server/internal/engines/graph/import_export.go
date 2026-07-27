//nolint:goconst // API payload keys and strategy literals are easier to review inline.
package graph

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"
)

const (
	graphArchiveFetchCap = 100 << 20 // 100 MiB
	graphArchiveLocalCap = 100 << 20 // 100 MiB
	graphTaskPollDelay   = 3 * time.Second
)

type graphTaskResponse struct {
	Data graphTask `json:"data"`
}

type graphTaskListResponse struct {
	Data graphTaskListData `json:"data"`
}

type graphTaskListData struct {
	Tasks []graphTask
}

func (d *graphTaskListData) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '[' {
		return json.Unmarshal(raw, &d.Tasks)
	}
	var obj struct {
		Tasks []graphTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	d.Tasks = obj.Tasks
	return nil
}

type graphTask struct {
	ID        int             `json:"id"`
	Status    string          `json:"status"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	AccID     string          `json:"accId,omitempty"`
	CreatedAt int64           `json:"createdAt,omitempty"`
	TimeStart *int64          `json:"timeStart,omitempty"`
	TimeEnd   *int64          `json:"timeEnd,omitempty"`
}

type graphTaskDetails struct {
	Error    bool            `json:"error,omitempty"`
	ErrMsg   string          `json:"errMsg,omitempty"`
	ErrStack string          `json:"errStack,omitempty"`
	Actor    *graphTaskActor `json:"actor,omitempty"`
	File     *graphTaskFile  `json:"file,omitempty"`
	Manifest *graphManifest  `json:"manifest,omitempty"`
}

type graphTaskActor struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

type graphTaskFile struct {
	ID       int    `json:"id,omitempty"`
	FileName string `json:"fileName,omitempty"`
	Title    string `json:"title,omitempty"`
	Size     int    `json:"size,omitempty"`
	Type     string `json:"type,omitempty"`
}

type graphManifest struct {
	Version string `json:"version,omitempty"`
	Created string `json:"created,omitempty"`
	Author  string `json:"author,omitempty"`
	Stats   struct {
		Counters map[string]int `json:"counters,omitempty"`
	} `json:"stats"`
}

type graphSourceInfo struct {
	URL         string `json:"url,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	GraphID     string `json:"graphId,omitempty"`
	LayerID     string `json:"layerId,omitempty"`
	SourceKind  string `json:"sourceKind,omitempty"`
}

type graphArchiveActorMeta struct {
	ID     string `json:"id,omitempty"`
	FormID int    `json:"formId,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Title  string `json:"title,omitempty"`
}

type graphResolvedActor struct {
	ID        string `json:"id,omitempty"`
	FormID    int    `json:"formId,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Title     string `json:"title,omitempty"`
	FormTitle string `json:"formTitle,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

type graphActorSearchResponse struct {
	Data graphActorSearchData `json:"data"`
}

type graphActorSearchData struct {
	List  []graphResolvedActor `json:"list"`
	Total int                  `json:"total,omitempty"`
}

func (d *graphActorSearchData) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '[' {
		return json.Unmarshal(raw, &d.List)
	}
	var obj struct {
		List  []graphResolvedActor `json:"list"`
		Items []graphResolvedActor `json:"items"`
		Data  []graphResolvedActor `json:"data"`
		Total int                  `json:"total,omitempty"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	switch {
	case obj.List != nil:
		d.List = obj.List
	case obj.Items != nil:
		d.List = obj.Items
	case obj.Data != nil:
		d.List = obj.Data
	}
	d.Total = obj.Total
	return nil
}

type graphImportExportClient struct {
	baseURL string
	auth    string
	http    *http.Client
}

func newGraphImportExportClient(ctx context.Context) (*graphImportExportClient, error) {
	auth := ecore.AuthHeaderForContext(ctx)
	if strings.TrimSpace(auth) == "" {
		return nil, errors.New("missing Authorization header")
	}
	return &graphImportExportClient{
		baseURL: graphImportExportBaseURL(ctx, auth),
		auth:    auth,
		http:    ecore.APIHTTPClient(),
	}, nil
}

func graphImportExportBaseURL(ctx context.Context, authHeader string) string {
	base := strings.TrimRight(ecore.BuildBaseURLForContext(ctx), "/")
	if !strings.HasPrefix(strings.TrimSpace(authHeader), "Bearer ") {
		return base
	}
	if strings.Contains(base, "/papi/") {
		return strings.Replace(base, "/papi/", "/api/", 1)
	}
	if trimmed, ok := strings.CutSuffix(base, "/papi"); ok {
		return trimmed + "/api"
	}
	return base
}

func (c *graphImportExportClient) doJSON(ctx context.Context, method, reqPath string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+reqPath, reader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("X-App-Client", "web")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("parse response: %w (body: %.200s)", err, respBody)
	}
	return nil
}

func (c *graphImportExportClient) download(ctx context.Context, fileName string) (data []byte, contentType string, err error) {
	reqPath := "/download/" + encodeGraphArchivePath(fileName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+reqPath, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("X-App-Client", "web")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err = io.ReadAll(io.LimitReader(resp.Body, graphArchiveFetchCap+1))
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if len(data) > graphArchiveFetchCap {
		return nil, "", fmt.Errorf("graph archive is too large (%d+ bytes; max %d)", graphArchiveFetchCap, graphArchiveFetchCap)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (c *graphImportExportClient) upload(ctx context.Context, accID, filename string, fileBytes []byte) (*graphTaskFile, error) {
	if accID == "" {
		return nil, errors.New("workspace ID (accId) is empty")
	}
	if len(fileBytes) == 0 {
		return nil, errors.New("file is empty")
	}
	if err := validateGraphArchiveBytes(fileBytes); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	partHeader.Set("Content-Type", "application/octet-stream")
	part, err := mw.CreatePart(partHeader)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return nil, fmt.Errorf("write file bytes: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	reqURL := fmt.Sprintf("%s/upload/%s?ttl=0", c.baseURL, url.PathEscape(accID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var ur struct {
		Data       graphTaskFile `json:"data"`
		StatusCode int           `json:"statusCode,omitempty"`
		Message    string        `json:"message,omitempty"`
	}
	if err := json.Unmarshal(respBody, &ur); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %.200s)", err, respBody)
	}
	if ur.Data.FileName == "" {
		return nil, fmt.Errorf("empty fileName in response: %.200s", respBody)
	}
	return &ur.Data, nil
}

func (c *graphImportExportClient) createTask(ctx context.Context, accID, name string, body any) (*graphTask, error) {
	var resp graphTaskResponse
	if err := c.doJSON(ctx, http.MethodPost, "/tasks/"+url.PathEscape(accID)+"/"+name, body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

func (c *graphImportExportClient) getTask(ctx context.Context, accID, name string, taskID int) (*graphTask, error) {
	if taskID <= 0 {
		return nil, errors.New("taskID is required")
	}
	reqPath := fmt.Sprintf("/tasks/%s/%s/%d", url.PathEscape(accID), name, taskID)
	var resp graphTaskResponse
	if err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Data.ID == 0 {
		return nil, fmt.Errorf("%s task %d not found", name, taskID)
	}
	return &resp.Data, nil
}

func (c *graphImportExportClient) listTasks(ctx context.Context, accID, name string, limit, offset int) ([]graphTask, error) {
	if limit <= 0 {
		limit = 15
	}
	if offset < 0 {
		offset = 0
	}
	reqPath := fmt.Sprintf("/tasks/list/%s/%s?limit=%d&offset=%d", url.PathEscape(accID), name, limit, offset)
	var resp graphTaskListResponse
	if err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Tasks, nil
}

func (c *graphImportExportClient) findTask(ctx context.Context, accID, name string, taskID, limit int) (*graphTask, error) {
	if taskID > 0 {
		if task, err := c.getTask(ctx, accID, name, taskID); err == nil {
			return task, nil
		}
	}
	tasks, err := c.listTasks(ctx, accID, name, limit, 0)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i].ID == taskID {
			return &tasks[i], nil
		}
	}
	if taskID > 0 {
		if task, err := c.getTask(ctx, accID, name, taskID); err == nil {
			return task, nil
		}
	}
	return nil, fmt.Errorf("%s task %d not found in the first %d tasks", name, taskID, limit)
}

func (c *graphImportExportClient) getActorByRef(ctx context.Context, formID int, ref string) (*graphResolvedActor, error) {
	if formID <= 0 || strings.TrimSpace(ref) == "" {
		return nil, errors.New("formID and ref are required")
	}
	reqPath := fmt.Sprintf("/actors/ref/%d/%s?filter=id,title,ref,formId,formTitle,createdAt", formID, url.PathEscape(ref))
	var resp struct {
		Data graphResolvedActor `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Data.ID == "" {
		return nil, fmt.Errorf("empty actor id for formId=%d ref=%q", formID, ref)
	}
	return &resp.Data, nil
}

func (c *graphImportExportClient) getActorMetadata(ctx context.Context, actorID string) (*graphResolvedActor, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, errors.New("actorID is required")
	}
	reqPath := "/actors/" + url.PathEscape(actorID) + "?filter=id,title,ref,formId,formTitle,createdAt"
	var resp struct {
		Data graphResolvedActor `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Data.ID == "" {
		return nil, fmt.Errorf("empty actor id for %s", actorID)
	}
	return &resp.Data, nil
}

func (c *graphImportExportClient) searchActorsByTitle(ctx context.Context, accID, title string, formID, limit int) ([]graphResolvedActor, error) {
	if strings.TrimSpace(accID) == "" || strings.TrimSpace(title) == "" {
		return nil, errors.New("accID and title are required")
	}
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	q.Set("filter", "id,title,ref,formId,formTitle,createdAt")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("searchType", "text")
	if formID > 0 {
		q.Set("formId", strconv.Itoa(formID))
	}
	reqPath := fmt.Sprintf("/actors_filters/search/%s/%s?%s", url.PathEscape(accID), url.PathEscape(title), q.Encode())
	var resp graphActorSearchResponse
	if err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data.List, nil
}

func (c *graphImportExportClient) waitTask(ctx context.Context, accID, name string, taskID, waitSec int) (*graphTask, *graphTaskDetails, error) {
	if waitSec <= 0 {
		task, err := c.findTask(ctx, accID, name, taskID, 50)
		if err != nil {
			return nil, nil, err
		}
		details, err := parseGraphTaskDetails(task)
		if err != nil {
			return nil, nil, err
		}
		return task, details, nil
	}
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	for {
		task, err := c.findTask(ctx, accID, name, taskID, 50)
		if err != nil {
			return nil, nil, err
		}
		details, err := parseGraphTaskDetails(task)
		if err != nil {
			return nil, nil, err
		}
		if isFinalGraphTaskStatus(task.Status) {
			return task, details, nil
		}
		if time.Now().Add(graphTaskPollDelay).After(deadline) {
			return task, details, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(graphTaskPollDelay):
		}
	}
}

func parseGraphTaskDetails(task *graphTask) (*graphTaskDetails, error) { //nolint:nilnil // Empty task details are valid for queued tasks.
	if task == nil || len(task.Details) == 0 || string(task.Details) == "null" {
		return nil, nil //nolint:nilnil // Empty details means there is no details payload yet.
	}
	var raw json.RawMessage
	if len(task.Details) > 0 && task.Details[0] == '"' {
		var s string
		if err := json.Unmarshal(task.Details, &s); err != nil {
			return nil, err
		}
		if strings.TrimSpace(s) == "" {
			return nil, nil //nolint:nilnil // Empty details string means there is no details payload yet.
		}
		raw = json.RawMessage(s)
	} else {
		raw = task.Details
	}
	var details graphTaskDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

func isFinalGraphTaskStatus(status string) bool {
	switch normalizeGraphTaskStatus(status) {
	case "completed", "failed", "canceled":
		return true
	default:
		return false
	}
}

func normalizeGraphTaskStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func graphTaskOutput(task *graphTask, details *graphTaskDetails) map[string]any {
	out := map[string]any{
		"id":        task.ID,
		"status":    task.Status,
		"name":      task.Name,
		"accId":     task.AccID,
		"createdAt": task.CreatedAt,
	}
	if len(task.Data) > 0 && string(task.Data) != "null" {
		var data any
		if err := json.Unmarshal(task.Data, &data); err == nil {
			out["data"] = data
		}
	}
	if task.TimeStart != nil {
		out["timeStart"] = *task.TimeStart
	}
	if task.TimeEnd != nil {
		out["timeEnd"] = *task.TimeEnd
	}
	if details != nil {
		out["details"] = details
	}
	return out
}

func graphTaskCounters(details *graphTaskDetails) map[string]int {
	if details == nil || details.Manifest == nil {
		return nil
	}
	return details.Manifest.Stats.Counters
}

func graphCountersEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if b[k] != av {
			return false
		}
	}
	return true
}

func graphTaskFailureHint(message string) string {
	msg := strings.ToLower(message)
	if strings.Contains(msg, "access") || strings.Contains(msg, "permission") ||
		strings.Contains(msg, "forbidden") || strings.Contains(msg, "denied") {
		return "Export requires access to every object included by the selected graph/layer recursion. If a layer contains actors, forms, accounts, files, or linked objects the current user cannot read, the backend can reject the export; grant access or export a narrower selection."
	}
	return ""
}

func parseGraphSourceURL(raw string) (*graphSourceInfo, error) { //nolint:nilnil // Empty sourceUrl is optional and handled by explicit ids.
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil //nolint:nilnil // sourceUrl is optional; callers may provide explicit ids instead.
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse sourceUrl: %w", err)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	for i, p := range parts {
		decoded, err := url.PathUnescape(p)
		if err == nil {
			parts[i] = decoded
		}
	}
	info := &graphSourceInfo{URL: raw}
	for i := range parts {
		if parts[i] != "actors_graph" || i+1 >= len(parts) {
			continue
		}
		info.WorkspaceID = parts[i+1]
		for j := i + 2; j < len(parts); j++ {
			if parts[j] == "graph" && j+1 < len(parts) {
				info.GraphID = parts[j+1]
			}
			if parts[j] == "layers" && j+1 < len(parts) {
				info.LayerID = parts[j+1]
			}
		}
		break
	}
	if info.GraphID == "" && info.LayerID == "" {
		return nil, errors.New("sourceUrl must be a Simulator graph URL like /actors_graph/<workspace>/graph/<graphId>/layers/<layerId>")
	}
	if info.GraphID != "" && !ecore.IsUUID(info.GraphID) && info.GraphID != "0" {
		return nil, fmt.Errorf("sourceUrl graph id must be a UUID or 0, got %q", info.GraphID)
	}
	if info.LayerID != "" && !ecore.IsUUID(info.LayerID) {
		return nil, fmt.Errorf("sourceUrl layer id must be a UUID, got %q", info.LayerID)
	}
	if info.WorkspaceID != "" && len(info.WorkspaceID) != 8 && !ecore.IsUUID(info.WorkspaceID) {
		return nil, fmt.Errorf("sourceUrl workspace segment must be a full UUID or 8-char short id, got %q", info.WorkspaceID)
	}
	return info, nil
}

func resolveSourceWorkspace(ctx context.Context, parsed string) string {
	parsed = strings.TrimSpace(parsed)
	if parsed == "" {
		return ""
	}
	current := ecore.WorkspaceIDForContext(ctx)
	if ecore.IsUUID(parsed) {
		return parsed
	}
	if current != "" && len(parsed) == 8 && strings.HasPrefix(current, parsed) {
		return current
	}
	return parsed
}

func graphSelectionProvided(args map[string]any) bool {
	if boolArg(args, "allWorkspace") {
		return true
	}
	actorIDs, _ := actorIDSliceArg(args)
	formIDs, _ := intSliceArg(args, "formIds")
	return len(actorIDs) > 0 || len(formIDs) > 0
}

func copyToolArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	maps.Copy(out, args)
	return out
}

func applyGraphSourceURL(ctx context.Context, args map[string]any) (*graphSourceInfo, error) {
	sourceURL := strings.TrimSpace(stringArg(args, "sourceUrl"))
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(stringArg(args, "graphUrl"))
	}
	info, err := parseGraphSourceURL(sourceURL)
	if err != nil || info == nil {
		return info, err
	}
	if strings.TrimSpace(stringArg(args, "sourceAccId")) == "" {
		if accID := resolveSourceWorkspace(ctx, info.WorkspaceID); accID != "" {
			args["sourceAccId"] = accID
			info.WorkspaceID = accID
		}
	} else if accID := resolveSourceWorkspace(ctx, stringArg(args, "sourceAccId")); accID != "" {
		info.WorkspaceID = accID
	}
	kind := strings.TrimSpace(stringArgDefault(args, "sourceKind", "auto"))
	if kind == "" {
		kind = "auto"
	}
	switch kind {
	case "auto":
		if info.LayerID != "" {
			kind = "layer"
		} else {
			kind = "graph"
		}
	case "layer", "graph":
	default:
		return nil, fmt.Errorf("sourceKind must be auto, layer, or graph, got %q", kind)
	}
	info.SourceKind = kind
	if !graphSelectionProvided(args) {
		switch kind {
		case "layer":
			if info.LayerID == "" {
				return nil, errors.New("sourceKind=layer requires a URL with /layers/<layerId>")
			}
			args["actorIds"] = []any{info.LayerID}
		case "graph":
			if info.GraphID == "" || info.GraphID == "0" {
				return nil, errors.New("sourceKind=graph requires a URL with /graph/<graphActorId>")
			}
			args["actorIds"] = []any{info.GraphID}
		}
	}
	return info, nil
}

func requireGraphTaskOK(name string, task *graphTask, details *graphTaskDetails) error {
	if task == nil {
		return fmt.Errorf("%s task is missing", name)
	}
	switch normalizeGraphTaskStatus(task.Status) {
	case "failed":
		if details != nil && details.ErrMsg != "" {
			return fmt.Errorf("%s task %d failed: %s", name, task.ID, details.ErrMsg)
		}
		return fmt.Errorf("%s task %d failed", name, task.ID)
	case "canceled":
		return fmt.Errorf("%s task %d was canceled", name, task.ID)
	case "completed":
		return nil
	default:
		return fmt.Errorf("%s task %d is still %s", name, task.ID, task.Status)
	}
}

func buildGraphExportBody(args map[string]any) (map[string]any, error) {
	allWorkspace := boolArg(args, "allWorkspace")
	if allWorkspace {
		if !boolArg(args, "confirmAllWorkspaceExport") {
			return nil, errors.New("allWorkspace export requires confirmAllWorkspaceExport=true")
		}
		return map[string]any{"allWorkspace": true}, nil
	}

	actorIDs, err := actorIDSliceArg(args)
	if err != nil {
		return nil, err
	}
	formIDs, err := intSliceArg(args, "formIds")
	if err != nil {
		return nil, err
	}
	if actorIDs == nil {
		actorIDs = []string{}
	}
	if formIDs == nil {
		formIDs = []int{}
	}
	if len(actorIDs) == 0 && len(formIDs) == 0 {
		return nil, errors.New("provide actorIds, formIds, or allWorkspace=true")
	}
	for _, actorID := range actorIDs {
		if !ecore.IsUUID(actorID) {
			return nil, fmt.Errorf("actorIds must contain valid UUIDs, got %q", actorID)
		}
	}

	ops := map[string]any{
		"transactions":         boolArg(args, "transactions"),
		"processes":            boolArg(args, "processes"),
		"users":                boolArg(args, "users"),
		"attachments":          boolArgDefault(args, "attachments", false),
		"balances":             boolArg(args, "balances"),
		"connectorsToAccounts": boolArg(args, "connectorsToAccounts"),
		"accountToActors":      boolArg(args, "accountToActors"),
	}
	if level, ok, err := maxRecursionLevelArg(args); err != nil {
		return nil, err
	} else if ok {
		ops["maxRecursionLevel"] = level
	}

	return map[string]any{
		"actors":               actorIDs,
		"forms":                formIDs,
		"ops":                  ops,
		"balances":             boolArg(args, "balances"),
		"connectorsToAccounts": boolArg(args, "connectorsToAccounts"),
		"systemCounters":       boolArg(args, "systemCounters"),
		"accountToActors":      boolArg(args, "accountToActors"),
	}, nil
}

func buildGraphImportBody(fileName, title string, args map[string]any) (map[string]any, error) {
	if !boolArg(args, "confirmImport") && !boolArg(args, "confirmClone") {
		return nil, errors.New("import creates or updates workspace objects; pass confirmImport=true")
	}
	strategy := strings.TrimSpace(stringArgDefault(args, "refStrategy", "replace"))
	if strategy == "" {
		strategy = "replace"
	}
	switch strategy {
	case "replace", "reuse", "error":
	default:
		return nil, fmt.Errorf("refStrategy must be replace, reuse, or error, got %q", strategy)
	}
	if strategy == "reuse" && !boolArg(args, "allowReuseImport") {
		return nil, errors.New("refStrategy=reuse can update existing matching REF objects; pass allowReuseImport=true if this is intended")
	}

	prefix := strings.TrimSpace(stringArg(args, "refReplacePrefix"))
	if strategy == "replace" {
		if prefix == "" {
			return nil, errors.New("refStrategy=replace requires non-empty refReplacePrefix so imported refs cannot collide silently")
		}
		if len(prefix) < 8 {
			return nil, errors.New("refReplacePrefix must be at least 8 characters")
		}
	} else if prefix != "" {
		return nil, errors.New("refReplacePrefix is only valid with refStrategy=replace; omit it for refStrategy=reuse or refStrategy=error")
	}

	ops := map[string]any{
		"processesRefStrategy":   strategy,
		"actorRefStrategy":       strategy,
		"formRefStrategy":        strategy,
		"transferRefStrategy":    strategy,
		"transactionRefStrategy": strategy,
	}
	if prefix != "" {
		ops["actorRefReplacePrefix"] = prefix
		ops["formRefReplacePrefix"] = prefix
		ops["transferRefReplacePrefix"] = prefix
		ops["transactionRefReplacePrefix"] = prefix
	}

	users, err := objectSliceArg(args, "users")
	if err != nil {
		return nil, err
	}
	scripts, err := objectSliceArg(args, "scripts")
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = path.Base(fileName)
	}
	return map[string]any{
		"fileName": fileName,
		"title":    title,
		"users":    users,
		"scripts":  scripts,
		"ops":      ops,
	}, nil
}

func graphToolError(prefix string, err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(prefix + err.Error()), nil
}

func graphToolErrorf(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

func graphToolJSON(toolName string, value any) (*mcp.CallToolResult, error) {
	out, err := json.Marshal(value)
	if err != nil {
		return graphToolErrorf("[Error] %s: marshal result: %v", toolName, err)
	}
	return mcp.NewToolResultText(string(out)), nil
}

func handleCreateGraphExportTask(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // mcp-go handler signature uses value requests.
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	accID := workspaceArg(ctx, args, "accId")
	if accID == "" {
		return mcp.NewToolResultError("[Error] createGraphExportTask: no workspace set — run set-workspace or pass accId"), nil
	}
	body, err := buildGraphExportBody(args)
	if err != nil {
		return graphToolError("[Error] createGraphExportTask: ", err)
	}
	client, err := newGraphImportExportClient(ctx)
	if err != nil {
		return graphToolError("[Error] createGraphExportTask: ", err)
	}
	task, err := client.createTask(ctx, accID, "export", body)
	if err != nil {
		return graphToolErrorf("[Error] createGraphExportTask: %v", err)
	}
	details, _ := parseGraphTaskDetails(task)
	waitSec := intArg(args, "waitSec")
	if waitSec > 0 {
		task, details, err = client.waitTask(ctx, accID, "export", task.ID, waitSec)
		if err != nil {
			return graphToolErrorf("[Error] createGraphExportTask: %v", err)
		}
	}
	return graphToolJSON("createGraphExportTask", graphTaskOutput(task, details))
}

func handleGetGraphImportExportTask(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // mcp-go handler signature uses value requests.
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	accID := workspaceArg(ctx, args, "accId")
	if accID == "" {
		return mcp.NewToolResultError("[Error] getGraphImportExportTask: no workspace set — run set-workspace or pass accId"), nil
	}
	name := strings.TrimSpace(stringArgDefault(args, "name", "export"))
	if name != "export" && name != "import" {
		return mcp.NewToolResultError("[Error] getGraphImportExportTask: name must be export or import"), nil
	}
	limit := intArgDefault(args, "limit", 15)
	offset := intArg(args, "offset")
	client, err := newGraphImportExportClient(ctx)
	if err != nil {
		return graphToolError("[Error] getGraphImportExportTask: ", err)
	}
	if taskID := intArg(args, "taskId"); taskID > 0 {
		task, err := client.findTask(ctx, accID, name, taskID, limit)
		if err != nil {
			return graphToolErrorf("[Error] getGraphImportExportTask: %v", err)
		}
		details, err := parseGraphTaskDetails(task)
		if err != nil {
			return graphToolErrorf("[Error] getGraphImportExportTask: parse details: %v", err)
		}
		return graphToolJSON("getGraphImportExportTask", graphTaskOutput(task, details))
	}
	tasks, err := client.listTasks(ctx, accID, name, limit, offset)
	if err != nil {
		return graphToolErrorf("[Error] getGraphImportExportTask: %v", err)
	}
	out := make([]map[string]any, 0, len(tasks))
	for i := range tasks {
		details, err := parseGraphTaskDetails(&tasks[i])
		if err != nil {
			return graphToolErrorf("[Error] getGraphImportExportTask: parse details for task %d: %v", tasks[i].ID, err)
		}
		out = append(out, graphTaskOutput(&tasks[i], details))
	}
	return graphToolJSON("getGraphImportExportTask", map[string]any{"tasks": out, "limit": limit, "offset": offset})
}

func handleImportGraphArchive(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // mcp-go handler signature uses value requests.
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	accID := workspaceArg(ctx, args, "accId")
	if accID == "" {
		return mcp.NewToolResultError("[Error] importGraphArchive: no workspace set — run set-workspace or pass accId"), nil
	}
	fileName := strings.TrimSpace(stringArg(args, "fileName"))
	localPath := strings.TrimSpace(stringArg(args, "localPath"))
	title := strings.TrimSpace(stringArg(args, "title"))
	if (fileName == "") == (localPath == "") {
		return mcp.NewToolResultError("[Error] importGraphArchive: provide exactly one of fileName or localPath"), nil
	}
	if _, err := buildGraphImportBody("placeholder.graph", title, args); err != nil {
		return graphToolError("[Error] importGraphArchive: ", err)
	}
	client, err := newGraphImportExportClient(ctx)
	if err != nil {
		return graphToolError("[Error] importGraphArchive: ", err)
	}
	var uploaded *graphTaskFile
	if localPath != "" {
		archiveBytes, filename, err := readLocalGraphArchive(localPath)
		if err != nil {
			return graphToolError("[Error] importGraphArchive: ", err)
		}
		uploaded, err = client.upload(ctx, accID, filename, archiveBytes)
		if err != nil {
			return graphToolErrorf("[Error] importGraphArchive upload: %v", err)
		}
		fileName = uploaded.FileName
		if title == "" {
			title = uploaded.Title
		}
	}
	body, err := buildGraphImportBody(fileName, title, args)
	if err != nil {
		return graphToolError("[Error] importGraphArchive: ", err)
	}
	task, err := client.createTask(ctx, accID, "import", body)
	if err != nil {
		return graphToolErrorf("[Error] importGraphArchive: %v", err)
	}
	details, _ := parseGraphTaskDetails(task)
	waitSec := intArg(args, "waitSec")
	if waitSec > 0 {
		task, details, err = client.waitTask(ctx, accID, "import", task.ID, waitSec)
		if err != nil {
			return graphToolErrorf("[Error] importGraphArchive: %v", err)
		}
	}
	result := graphTaskOutput(task, details)
	if uploaded != nil {
		result["uploadedFile"] = uploaded
	}
	return graphToolJSON("importGraphArchive", result)
}

func handleCloneGraphObjects(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // mcp-go handler signature uses value requests.
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := copyToolArgs(req.GetArguments())
	if !boolArg(args, "confirmClone") {
		return mcp.NewToolResultError("[Error] cloneGraphObjects: cloning imports objects into a workspace; pass confirmClone=true"), nil
	}
	sourceInfo, err := applyGraphSourceURL(ctx, args)
	if err != nil {
		return graphToolError("[Error] cloneGraphObjects sourceUrl: ", err)
	}
	sourceAccID := workspaceArg(ctx, args, "sourceAccId")
	exportBody, err := buildGraphExportBody(args)
	if err != nil {
		return graphToolError("[Error] cloneGraphObjects export: ", err)
	}
	selection := map[string]any{}
	if actorIDs, err := actorIDSliceArg(args); err == nil && len(actorIDs) > 0 {
		selection["actorIds"] = actorIDs
	}
	if formIDs, err := intSliceArg(args, "formIds"); err == nil && len(formIDs) > 0 {
		selection["formIds"] = formIDs
	}
	if boolArg(args, "allWorkspace") {
		selection["allWorkspace"] = true
	}
	if sourceInfo != nil {
		selection["sourceUrl"] = sourceInfo.URL
		selection["sourceWorkspaceId"] = sourceInfo.WorkspaceID
		selection["sourceGraphId"] = sourceInfo.GraphID
		selection["sourceLayerId"] = sourceInfo.LayerID
		selection["sourceKind"] = sourceInfo.SourceKind
	}
	return runGraphClone(ctx, "cloneGraphObjects", args, sourceAccID, exportBody, selection)
}

func handleCloneGraphLayer(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { //nolint:gocritic // mcp-go handler signature uses value requests.
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := copyToolArgs(req.GetArguments())
	if !boolArg(args, "confirmClone") {
		return mcp.NewToolResultError("[Error] cloneGraphLayer: cloning imports objects into a workspace; pass confirmClone=true"), nil
	}
	sourceInfo, err := applyGraphSourceURL(ctx, args)
	if err != nil {
		return graphToolError("[Error] cloneGraphLayer sourceUrl: ", err)
	}
	layerID := strings.TrimSpace(stringArg(args, "layerId"))
	if layerID == "" && sourceInfo != nil {
		layerID = sourceInfo.LayerID
	}
	if layerID == "" {
		return mcp.NewToolResultError("[Error] cloneGraphLayer: layerId is required"), nil
	}
	if r := ecore.RequireUUID("layerId", layerID); r != nil {
		return r, nil
	}
	sourceAccID := workspaceArg(ctx, args, "sourceAccId")

	exportArgs := map[string]any{}
	maps.Copy(exportArgs, args)
	exportArgs["actorIds"] = []any{layerID}
	body, err := buildGraphExportBody(exportArgs)
	if err != nil {
		return graphToolError("[Error] cloneGraphLayer export: ", err)
	}
	subject := map[string]any{"layerId": layerID, "sourceLayerId": layerID}
	if sourceInfo != nil {
		subject["sourceUrl"] = sourceInfo.URL
		subject["sourceWorkspaceId"] = sourceInfo.WorkspaceID
		subject["sourceGraphId"] = sourceInfo.GraphID
		subject["sourceLayerId"] = sourceInfo.LayerID
		subject["sourceKind"] = "layer"
	}
	return runGraphClone(ctx, "cloneGraphLayer", args, sourceAccID, body, subject)
}

func runGraphClone(ctx context.Context, toolName string, args map[string]any, sourceAccID string, exportBody, subject map[string]any) (*mcp.CallToolResult, error) {
	if sourceAccID == "" {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] %s: no source workspace set — run set-workspace or pass sourceAccId", toolName)), nil
	}
	targetAccID := strings.TrimSpace(stringArg(args, "targetAccId"))
	if targetAccID == "" {
		targetAccID = sourceAccID
	}
	client, err := newGraphImportExportClient(ctx)
	if err != nil {
		return graphToolErrorf("[Error] %s: %s", toolName, err.Error())
	}

	waitSec := intArgDefault(args, "waitSec", 300)
	exportTask, err := client.createTask(ctx, sourceAccID, "export", exportBody)
	if err != nil {
		return graphToolErrorf("[Error] %s create export: %v", toolName, err)
	}
	exportTask, exportDetails, err := client.waitTask(ctx, sourceAccID, "export", exportTask.ID, waitSec)
	if err != nil {
		return graphToolErrorf("[Error] %s wait export: %v", toolName, err)
	}
	if err := requireGraphTaskOK("export", exportTask, exportDetails); err != nil {
		failure := map[string]any{
			"exportTask": graphTaskOutput(exportTask, exportDetails),
			"error":      err.Error(),
		}
		if hint := graphTaskFailureHint(err.Error()); hint != "" {
			failure["hint"] = hint
		}
		out, err := json.Marshal(failure)
		if err != nil {
			return graphToolErrorf("[Error] %s: marshal export failure: %v", toolName, err)
		}
		return graphToolErrorf("[Error] %s: %s", toolName, string(out))
	}
	if exportDetails == nil || exportDetails.File == nil || exportDetails.File.FileName == "" {
		return graphToolErrorf("[Error] %s: completed export did not return details.file.fileName", toolName)
	}

	graphBytes, contentType, err := client.download(ctx, exportDetails.File.FileName)
	if err != nil {
		return graphToolErrorf("[Error] %s download export: %v", toolName, err)
	}
	if err := validateGraphArchiveBytes(graphBytes); err != nil {
		return graphToolErrorf("[Error] %s validate export: %v", toolName, err)
	}
	uploadTitle := exportDetails.File.Title
	if uploadTitle == "" {
		uploadTitle = fmt.Sprintf("graph-export-%d.graph", exportTask.ID)
	}
	if filepath.Ext(uploadTitle) == "" {
		uploadTitle += ".graph"
	}
	uploaded, err := client.upload(ctx, targetAccID, uploadTitle, graphBytes)
	if err != nil {
		return graphToolErrorf("[Error] %s upload import archive: %v", toolName, err)
	}

	importArgs := map[string]any{}
	maps.Copy(importArgs, args)
	importArgs["confirmImport"] = true
	delete(importArgs, "users")
	delete(importArgs, "scripts")
	if mappings, ok := args["userMappings"]; ok {
		importArgs["users"] = mappings
	}
	if mappings, ok := args["scriptMappings"]; ok {
		importArgs["scripts"] = mappings
	}
	body, err := buildGraphImportBody(uploaded.FileName, uploaded.Title, importArgs)
	if err != nil {
		return graphToolErrorf("[Error] %s import: %s", toolName, err.Error())
	}
	importTask, err := client.createTask(ctx, targetAccID, "import", body)
	if err != nil {
		return graphToolErrorf("[Error] %s create import: %v", toolName, err)
	}
	importTask, importDetails, err := client.waitTask(ctx, targetAccID, "import", importTask.ID, waitSec)
	if err != nil {
		return graphToolErrorf("[Error] %s wait import: %v", toolName, err)
	}
	if err := requireGraphTaskOK("import", importTask, importDetails); err != nil {
		failure := map[string]any{
			"exportTask":   graphTaskOutput(exportTask, exportDetails),
			"uploadedFile": uploaded,
			"importTask":   graphTaskOutput(importTask, importDetails),
			"error":        err.Error(),
		}
		if hint := graphTaskFailureHint(err.Error()); hint != "" {
			failure["hint"] = hint
		}
		out, err := json.Marshal(failure)
		if err != nil {
			return graphToolErrorf("[Error] %s: marshal import failure: %v", toolName, err)
		}
		return graphToolErrorf("[Error] %s: %s", toolName, string(out))
	}

	warnings := []string{
		"Graph import creates or updates workspace objects. Use replace with a unique refReplacePrefix for normal cloning; reuse can update existing matching REF objects.",
		"By default the import target is the current/source workspace in the same Simulator environment. To import elsewhere, pass targetAccId for that workspace or switch the MCP environment first.",
		"Server export recursion can include substantially more than the visible graph/layer. Review manifest counters before using this on production workspaces.",
	}
	if targetAccID == sourceAccID {
		warnings = append(warnings, "The target workspace is the same as the source workspace. With refStrategy=replace this creates duplicated objects under refReplacePrefix; with reuse it can update existing REF-matching objects.")
	}
	target, targetWarnings := resolveGraphCloneTarget(ctx, client, targetAccID, graphBytes, args, subject, importTask, importDetails)
	warnings = append(warnings, targetWarnings...)

	result := map[string]any{
		"sourceAccId":        sourceAccID,
		"targetAccId":        targetAccID,
		"exportTask":         graphTaskOutput(exportTask, exportDetails),
		"exportArchiveBytes": len(graphBytes),
		"exportContentType":  contentType,
		"uploadedFile":       uploaded,
		"importTask":         graphTaskOutput(importTask, importDetails),
		"refReplacePrefix":   stringArg(args, "refReplacePrefix"),
		"importRefStrategy":  stringArgDefault(args, "refStrategy", "replace"),
		"warnings":           warnings,
	}
	if len(target) > 0 {
		result["target"] = target
	}
	if exportCounters, importCounters := graphTaskCounters(exportDetails), graphTaskCounters(importDetails); exportCounters != nil && importCounters != nil {
		result["countersMatch"] = graphCountersEqual(exportCounters, importCounters)
	}
	maps.Copy(result, subject)
	return graphToolJSON(toolName, result)
}

func readLocalGraphArchive(localPath string) (data []byte, filename string, err error) {
	p := strings.TrimSpace(localPath)
	if p == "" {
		return nil, "", errors.New("localPath is empty")
	}
	if !filepath.IsAbs(p) {
		return nil, "", errors.New("localPath must be absolute")
	}
	if strings.ToLower(filepath.Ext(p)) != ".graph" {
		return nil, "", errors.New("localPath must point to a .graph archive")
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, "", err
	}
	if info.IsDir() {
		return nil, "", errors.New("localPath points to a directory")
	}
	if info.Size() > graphArchiveLocalCap {
		return nil, "", fmt.Errorf("localPath file is too large (%d bytes; max %d)", info.Size(), graphArchiveLocalCap)
	}
	data, err = os.ReadFile(p)
	if err != nil {
		return nil, "", err
	}
	if err := validateGraphArchiveBytes(data); err != nil {
		return nil, "", err
	}
	return data, filepath.Base(p), nil
}

func validateGraphArchiveBytes(data []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid .graph zip archive: %w", err)
	}
	hasManifest := false
	hasActor := false
	hasForm := false
	for _, f := range reader.File {
		switch {
		case f.Name == "GraphManifest.yaml":
			hasManifest = true
		case strings.HasPrefix(f.Name, "topology/actors/") && strings.HasSuffix(f.Name, ".yaml"):
			hasActor = true
		case strings.HasPrefix(f.Name, "forms/") && strings.HasSuffix(f.Name, ".yaml"):
			hasForm = true
		}
	}
	if !hasManifest {
		return errors.New(".graph archive is missing GraphManifest.yaml")
	}
	if !hasActor && !hasForm {
		return errors.New(".graph archive must contain at least one actor or form yaml")
	}
	return nil
}

func extractGraphArchiveActorMetadata(data []byte, ids []string) (map[string]graphArchiveActorMeta, error) {
	want := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return map[string]graphArchiveActorMeta{}, nil
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid .graph zip archive: %w", err)
	}
	out := map[string]graphArchiveActorMeta{}
	for _, f := range reader.File {
		if !strings.HasPrefix(f.Name, "topology/actors/") || !strings.HasSuffix(f.Name, ".yaml") {
			continue
		}
		id := strings.TrimSuffix(path.Base(f.Name), ".yaml")
		if !want[id] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(rc, 1<<20))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close %s: %w", f.Name, closeErr)
		}
		var actor map[string]any
		if err := yaml.Unmarshal(raw, &actor); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.Name, err)
		}
		meta := graphArchiveActorMeta{
			ID:     firstString(actor, "id"),
			FormID: firstInt(actor, "formId", "formID", "form_id"),
			Ref:    firstString(actor, "ref"),
			Title:  firstString(actor, "title"),
		}
		if meta.ID == "" {
			meta.ID = id
		}
		out[id] = meta
	}
	return out, nil
}

func firstString(m map[string]any, names ...string) string {
	for _, name := range names {
		if s, ok := m[name].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstInt(m map[string]any, names ...string) int {
	for _, name := range names {
		if n := toInt(m[name]); n > 0 {
			return n
		}
	}
	return 0
}

func encodeGraphArchivePath(fileName string) string {
	parts := strings.Split(fileName, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func graphWebBaseURL(ctx context.Context) string {
	base := strings.TrimRight(ecore.BuildBaseURLForContext(ctx), "/")
	for _, suffix := range []string{"/papi/1.0", "/api/1.0", "/papi", "/api"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return strings.TrimRight(base, "/")
}

func graphShortAcc(acc string) string {
	if len(acc) > 8 && acc[8] == '-' {
		return acc[:8]
	}
	return acc
}

func buildGraphLayerURL(ctx context.Context, accID, graphID, layerID string) string {
	base := graphWebBaseURL(ctx)
	if base == "" || accID == "" || graphID == "" {
		return ""
	}
	linkPath := fmt.Sprintf("%s/actors_graph/%s/graph/%s/layers", base, url.PathEscape(graphShortAcc(accID)), url.PathEscape(graphID))
	if layerID != "" {
		linkPath += "/" + url.PathEscape(layerID)
	}
	return linkPath
}

func resolveGraphCloneTarget(ctx context.Context, client *graphImportExportClient, targetAccID string, graphBytes []byte, args, subject map[string]any, importTask *graphTask, importDetails *graphTaskDetails) (target map[string]any, warnings []string) { //nolint:gocyclo // REF and no-REF target resolution paths stay together for consistent warnings.
	target = map[string]any{}
	warnings = []string{}
	if importDetails != nil && importDetails.Actor != nil && importDetails.Actor.ID != "" {
		target["importActor"] = importDetails.Actor
	}
	if stringArgDefault(args, "refStrategy", "replace") != "replace" {
		warnings = append(warnings, "Target URL was not resolved automatically because refStrategy is not replace; resolve the imported object manually or use replace with a unique refReplacePrefix.")
		return target, warnings
	}
	prefix := strings.TrimSpace(stringArg(args, "refReplacePrefix"))
	if prefix == "" {
		return target, warnings
	}

	sourceGraphID := strings.TrimSpace(stringArg(subject, "sourceGraphId"))
	sourceLayerID := strings.TrimSpace(stringArg(subject, "sourceLayerId"))
	if sourceLayerID == "" {
		sourceLayerID = strings.TrimSpace(stringArg(subject, "layerId"))
	}
	sourceIDs := []string{}
	if sourceGraphID != "" && sourceGraphID != "0" {
		sourceIDs = append(sourceIDs, sourceGraphID)
	}
	if sourceLayerID != "" {
		sourceIDs = append(sourceIDs, sourceLayerID)
	}
	if actorIDs, err := actorIDSliceArg(subject); err == nil {
		for _, actorID := range actorIDs {
			if actorID != "" {
				sourceIDs = append(sourceIDs, actorID)
			}
		}
	}
	metas, err := extractGraphArchiveActorMetadata(graphBytes, sourceIDs)
	if err != nil {
		warnings = append(warnings, "Target URL was not resolved automatically: "+err.Error())
		return target, warnings
	}

	targetActors := map[string]graphResolvedActor{}
	resolve := func(sourceID string, warnMissing bool) *graphResolvedActor {
		if sourceID == "" {
			return nil
		}
		if resolved, ok := targetActors[sourceID]; ok {
			return &resolved
		}
		meta, ok := metas[sourceID]
		if !ok {
			if warnMissing {
				warnings = append(warnings, fmt.Sprintf("Target actor for source %s was not resolved: source actor metadata was not present in the export archive.", sourceID))
			}
			return nil
		}
		if meta.FormID <= 0 || meta.Ref == "" {
			resolved, reason, err := resolveImportedActorWithoutRef(ctx, client, targetAccID, sourceID, meta, importTask)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Target actor for source %s was not resolved without ref metadata: %v", sourceID, err))
				return nil
			}
			if reason != "" {
				warnings = append(warnings, reason)
			}
			if resolved == nil {
				warnings = append(warnings, fmt.Sprintf("Target actor for source %s was not resolved: exported actor has no formId/ref metadata.", sourceID))
				return nil
			}
			targetActors[sourceID] = *resolved
			return resolved
		}
		resolved, err := client.getActorByRef(ctx, meta.FormID, prefix+meta.Ref)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Target actor for source %s was not resolved by ref: %v", sourceID, err))
			return nil
		}
		targetActors[sourceID] = *resolved
		return resolved
	}

	var targetGraph *graphResolvedActor
	if sourceGraphID != "" && sourceGraphID != "0" {
		targetGraph = resolve(sourceGraphID, stringArg(subject, "sourceKind") != "layer")
	}
	targetLayer := resolve(sourceLayerID, true)
	if len(targetActors) > 0 {
		target["actorsBySourceId"] = targetActors
	}
	if targetGraph != nil {
		target["graphId"] = targetGraph.ID
	}
	if targetLayer != nil {
		target["layerId"] = targetLayer.ID
	}
	switch {
	case targetGraph != nil && targetLayer != nil:
		target["url"] = buildGraphLayerURL(ctx, targetAccID, targetGraph.ID, targetLayer.ID)
	case targetLayer != nil:
		target["url"] = buildGraphLayerURL(ctx, targetAccID, targetLayer.ID, "")
	case targetGraph != nil:
		target["url"] = buildGraphLayerURL(ctx, targetAccID, targetGraph.ID, "")
	}
	return target, warnings
}

func resolveImportedActorWithoutRef(ctx context.Context, client *graphImportExportClient, targetAccID, sourceID string, meta graphArchiveActorMeta, importTask *graphTask) (*graphResolvedActor, string, error) {
	title := strings.TrimSpace(meta.Title)
	formID := meta.FormID
	if title == "" || formID <= 0 {
		source, err := client.getActorMetadata(ctx, sourceID)
		if err != nil {
			return nil, "", err
		}
		if title == "" {
			title = strings.TrimSpace(source.Title)
		}
		if formID <= 0 {
			formID = source.FormID
		}
	}
	if title == "" || formID <= 0 {
		return nil, "", nil
	}
	candidates := []graphResolvedActor{}
	seenCandidates := map[string]bool{}
	for _, query := range graphActorFallbackSearchQueries(title) {
		found, err := client.searchActorsByTitle(ctx, targetAccID, query, formID, 50)
		if err != nil {
			return nil, "", err
		}
		for _, candidate := range found {
			if candidate.ID == "" || seenCandidates[candidate.ID] {
				continue
			}
			seenCandidates[candidate.ID] = true
			candidates = append(candidates, candidate)
		}
	}
	minCreatedAt := graphImportStartedAt(importTask)
	matches := make([]graphResolvedActor, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == "" || candidate.ID == sourceID {
			continue
		}
		if candidate.Title != title {
			continue
		}
		if formID > 0 && candidate.FormID != formID {
			continue
		}
		if minCreatedAt > 0 {
			if candidate.CreatedAt == 0 || candidate.CreatedAt < minCreatedAt-2 {
				continue
			}
		}
		matches = append(matches, candidate)
	}
	switch len(matches) {
	case 0:
		return nil, "", nil
	case 1:
		return &matches[0], fmt.Sprintf("Target actor for source %s was resolved by title/formId fallback because the source actor has no ref; verify the returned URL before using it in production.", sourceID), nil
	default:
		return nil, "", fmt.Errorf("found %d candidate actors named %q with formId=%d after import; not guessing", len(matches), title, formID)
	}
}

func graphActorFallbackSearchQueries(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	queries := []string{title}
	parts := strings.Fields(title)
	if len(parts) > 4 {
		short := strings.Join(parts[:4], " ")
		if short != title {
			queries = append(queries, short)
		}
	}
	return queries
}

func graphImportStartedAt(task *graphTask) int64 {
	if task == nil {
		return 0
	}
	if task.TimeStart != nil && *task.TimeStart > 0 {
		return *task.TimeStart
	}
	return task.CreatedAt
}

func workspaceArg(ctx context.Context, args map[string]any, name string) string {
	if s := strings.TrimSpace(stringArg(args, name)); s != "" {
		return s
	}
	return ecore.WorkspaceIDForContext(ctx)
}

func stringArg(args map[string]any, name string) string {
	if args == nil {
		return ""
	}
	switch v := args[name].(type) {
	case string:
		return v
	default:
		return ""
	}
}

func stringArgDefault(args map[string]any, name, def string) string {
	if v := stringArg(args, name); v != "" {
		return v
	}
	return def
}

func boolArg(args map[string]any, name string) bool {
	if args == nil {
		return false
	}
	v, ok := args[name]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(val)
		return b
	default:
		return false
	}
}

func boolArgDefault(args map[string]any, name string, def bool) bool {
	if args == nil {
		return def
	}
	if _, ok := args[name]; !ok {
		return def
	}
	return boolArg(args, name)
}

func intArg(args map[string]any, name string) int {
	if args == nil {
		return 0
	}
	return toInt(args[name])
}

func intArgDefault(args map[string]any, name string, def int) int {
	if args == nil {
		return def
	}
	if _, ok := args[name]; !ok {
		return def
	}
	v := intArg(args, name)
	if v == 0 {
		return def
	}
	return v
}

func actorIDSliceArg(args map[string]any) ([]string, error) {
	if args == nil {
		return nil, nil
	}
	raw, ok := args["actorIds"]
	if !ok || raw == nil {
		return nil, nil
	}
	if arr, ok := raw.([]string); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if strings.TrimSpace(item) == "" {
				return nil, errors.New("actorIds must contain non-empty strings")
			}
			out = append(out, strings.TrimSpace(item))
		}
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("actorIds must be an array")
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, errors.New("actorIds must contain non-empty strings")
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func intSliceArg(args map[string]any, name string) ([]int, error) {
	if args == nil {
		return nil, nil
	}
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, nil
	}
	if arr, ok := raw.([]int); ok {
		out := make([]int, 0, len(arr))
		for _, n := range arr {
			if n <= 0 {
				return nil, fmt.Errorf("%s must contain positive numeric form ids", name)
			}
			out = append(out, n)
		}
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		n := toInt(item)
		if n <= 0 {
			return nil, fmt.Errorf("%s must contain positive numeric form ids", name)
		}
		out = append(out, n)
	}
	return out, nil
}

func objectSliceArg(args map[string]any, name string) ([]any, error) {
	if args == nil {
		return []any{}, nil
	}
	raw, ok := args[name]
	if !ok || raw == nil {
		return []any{}, nil
	}
	if arr, ok := raw.([]map[string]any); ok {
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, item)
		}
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	return arr, nil
}

func maxRecursionLevelArg(args map[string]any) (level int, ok bool, err error) {
	if args == nil {
		return 0, false, nil
	}
	raw, ok := args["maxRecursionLevel"]
	if !ok || raw == nil {
		return 0, false, nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" || s == "max" {
			return 0, false, nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, false, errors.New("maxRecursionLevel must be \"max\" or an integer 1..10")
		}
		if n < 1 || n > 10 {
			return 0, false, errors.New("maxRecursionLevel must be 1..10")
		}
		return n, true, nil
	}
	n := toInt(raw)
	if n < 1 || n > 10 {
		return 0, false, errors.New("maxRecursionLevel must be 1..10")
	}
	return n, true, nil
}

func registerGraphImportExportTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("createGraphExportTask",
			mcp.WithDescription("Start an async Simulator .graph export task. To export/clone a layer, pass the layer actor UUID in actorIds. To export a graph, pass the graph root actor UUID. The server recursively exports referenced objects: maxRecursionLevel=max is the safest for a later import but can include much more than the visible graph/layer; lower numeric levels may create an archive that later fails to import. Export can fail if the current user lacks access to any object included by recursion. Review completed manifest counters before production use."),
			mcp.WithString("accId", mcp.Description("Workspace id. Defaults to the configured/request workspace.")),
			mcp.WithArray("actorIds", mcp.Description("Actor UUIDs to export. For a graph layer, include the layer actor UUID.")),
			mcp.WithArray("formIds", mcp.Description("Numeric form ids to export.")),
			mcp.WithBoolean("allWorkspace", mcp.Description("Export the whole workspace. Requires confirmAllWorkspaceExport=true.")),
			mcp.WithBoolean("confirmAllWorkspaceExport", mcp.Description("Required guard for allWorkspace=true because it can export a large/sensitive archive.")),
			mcp.WithString("maxRecursionLevel", mcp.Description("Reference recursion depth: \"max\" (default) or integer 1..10. For imports, \"max\" is safest; low values can be incomplete.")),
			mcp.WithBoolean("attachments", mcp.Description("Include file attachments. Default false in MCP to avoid unintentionally exporting files.")),
			mcp.WithBoolean("transactions", mcp.Description("Include transactions. Default false.")),
			mcp.WithBoolean("processes", mcp.Description("Include linked Corezoid processes/projects/stages. Default false.")),
			mcp.WithBoolean("users", mcp.Description("Include users/groups access mapping data. Default false.")),
			mcp.WithBoolean("balances", mcp.Description("Include balances. Default false.")),
			mcp.WithBoolean("connectorsToAccounts", mcp.Description("Include connector-to-account links. Default false.")),
			mcp.WithBoolean("systemCounters", mcp.Description("Include system counters. Default false.")),
			mcp.WithBoolean("accountToActors", mcp.Description("Include account-to-actor links. Default false.")),
			mcp.WithNumber("waitSec", mcp.Description("Optional seconds to poll for completion before returning. Without it, returns the created task.")),
		),
		handleCreateGraphExportTask,
	)

	s.AddTool(
		mcp.NewTool("getGraphImportExportTask",
			mcp.WithDescription("List or inspect async Simulator graph import/export tasks. Completed export tasks carry details.file.fileName for download/import and details.manifest.stats.counters for review. Failed tasks carry details.errMsg."),
			mcp.WithString("accId", mcp.Description("Workspace id. Defaults to the configured/request workspace.")),
			mcp.WithString("name", mcp.Description("Task type: export or import. Default export.")),
			mcp.WithNumber("taskId", mcp.Description("Optional task id to inspect directly. Uses the direct task endpoint before falling back to list lookup.")),
			mcp.WithNumber("limit", mcp.Description("Page size. Default 15.")),
			mcp.WithNumber("offset", mcp.Description("Page offset. Default 0.")),
		),
		handleGetGraphImportExportTask,
	)

	s.AddTool(
		mcp.NewTool("importGraphArchive",
			mcp.WithDescription("Start an async Simulator .graph import task from an uploaded fileName or a local .graph archive. This is a write operation: it can create many actors/forms/edges/accounts and, with refStrategy=reuse, update existing REF-matching objects. Normal cloning should use refStrategy=replace plus a unique refReplacePrefix. Requires confirmImport=true."),
			mcp.WithString("accId", mcp.Description("Target workspace id. Defaults to the configured/request workspace.")),
			mcp.WithString("fileName", mcp.Description("Already uploaded storage fileName of the .graph archive. Provide exactly one of fileName or localPath.")),
			mcp.WithString("localPath", mcp.Description("Absolute path to a local .graph archive on the MCP server host. The file is validated as a .graph zip before upload. Provide exactly one of fileName or localPath.")),
			mcp.WithString("title", mcp.Description("Optional import file title. Defaults to the archive basename/title.")),
			mcp.WithString("refStrategy", mcp.Description("REF collision strategy: replace (default, creates objects with prefixed refs), reuse (updates matching existing refs; requires allowReuseImport=true), or error.")),
			mcp.WithString("refReplacePrefix", mcp.Description("Required only with refStrategy=replace. Use a unique prefix, e.g. clone_20260724_. Omit for refStrategy=error or reuse.")),
			mcp.WithBoolean("confirmImport", mcp.Description("Required true guard acknowledging that import writes objects into the workspace.")),
			mcp.WithBoolean("allowReuseImport", mcp.Description("Required true only when refStrategy=reuse, because reuse can update existing REF-matching objects.")),
			mcp.WithArray("users", mcp.Description("Optional user/group mapping array, same shape as UI: {fromId,toIds[]}.")),
			mcp.WithArray("scripts", mcp.Description("Optional Corezoid script credential mapping array, same shape as UI import task.")),
			mcp.WithNumber("waitSec", mcp.Description("Optional seconds to poll for completion before returning. Without it, returns the created task.")),
		),
		handleImportGraphArchive,
	)

	s.AddTool(
		mcp.NewTool("cloneGraphLayer",
			mcp.WithDescription("Clone/copy a Simulator graph layer through the official async export/import flow: export layer actor to a .graph archive, wait for completion, download, upload into the target workspace, then import with the chosen REF strategy. This does not delete the source layer. Accepts either layerId or sourceUrl; sourceUrl can be a Simulator /actors_graph/<workspace>/graph/<graphId>/layers/<layerId> link. On successful replace imports, the result tries to include target.layerId and target.url for the cloned layer. This is a convenience wrapper over cloneGraphObjects. Export can fail if the current user lacks access to any object included by layer recursion. This can create many objects; use replace with a unique refReplacePrefix for normal cloning. Requires confirmClone=true. Avoid casual use in production workspaces until manifest counters are reviewed."),
			mcp.WithString("layerId", mcp.Description("Source layer actor UUID. Optional when sourceUrl contains /layers/<layerId>.")),
			mcp.WithString("sourceUrl", mcp.Description("Optional Simulator graph/layer URL. For layer clone, must contain /layers/<layerId>; sourceAccId is inferred from the URL when possible.")),
			mcp.WithString("sourceAccId", mcp.Description("Source workspace id. Defaults to the configured/request workspace.")),
			mcp.WithString("targetAccId", mcp.Description("Target workspace id in the current Simulator environment. Defaults to sourceAccId, which itself defaults to the configured/request workspace.")),
			mcp.WithString("maxRecursionLevel", mcp.Description("Reference recursion depth for export: \"max\" (default) or integer 1..10. Full clone/import is safest with max; low values can be incomplete.")),
			mcp.WithBoolean("attachments", mcp.Description("Include attachments in export/import. Default false.")),
			mcp.WithBoolean("transactions", mcp.Description("Include transactions. Default false.")),
			mcp.WithBoolean("processes", mcp.Description("Include linked Corezoid processes/projects/stages. Default false.")),
			mcp.WithBoolean("users", mcp.Description("Include users/groups access data. Default false; unmapped imported access can grant rights to the importing user.")),
			mcp.WithBoolean("balances", mcp.Description("Include balances. Default false.")),
			mcp.WithBoolean("connectorsToAccounts", mcp.Description("Include connector-to-account links. Default false.")),
			mcp.WithBoolean("systemCounters", mcp.Description("Include system counters. Default false.")),
			mcp.WithBoolean("accountToActors", mcp.Description("Include account-to-actor links. Default false.")),
			mcp.WithString("refStrategy", mcp.Description("Import REF collision strategy: replace (default), reuse, or error. reuse requires allowReuseImport=true.")),
			mcp.WithString("refReplacePrefix", mcp.Description("Required only with refStrategy=replace. Unique prefix for replace strategy, e.g. clone_20260724_. Omit for refStrategy=error or reuse; non-empty values are rejected for those strategies.")),
			mcp.WithBoolean("confirmClone", mcp.Description("Required true guard acknowledging that clone imports objects into the target workspace.")),
			mcp.WithBoolean("allowReuseImport", mcp.Description("Required true only when refStrategy=reuse, because reuse can update existing REF-matching objects.")),
			mcp.WithArray("userMappings", mcp.Description("Optional import user/group mapping array, same shape as UI: {fromId,toIds[]}. If omitted and users are included in the export, access import behavior follows the backend default.")),
			mcp.WithArray("scriptMappings", mcp.Description("Optional Corezoid script credential mapping array, same shape as UI import task.")),
			mcp.WithNumber("waitSec", mcp.Description("Seconds to wait for each async export/import task. Default 300.")),
		),
		handleCloneGraphLayer,
	)

	s.AddTool(
		mcp.NewTool("cloneGraphObjects",
			mcp.WithDescription("Clone/copy selected Simulator graph objects through the official async export/import flow. This does not delete source objects. Pass sourceUrl, graph/layer actor UUIDs in actorIds, optional formIds, or allWorkspace=true with explicit confirmation. sourceUrl can be a Simulator /actors_graph/<workspace>/graph/<graphId>/layers/<layerId> link; sourceKind=auto clones the layer when the URL has /layers/<layerId>, otherwise the graph root. On successful replace imports, the result tries to include target.graphId/target.layerId/target.url for the cloned graph/layer. Graph root export can complete while later import is rejected by backend validation, so success means final import status=completed. Export can fail if the current user lacks access to any object included by recursion. The import target defaults to the current/source workspace in the same Simulator environment; pass targetAccId to import into another workspace. This can create many objects; use replace with a unique refReplacePrefix for normal cloning. Requires confirmClone=true."),
			mcp.WithString("sourceUrl", mcp.Description("Optional Simulator graph/layer URL. If actorIds/formIds/allWorkspace are omitted, the tool derives actorIds from this URL and sourceKind.")),
			mcp.WithString("sourceKind", mcp.Description("How to interpret sourceUrl when deriving actorIds: auto (default; layer URL clones layer, graph URL clones graph), layer, or graph.")),
			mcp.WithArray("actorIds", mcp.Description("Actor UUIDs to clone. Use the graph root UUID to clone a graph, or a layer UUID to clone a layer.")),
			mcp.WithArray("formIds", mcp.Description("Numeric form ids to clone with their referenced objects.")),
			mcp.WithBoolean("allWorkspace", mcp.Description("Clone/export the whole workspace. Requires confirmAllWorkspaceExport=true and confirmClone=true.")),
			mcp.WithBoolean("confirmAllWorkspaceExport", mcp.Description("Required guard for allWorkspace=true because it can export a large/sensitive archive.")),
			mcp.WithString("sourceAccId", mcp.Description("Source workspace id. Defaults to the configured/request workspace.")),
			mcp.WithString("targetAccId", mcp.Description("Target workspace id in the current Simulator environment. Defaults to sourceAccId, which itself defaults to the configured/request workspace.")),
			mcp.WithString("maxRecursionLevel", mcp.Description("Reference recursion depth for export: \"max\" (default) or integer 1..10. Full clone/import is safest with max; low values can be incomplete.")),
			mcp.WithBoolean("attachments", mcp.Description("Include attachments in export/import. Default false.")),
			mcp.WithBoolean("transactions", mcp.Description("Include transactions. Default false.")),
			mcp.WithBoolean("processes", mcp.Description("Include linked Corezoid processes/projects/stages. Default false.")),
			mcp.WithBoolean("users", mcp.Description("Include users/groups access data. Default false; unmapped imported access can grant rights to the importing user.")),
			mcp.WithBoolean("balances", mcp.Description("Include balances. Default false.")),
			mcp.WithBoolean("connectorsToAccounts", mcp.Description("Include connector-to-account links. Default false.")),
			mcp.WithBoolean("systemCounters", mcp.Description("Include system counters. Default false.")),
			mcp.WithBoolean("accountToActors", mcp.Description("Include account-to-actor links. Default false.")),
			mcp.WithString("refStrategy", mcp.Description("Import REF collision strategy: replace (default), reuse, or error. reuse requires allowReuseImport=true.")),
			mcp.WithString("refReplacePrefix", mcp.Description("Required only with refStrategy=replace. Unique prefix for replace strategy, e.g. clone_20260724_. Omit for refStrategy=error or reuse; non-empty values are rejected for those strategies.")),
			mcp.WithBoolean("confirmClone", mcp.Description("Required true guard acknowledging that clone imports objects into the target workspace.")),
			mcp.WithBoolean("allowReuseImport", mcp.Description("Required true only when refStrategy=reuse, because reuse can update existing REF-matching objects.")),
			mcp.WithArray("userMappings", mcp.Description("Optional import user/group mapping array, same shape as UI: {fromId,toIds[]}. If omitted and users are included in the export, access import behavior follows the backend default.")),
			mcp.WithArray("scriptMappings", mcp.Description("Optional Corezoid script credential mapping array, same shape as UI import task.")),
			mcp.WithNumber("waitSec", mcp.Description("Seconds to wait for each async export/import task. Default 300.")),
		),
		handleCloneGraphObjects,
	)
}
