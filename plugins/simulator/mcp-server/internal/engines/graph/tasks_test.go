//nolint:testpackage // White-box tests cover unexported safety guards and handlers.
package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func graphTaskTestContext(t *testing.T, baseURL string) context.Context {
	t.Helper()
	ecore.SetStateless(true)
	t.Cleanup(func() { ecore.SetStateless(false) })

	ctx := apiclient.WithAuthorization(context.Background(), "Simulator test-token")
	ctx = apiclient.WithWorkspaceID(ctx, "test-workspace")
	return apiclient.WithBaseURL(ctx, baseURL)
}

func callGraphTaskHandler(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), ctx context.Context, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned a nil result")
	}
	return res
}

func graphTaskResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is not text: %#v", result.Content[0])
	}
	return text.Text
}

func replaceImportArgs() map[string]any {
	return map[string]any{
		"fileName":                    "bucket/export.graph",
		"confirmImport":               true,
		"actorRefStrategy":            graphImportStrategyReplace,
		"actorRefReplacePrefix":       "clone_20260811_",
		"formRefStrategy":             graphImportStrategyReplace,
		"formRefReplacePrefix":        "clone_20260811_",
		"transferRefStrategy":         graphImportStrategyReplace,
		"transferRefReplacePrefix":    "clone_20260811_",
		"transactionRefStrategy":      graphImportStrategyReplace,
		"transactionRefReplacePrefix": "clone_20260811_",
		"processesRefStrategy":        graphImportStrategyReplace,
	}
}

func TestHandleExportGraphRequiresAllWorkspaceConfirmationBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer srv.Close()

	res := callGraphTaskHandler(t, handleExportGraph, graphTaskTestContext(t, srv.URL), map[string]any{
		"allWorkspace": true,
	})
	if !res.IsError || !strings.Contains(graphTaskResultText(t, res), "confirmAllWorkspaceExport=true") {
		t.Fatalf("expected confirmation error, got %#v", res)
	}
	if requests != 0 {
		t.Fatalf("unsafe export made %d backend requests", requests)
	}
}

func TestHandleExportGraphConfirmedAllWorkspacePayload(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/test-workspace/export" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"data":{"id":701,"name":"export","status":"created"}}`))
	}))
	defer srv.Close()

	res := callGraphTaskHandler(t, handleExportGraph, graphTaskTestContext(t, srv.URL), map[string]any{
		"allWorkspace":              true,
		"confirmAllWorkspaceExport": true,
		"attachments":               false,
		"transactions":              false,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", graphTaskResultText(t, res))
	}
	if body["allWorkspace"] != true {
		t.Fatalf("allWorkspace missing from request: %#v", body)
	}
	if _, exists := body["confirmAllWorkspaceExport"]; exists {
		t.Fatalf("confirmation guard leaked into backend payload: %#v", body)
	}
	ops, ok := body["ops"].(map[string]any)
	if !ok || ops["attachments"] != false || ops["transactions"] != false {
		t.Fatalf("export options not preserved: %#v", body)
	}
}

func TestHandleExportGraphSelectedActorDoesNotRequireBroadConfirmation(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":{"id":701,"name":"export","status":"created"}}`))
	}))
	defer srv.Close()

	res := callGraphTaskHandler(t, handleExportGraph, graphTaskTestContext(t, srv.URL), map[string]any{
		"actors": []any{"11111111-1111-4111-8111-111111111111"},
	})
	if res.IsError {
		t.Fatalf("selected-actor export should not need broad confirmation: %s", graphTaskResultText(t, res))
	}
	if requests != 1 {
		t.Fatalf("selected-actor export made %d requests, want 1", requests)
	}
}

func TestBuildGraphImportOpsGuardrails(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{
			name: "confirmation required",
			edit: func(args map[string]any) { delete(args, "confirmImport") },
			want: "confirmImport=true",
		},
		{
			name: "every strategy explicit",
			edit: func(args map[string]any) { delete(args, "formRefStrategy") },
			want: "formRefStrategy is required",
		},
		{
			name: "strategy allow list",
			edit: func(args map[string]any) { args["actorRefStrategy"] = "overwrite" },
			want: "actorRefStrategy is required",
		},
		{
			name: "replace prefix required",
			edit: func(args map[string]any) { delete(args, "actorRefReplacePrefix") },
			want: "actorRefReplacePrefix must be at least",
		},
		{
			name: "short replace prefix rejected",
			edit: func(args map[string]any) { args["transactionRefReplacePrefix"] = "copy_" },
			want: "transactionRefReplacePrefix must be at least",
		},
		{
			name: "reuse prefix rejected",
			edit: func(args map[string]any) {
				args["actorRefStrategy"] = graphImportStrategyReuse
				args["allowReuseImport"] = true
			},
			want: "actorRefReplacePrefix must be omitted",
		},
		{
			name: "reuse acknowledgement required",
			edit: func(args map[string]any) {
				args["actorRefStrategy"] = graphImportStrategyReuse
				delete(args, "actorRefReplacePrefix")
			},
			want: "allowReuseImport=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := replaceImportArgs()
			tt.edit(args)
			_, err := buildGraphImportOps(args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestBuildGraphImportOpsAllowsExplicitReuse(t *testing.T) {
	args := replaceImportArgs()
	args["actorRefStrategy"] = graphImportStrategyReuse
	delete(args, "actorRefReplacePrefix")
	args["allowReuseImport"] = true

	ops, err := buildGraphImportOps(args)
	if err != nil {
		t.Fatalf("explicit reuse should be accepted: %v", err)
	}
	if ops["actorRefStrategy"] != graphImportStrategyReuse {
		t.Fatalf("reuse strategy missing from ops: %#v", ops)
	}
	if _, exists := ops["actorRefReplacePrefix"]; exists {
		t.Fatalf("reuse prefix leaked into ops: %#v", ops)
	}
}

func TestHandleImportGraphGuardRunsBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer srv.Close()

	args := replaceImportArgs()
	delete(args, "confirmImport")
	res := callGraphTaskHandler(t, handleImportGraph, graphTaskTestContext(t, srv.URL), args)
	if !res.IsError || !strings.Contains(graphTaskResultText(t, res), "confirmImport=true") {
		t.Fatalf("expected confirmation error, got %#v", res)
	}
	if requests != 0 {
		t.Fatalf("unsafe import made %d backend requests", requests)
	}
}

func TestHandleImportGraphSendsValidatedStrategiesOnly(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/test-workspace/import" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"data":{"id":702,"name":"import","status":"created"}}`))
	}))
	defer srv.Close()

	args := replaceImportArgs()
	res := callGraphTaskHandler(t, handleImportGraph, graphTaskTestContext(t, srv.URL), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", graphTaskResultText(t, res))
	}
	if body["fileName"] != "bucket/export.graph" {
		t.Fatalf("fileName missing from request: %#v", body)
	}
	if _, exists := body["confirmImport"]; exists {
		t.Fatalf("confirmation guard leaked into backend payload: %#v", body)
	}
	if _, exists := body["allowReuseImport"]; exists {
		t.Fatalf("reuse guard leaked into backend payload: %#v", body)
	}
	ops, ok := body["ops"].(map[string]any)
	if !ok || len(ops) != 9 {
		t.Fatalf("expected all five strategies and four prefixes, got %#v", body)
	}
	if ops["actorRefStrategy"] != graphImportStrategyReplace || ops["processesRefStrategy"] != graphImportStrategyReplace {
		t.Fatalf("validated strategies not preserved: %#v", ops)
	}
}

func TestHandleUploadGraphFileRequiresExactlyOneSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("ambiguous upload must not make a backend request")
	}))
	defer srv.Close()
	ctx := graphTaskTestContext(t, srv.URL)

	for _, args := range []map[string]any{
		{},
		{"base64": "Z3JhcGg=", "fileUrl": "https://example.com/export.graph"},
	} {
		res := callGraphTaskHandler(t, handleUploadGraphFile, ctx, args)
		if !res.IsError || !strings.Contains(graphTaskResultText(t, res), "exactly one") {
			t.Fatalf("expected exactly-one error for %#v, got %#v", args, res)
		}
	}
}

func TestHandleUploadGraphFileDownloadsAndUploadsSameOriginArchive(t *testing.T) {
	var downloadAuth, uploadContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/download/export.graph":
			downloadAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("graph archive"))
		case r.Method == http.MethodPost && r.URL.Path == "/upload/test-workspace":
			uploadContentType = r.Header.Get("Content-Type")
			if r.URL.Query().Get("ttl") != "0" {
				t.Fatalf("upload ttl = %q, want 0", r.URL.Query().Get("ttl"))
			}
			_, _ = w.Write([]byte(`{"data":{"fileName":"uploads/export.graph"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	res := callGraphTaskHandler(t, handleUploadGraphFile, graphTaskTestContext(t, srv.URL), map[string]any{
		"fileUrl":      srv.URL + "/download/export.graph",
		"originalName": "export.graph",
	})
	if res.IsError {
		t.Fatalf("unexpected upload error: %s", graphTaskResultText(t, res))
	}
	if downloadAuth != "Simulator test-token" {
		t.Fatalf("download authorization = %q", downloadAuth)
	}
	if !strings.HasPrefix(uploadContentType, "multipart/form-data;") {
		t.Fatalf("upload content type = %q", uploadContentType)
	}
	if text := graphTaskResultText(t, res); !strings.Contains(text, `"fileName":"uploads/export.graph"`) {
		t.Fatalf("unexpected upload result: %s", text)
	}
}

func TestHandleGetTaskStatusValidatesBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer srv.Close()
	ctx := graphTaskTestContext(t, srv.URL)

	for _, args := range []map[string]any{
		{"taskId": 0, "name": "export"},
		{"taskId": 701, "name": "unknown"},
	} {
		res := callGraphTaskHandler(t, handleGetTaskStatus, ctx, args)
		if !res.IsError {
			t.Fatalf("expected validation error for %#v", args)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid status calls made %d backend requests", requests)
	}
}

func TestHandleGetTaskStatusUnwrapsDetailsAndBuildsDownloadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/tasks/test-workspace/export/701" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"id":701,"name":"export","status":"completed","details":"{\"file\":{\"fileName\":\"bucket/my graph.graph\"},\"manifest\":{\"stats\":{\"counters\":{\"actors\":3}}}}"}}`))
	}))
	defer srv.Close()

	res := callGraphTaskHandler(t, handleGetTaskStatus, graphTaskTestContext(t, srv.URL), map[string]any{
		"taskId": 701,
		"name":   "export",
	})
	if res.IsError {
		t.Fatalf("unexpected status error: %s", graphTaskResultText(t, res))
	}
	text := graphTaskResultText(t, res)
	for _, want := range []string{
		`"status":"completed"`,
		`"fileName":"bucket/my graph.graph"`,
		`"actors":3`,
		`/download/bucket/my%20graph.graph`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status result missing %q: %s", want, text)
		}
	}
}

func TestDownloadGraphFileFromURLScopesAuthorizationToAPIOrigin(t *testing.T) {
	t.Run("same origin", func(t *testing.T) {
		var authorization string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("graph archive"))
		}))
		defer srv.Close()

		ctx := apiclient.WithAuthorization(context.Background(), "Simulator same-origin-token")
		ctx = apiclient.WithBaseURL(ctx, srv.URL+"/papi/1.0")
		if _, err := downloadGraphFileFromURL(ctx, srv.URL+"/download/export.graph"); err != nil {
			t.Fatalf("download failed: %v", err)
		}
		if authorization != "Simulator same-origin-token" {
			t.Fatalf("same-origin request authorization = %q", authorization)
		}
	})

	t.Run("external origin", func(t *testing.T) {
		var authorization string
		external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("graph archive"))
		}))
		defer external.Close()

		ctx := apiclient.WithAuthorization(context.Background(), "Simulator must-not-leak")
		ctx = apiclient.WithBaseURL(ctx, "https://api.simulator.company/papi/1.0")
		if _, err := downloadGraphFileFromURL(ctx, external.URL+"/export.graph"); err != nil {
			t.Fatalf("download failed: %v", err)
		}
		if authorization != "" {
			t.Fatalf("authorization leaked to external origin: %q", authorization)
		}
	})
}

func TestGraphArchiveToolSchemasThroughMCPHandshake(t *testing.T) {
	srv := server.NewMCPServer("graph-schema-test", "0")
	Register(srv)
	cli, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	var init mcp.InitializeRequest
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "graph-schema-test", Version: "0"}
	if _, err := cli.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	list, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	tools := make(map[string]mcp.Tool, len(list.Tools))
	for _, tool := range list.Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{"exportGraph", "uploadGraphFile", "importGraph", "getTaskStatus"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("tools/list is missing %q", name)
		}
	}

	required := make(map[string]bool)
	for _, name := range tools["importGraph"].InputSchema.Required {
		required[name] = true
	}
	for _, name := range []string{
		"fileName", "actorRefStrategy", "formRefStrategy", "transferRefStrategy",
		"transactionRefStrategy", "processesRefStrategy", "confirmImport",
	} {
		if !required[name] {
			t.Errorf("importGraph schema does not require %q", name)
		}
	}
	raw, err := json.Marshal(tools["importGraph"].InputSchema)
	if err != nil {
		t.Fatalf("marshal importGraph schema: %v", err)
	}
	for _, want := range []string{`"enum":["reuse","replace"]`, `"allowReuseImport"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("importGraph schema missing %s: %s", want, raw)
		}
	}
}
