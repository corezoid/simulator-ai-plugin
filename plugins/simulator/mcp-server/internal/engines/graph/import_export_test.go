//nolint:goconst // Test fixtures intentionally keep API keys and sample values inline.
package graph

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestGraphImportExportBaseURLSelectsWebAPIForBearer(t *testing.T) {
	ecore.Configure("https://mw.simulator.company/papi/1.0", false)
	if got := graphImportExportBaseURL(context.Background(), "Bearer sid"); got != "https://mw.simulator.company/api/1.0" {
		t.Fatalf("Bearer auth should use web API base, got %q", got)
	}
	if got := graphImportExportBaseURL(context.Background(), "Simulator jwt"); got != "https://mw.simulator.company/papi/1.0" {
		t.Fatalf("Simulator auth should keep PAPI base, got %q", got)
	}
	ctx := apiclient.WithBaseURL(context.Background(), "https://tenant.example/papi/1.0")
	if got := graphImportExportBaseURL(ctx, "Bearer sid"); got != "https://tenant.example/api/1.0" {
		t.Fatalf("ctx base override should be transformed safely, got %q", got)
	}
}

func TestBuildGraphExportBodyGuardsAllWorkspace(t *testing.T) {
	if _, err := buildGraphExportBody(map[string]any{"allWorkspace": true}); err == nil {
		t.Fatal("expected allWorkspace export to require confirmation")
	}
	if _, err := buildGraphExportBody(map[string]any{"actorIds": []any{"../../bad"}}); err == nil {
		t.Fatal("expected actorIds UUID validation")
	}
	body, err := buildGraphExportBody(map[string]any{"allWorkspace": true, "confirmAllWorkspaceExport": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["allWorkspace"] != true {
		t.Fatalf("expected allWorkspace body, got %#v", body)
	}
}

func TestBuildGraphExportBodyUsesEmptyArraysForOmittedSelections(t *testing.T) {
	body, err := buildGraphExportBody(map[string]any{
		"actorIds": []any{"11111111-1111-4111-8111-111111111111"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"actors":["11111111-1111-4111-8111-111111111111"]`) || !strings.Contains(text, `"forms":[]`) {
		t.Fatalf("expected live-compatible array selections, got %s", text)
	}

	body, err = buildGraphExportBody(map[string]any{"formIds": []any{123}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err = json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	text = string(raw)
	if !strings.Contains(text, `"actors":[]`) || !strings.Contains(text, `"forms":[123]`) {
		t.Fatalf("expected live-compatible array selections, got %s", text)
	}
}

func TestBuildGraphExportBodyMirrorsUISchemaFlags(t *testing.T) {
	body, err := buildGraphExportBody(map[string]any{
		"actorIds":             []any{"11111111-1111-4111-8111-111111111111"},
		"formIds":              []any{123},
		"attachments":          true,
		"transactions":         true,
		"processes":            true,
		"users":                true,
		"balances":             true,
		"connectorsToAccounts": true,
		"systemCounters":       true,
		"accountToActors":      true,
		"maxRecursionLevel":    3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ops := requireOpsMap(t, body)
	for _, name := range []string{"attachments", "transactions", "processes", "users", "balances", "connectorsToAccounts", "accountToActors"} {
		if ops[name] != true {
			t.Fatalf("expected ops.%s=true in UI-compatible export body, got %#v", name, ops)
		}
	}
	if ops["maxRecursionLevel"] != 3 {
		t.Fatalf("expected ops.maxRecursionLevel=3, got %#v", ops)
	}
	for _, name := range []string{"balances", "connectorsToAccounts", "systemCounters", "accountToActors"} {
		if body[name] != true {
			t.Fatalf("expected top-level %s=true in UI-compatible export body, got %#v", name, body)
		}
	}
}

func TestBuildGraphImportBodyGuardrails(t *testing.T) {
	base := map[string]any{"confirmImport": true, "refReplacePrefix": "clone_20260724_"}
	if _, err := buildGraphImportBody("file.graph", "", map[string]any{"refReplacePrefix": "clone_20260724_"}); err == nil {
		t.Fatal("expected import to require confirmation")
	}
	if _, err := buildGraphImportBody("file.graph", "", map[string]any{"confirmImport": true}); err == nil {
		t.Fatal("expected replace import to require prefix")
	}
	if _, err := buildGraphImportBody("file.graph", "", map[string]any{"confirmImport": true, "refStrategy": "reuse"}); err == nil {
		t.Fatal("expected reuse import to require allowReuseImport")
	}
	body, err := buildGraphImportBody("file.graph", "", base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ops := requireOpsMap(t, body)
	if ops["actorRefStrategy"] != "replace" || ops["actorRefReplacePrefix"] != "clone_20260724_" {
		t.Fatalf("unexpected ops: %#v", ops)
	}
}

func TestBuildGraphImportBodyStrategiesWithoutPrefix(t *testing.T) {
	body, err := buildGraphImportBody("file.graph", "", map[string]any{
		"confirmImport": true,
		"refStrategy":   "error",
	})
	if err != nil {
		t.Fatalf("refStrategy=error should not require a prefix: %v", err)
	}
	ops := requireOpsMap(t, body)
	if ops["actorRefStrategy"] != "error" {
		t.Fatalf("expected error strategy, got %#v", ops)
	}
	if _, ok := ops["actorRefReplacePrefix"]; ok {
		t.Fatalf("error strategy should not send replace prefixes: %#v", ops)
	}

	body, err = buildGraphImportBody("file.graph", "", map[string]any{
		"confirmImport":    true,
		"refStrategy":      "reuse",
		"allowReuseImport": true,
	})
	if err != nil {
		t.Fatalf("refStrategy=reuse with explicit allow should not require a prefix: %v", err)
	}
	ops = requireOpsMap(t, body)
	if ops["actorRefStrategy"] != "reuse" {
		t.Fatalf("expected reuse strategy, got %#v", ops)
	}
	if _, ok := ops["actorRefReplacePrefix"]; ok {
		t.Fatalf("reuse strategy should not send replace prefixes: %#v", ops)
	}

	for _, args := range []map[string]any{
		{"confirmImport": true, "refStrategy": "error", "refReplacePrefix": "clone_20260724_"},
		{"confirmImport": true, "refStrategy": "reuse", "allowReuseImport": true, "refReplacePrefix": "clone_20260724_"},
	} {
		if _, err := buildGraphImportBody("file.graph", "", args); err == nil || !strings.Contains(err.Error(), "only valid with refStrategy=replace") {
			t.Fatalf("expected non-replace prefix rejection, got %v for %#v", err, args)
		}
	}
}

func requireOpsMap(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	ops, ok := body["ops"].(map[string]any)
	if !ok {
		t.Fatalf("expected ops map, got %#v", body["ops"])
	}
	return ops
}

func requireAnySlice(t *testing.T, value any) []any {
	t.Helper()
	items := requireAnySliceValue(t, value)
	if len(items) == 0 {
		t.Fatalf("expected non-empty []any, got %#v", value)
	}
	return items
}

func requireAnySliceValue(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %#v", value)
	}
	return items
}

func TestGraphTaskListResponseAcceptsLiveArrayShape(t *testing.T) {
	var resp graphTaskListResponse
	if err := json.Unmarshal([]byte(`{"data":[{"id":1,"status":"completed","name":"export"}]}`), &resp); err != nil {
		t.Fatalf("array data shape rejected: %v", err)
	}
	if len(resp.Data.Tasks) != 1 || resp.Data.Tasks[0].ID != 1 {
		t.Fatalf("unexpected array data parse: %#v", resp.Data.Tasks)
	}
	if err := json.Unmarshal([]byte(`{"data":{"tasks":[{"id":2,"status":"completed","name":"export"}]}}`), &resp); err != nil {
		t.Fatalf("object data shape rejected: %v", err)
	}
	if len(resp.Data.Tasks) != 1 || resp.Data.Tasks[0].ID != 2 {
		t.Fatalf("unexpected object data parse: %#v", resp.Data.Tasks)
	}
}

func TestGetGraphImportExportTaskByIDUsesDirectEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123}}"}}`)
		case r.URL.Path == "/papi/1.0/tasks/list/source/export":
			t.Fatalf("taskId lookup must use direct task endpoint before list fallback")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"accId":  "source",
		"name":   "export",
		"taskId": 101,
	}
	res, err := handleGetGraphImportExportTask(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"id":101`) || !strings.Contains(text, `"fileName":"bucket/export.graph"`) {
		t.Fatalf("unexpected task output: %s", text)
	}
}

func TestApplyGraphSourceURLSelection(t *testing.T) {
	const (
		sourceAccID = "11111111-2222-4333-8444-555555555555"
		graphID     = "33333333-3333-4333-8333-333333333333"
		layerID     = "22222222-2222-4222-8222-222222222222"
	)
	ctx := apiclient.WithWorkspaceID(context.Background(), sourceAccID)
	args := map[string]any{
		"sourceUrl": "https://mw.simulator.company/actors_graph/11111111/graph/" + graphID + "/layers/" + layerID,
	}
	info, err := applyGraphSourceURL(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.SourceKind != "layer" || info.GraphID != graphID || info.LayerID != layerID {
		t.Fatalf("unexpected source info: %#v", info)
	}
	if got := args["sourceAccId"]; got != sourceAccID {
		t.Fatalf("short workspace segment should resolve to current full workspace, got %#v", got)
	}
	if info.WorkspaceID != sourceAccID {
		t.Fatalf("source info should keep the resolved full workspace id, got %#v", info.WorkspaceID)
	}
	if got := requireAnySlice(t, args["actorIds"])[0]; got != layerID {
		t.Fatalf("auto mode should select layer from layer URL, got %#v", args["actorIds"])
	}

	args = map[string]any{
		"sourceUrl":  "https://mw.simulator.company/actors_graph/" + sourceAccID + "/graph/" + graphID + "/layers/" + layerID,
		"sourceKind": "graph",
	}
	info, err = applyGraphSourceURL(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.SourceKind != "graph" {
		t.Fatalf("sourceKind=graph was not preserved: %#v", info)
	}
	if got := requireAnySlice(t, args["actorIds"])[0]; got != graphID {
		t.Fatalf("graph mode should select graph root, got %#v", args["actorIds"])
	}
}

func TestReadLocalGraphArchiveRejectsUnsafeFiles(t *testing.T) {
	if _, _, err := readLocalGraphArchive("relative.graph"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
	tmp := t.TempDir()
	txt := tmp + "/secret.txt"
	if err := osWriteFile(txt, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLocalGraphArchive(txt); err == nil || !strings.Contains(err.Error(), ".graph") {
		t.Fatalf("expected .graph extension error, got %v", err)
	}
}

func TestValidateGraphArchiveBytes(t *testing.T) {
	if err := validateGraphArchiveBytes([]byte("not zip")); err == nil {
		t.Fatal("expected invalid zip error")
	}
	if err := validateGraphArchiveBytes(makeGraphArchive(t, false, true, false)); err == nil || !strings.Contains(err.Error(), "GraphManifest") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
	if err := validateGraphArchiveBytes(makeGraphArchive(t, true, false, false)); err == nil || !strings.Contains(err.Error(), "actor or form") {
		t.Fatalf("expected missing actor/form error, got %v", err)
	}
	if err := validateGraphArchiveBytes(makeGraphArchive(t, true, true, true)); err != nil {
		t.Fatalf("valid archive rejected: %v", err)
	}
}

func TestCloneGraphLayerHappyPath(t *testing.T) { //nolint:gocyclo // One httptest flow documents the export/download/upload/import sequence.
	const (
		sourceLayerID = "11111111-1111-4111-8111-111111111111"
		targetLayerID = "22222222-2222-4222-8222-222222222222"
	)
	graphBytes := makeGraphArchiveWithActors(t, map[string]string{
		sourceLayerID: "id: " + sourceLayerID + "\nformId: 200\nref: layer_ref\ntitle: Layer\n",
	})
	var exportBody, importBody map[string]any
	var uploaded bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/source/export":
			if err := json.NewDecoder(r.Body).Decode(&exportBody); err != nil {
				t.Fatalf("decode export body: %v", err)
			}
			writeJSON(w, `{"data":{"id":101,"status":"created","name":"export","accId":"source"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/source/export":
			writeJSON(w, `{"data":[{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/export.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/target":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Fatalf("expected multipart upload, got %q", r.Header.Get("Content-Type"))
			}
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			uploaded = true
			writeJSON(w, `{"data":{"id":201,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/target/import":
			if err := json.NewDecoder(r.Body).Decode(&importBody); err != nil {
				t.Fatalf("decode import body: %v", err)
			}
			writeJSON(w, `{"data":{"id":301,"status":"created","name":"import","accId":"target"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/target/import/301":
			writeJSON(w, `{"data":{"id":301,"status":"completed","name":"import","accId":"target","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/target/import":
			writeJSON(w, `{"data":[{"id":301,"status":"completed","name":"import","accId":"target","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/actors/ref/200/clone_20260724_layer_ref":
			writeJSON(w, `{"data":{"id":"`+targetLayerID+`","formId":200,"ref":"clone_20260724_layer_ref","title":"Layer"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"layerId":          sourceLayerID,
		"sourceAccId":      "source",
		"targetAccId":      "target",
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"users":            true,
		"waitSec":          1,
	}
	res, err := handleCloneGraphLayer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if !uploaded {
		t.Fatal("expected graph archive upload")
	}
	if got := requireAnySlice(t, exportBody["actors"])[0]; got != sourceLayerID {
		t.Fatalf("export actor mismatch: %#v", exportBody)
	}
	exportOps := requireOpsMap(t, exportBody)
	if exportOps["users"] != true {
		t.Fatalf("export users flag was not preserved: %#v", exportOps)
	}
	if exportOps["balances"] != false || exportOps["connectorsToAccounts"] != false || exportOps["accountToActors"] != false {
		t.Fatalf("export relation flags must be mirrored inside ops for live backend schema: %#v", exportOps)
	}
	ops := requireOpsMap(t, importBody)
	if ops["actorRefStrategy"] != "replace" || ops["actorRefReplacePrefix"] != "clone_20260724_" {
		t.Fatalf("unexpected import ops: %#v", ops)
	}
	if users := requireAnySliceValue(t, importBody["users"]); len(users) != 0 {
		t.Fatalf("export users flag leaked into import mappings: %#v", users)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"exportTask"`) || !strings.Contains(text, `"importTask"`) {
		t.Fatalf("unexpected result: %s", text)
	}
	if !strings.Contains(text, `"countersMatch":true`) {
		t.Fatalf("expected matching counters in result, got %s", text)
	}
}

func TestCloneGraphLayerReuseStrategyAllowsNoPrefix(t *testing.T) {
	graphBytes := makeGraphArchive(t, true, true, true)
	var importBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/source/export":
			writeJSON(w, `{"data":{"id":101,"status":"created","name":"export","accId":"source"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/export.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/target":
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			writeJSON(w, `{"data":{"id":201,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/target/import":
			if err := json.NewDecoder(r.Body).Decode(&importBody); err != nil {
				t.Fatalf("decode import body: %v", err)
			}
			writeJSON(w, `{"data":{"id":301,"status":"created","name":"import","accId":"target"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/target/import/301":
			writeJSON(w, `{"data":{"id":301,"status":"completed","name":"import","accId":"target","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"layerId":          "11111111-1111-4111-8111-111111111111",
		"sourceAccId":      "source",
		"targetAccId":      "target",
		"refStrategy":      "reuse",
		"allowReuseImport": true,
		"confirmClone":     true,
		"waitSec":          1,
	}
	res, err := handleCloneGraphLayer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	ops := requireOpsMap(t, importBody)
	if ops["actorRefStrategy"] != "reuse" || ops["formRefStrategy"] != "reuse" {
		t.Fatalf("unexpected import ops: %#v", ops)
	}
	if _, ok := ops["actorRefReplacePrefix"]; ok {
		t.Fatalf("reuse strategy must not send replace prefixes: %#v", ops)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"importRefStrategy":"reuse"`) {
		t.Fatalf("expected reuse strategy in result, got %s", text)
	}
}

func TestCloneGraphObjectsDefaultsTargetToCurrentWorkspace(t *testing.T) {
	graphBytes := makeGraphArchive(t, true, true, true)
	var exportBody, importBody map[string]any
	uploadedToCurrent := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/current/export":
			if err := json.NewDecoder(r.Body).Decode(&exportBody); err != nil {
				t.Fatalf("decode export body: %v", err)
			}
			writeJSON(w, `{"data":{"id":111,"status":"created","name":"export","accId":"current"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/current/export/111":
			writeJSON(w, `{"data":{"id":111,"status":"completed","name":"export","accId":"current","details":"{\"file\":{\"fileName\":\"bucket/graph.graph\",\"title\":\"graph.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/current/export":
			writeJSON(w, `{"data":[{"id":111,"status":"completed","name":"export","accId":"current","details":"{\"file\":{\"fileName\":\"bucket/graph.graph\",\"title\":\"graph.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/graph.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/current":
			uploadedToCurrent = true
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			writeJSON(w, `{"data":{"id":222,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/current/import":
			if err := json.NewDecoder(r.Body).Decode(&importBody); err != nil {
				t.Fatalf("decode import body: %v", err)
			}
			writeJSON(w, `{"data":{"id":333,"status":"created","name":"import","accId":"current"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/current/import/333":
			writeJSON(w, `{"data":{"id":333,"status":"completed","name":"import","accId":"current","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/current/import":
			writeJSON(w, `{"data":[{"id":333,"status":"completed","name":"import","accId":"current","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"actorIds":         []any{"22222222-2222-4222-8222-222222222222"},
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	ctx := apiclient.WithWorkspaceID(context.Background(), "current")
	res, err := handleCloneGraphObjects(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if !uploadedToCurrent {
		t.Fatal("expected upload into current workspace")
	}
	if got := requireAnySlice(t, exportBody["actors"])[0]; got != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("export actor mismatch: %#v", exportBody)
	}
	ops := requireOpsMap(t, importBody)
	if ops["actorRefReplacePrefix"] != "clone_20260724_" {
		t.Fatalf("unexpected import ops: %#v", ops)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"sourceAccId":"current"`) || !strings.Contains(text, `"targetAccId":"current"`) {
		t.Fatalf("expected source/target to default to current workspace, got %s", text)
	}
	if !strings.Contains(text, `"countersMatch":true`) {
		t.Fatalf("expected matching counters in result, got %s", text)
	}
	if !strings.Contains(text, "target workspace is the same as the source workspace") {
		t.Fatalf("expected same-workspace warning, got %s", text)
	}
}

func TestCloneGraphObjectsFromURLReturnsTargetLayerURL(t *testing.T) { //nolint:gocyclo // One httptest flow documents URL clone plus target resolution.
	const (
		sourceAccID = "11111111-2222-4333-8444-555555555555"
		graphID     = "33333333-3333-4333-8333-333333333333"
		layerID     = "22222222-2222-4222-8222-222222222222"
		targetAccID = "target"
		newGraphID  = "44444444-4444-4444-8444-444444444444"
		newLayerID  = "55555555-5555-4555-8555-555555555555"
	)
	graphBytes := makeGraphArchiveWithActors(t, map[string]string{
		graphID: "id: " + graphID + "\nformId: 100\nref: graph_ref\ntitle: Graph\n",
		layerID: "id: " + layerID + "\nformId: 200\nref: layer_ref\ntitle: Layer\n",
	})
	var exportBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/"+sourceAccID+"/export":
			if err := json.NewDecoder(r.Body).Decode(&exportBody); err != nil {
				t.Fatalf("decode export body: %v", err)
			}
			writeJSON(w, `{"data":{"id":111,"status":"created","name":"export","accId":"`+sourceAccID+`"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/"+sourceAccID+"/export/111":
			writeJSON(w, `{"data":{"id":111,"status":"completed","name":"export","accId":"`+sourceAccID+`","details":"{\"file\":{\"fileName\":\"bucket/source.graph\",\"title\":\"source.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/"+sourceAccID+"/export":
			writeJSON(w, `{"data":[{"id":111,"status":"completed","name":"export","accId":"`+sourceAccID+`","details":"{\"file\":{\"fileName\":\"bucket/source.graph\",\"title\":\"source.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/source.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/"+targetAccID:
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			writeJSON(w, `{"data":{"id":222,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/"+targetAccID+"/import":
			writeJSON(w, `{"data":{"id":333,"status":"created","name":"import","accId":"`+targetAccID+`"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/"+targetAccID+"/import/333":
			writeJSON(w, `{"data":{"id":333,"status":"completed","name":"import","accId":"`+targetAccID+`","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/"+targetAccID+"/import":
			writeJSON(w, `{"data":[{"id":333,"status":"completed","name":"import","accId":"`+targetAccID+`","details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":2,\"forms\":1,\"edges\":1}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/actors/ref/100/clone_20260724_graph_ref":
			writeJSON(w, `{"data":{"id":"`+newGraphID+`","formId":100,"ref":"clone_20260724_graph_ref","title":"Graph"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/actors/ref/200/clone_20260724_layer_ref":
			writeJSON(w, `{"data":{"id":"`+newLayerID+`","formId":200,"ref":"clone_20260724_layer_ref","title":"Layer"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sourceUrl":        "https://mw.simulator.company/actors_graph/11111111/graph/" + graphID + "/layers/" + layerID,
		"targetAccId":      targetAccID,
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	ctx := apiclient.WithWorkspaceID(context.Background(), sourceAccID)
	ctx = apiclient.WithBaseURL(ctx, srv.URL+"/papi/1.0")
	res, err := handleCloneGraphObjects(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if got := requireAnySlice(t, exportBody["actors"])[0]; got != layerID {
		t.Fatalf("expected sourceUrl auto mode to clone layer, got export body %#v", exportBody)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"sourceGraphId":"`+graphID+`"`) || !strings.Contains(text, `"sourceLayerId":"`+layerID+`"`) {
		t.Fatalf("expected source URL metadata, got %s", text)
	}
	if !strings.Contains(text, `"graphId":"`+newGraphID+`"`) || !strings.Contains(text, `"layerId":"`+newLayerID+`"`) {
		t.Fatalf("expected resolved target graph/layer ids, got %s", text)
	}
	wantURL := srv.URL + "/actors_graph/target/graph/" + newGraphID + "/layers/" + newLayerID
	if !strings.Contains(text, `"url":"`+wantURL+`"`) {
		t.Fatalf("expected target URL %s, got %s", wantURL, text)
	}
}

func TestCloneGraphObjectsFromURLResolvesRefLessLayerByRecentTitle(t *testing.T) {
	const (
		sourceAccID = "11111111-2222-4333-8444-555555555555"
		graphID     = "33333333-3333-4333-8333-333333333333"
		layerID     = "22222222-2222-4222-8222-222222222222"
		targetAccID = "target"
		newLayerID  = "55555555-5555-4555-8555-555555555555"
		layerTitle  = "Layer Without Ref Long Fixture"
	)
	graphBytes := makeGraphArchiveWithActors(t, map[string]string{
		layerID: "id: " + layerID + "\ntitle: " + layerTitle + "\n",
	})
	searchCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/"+sourceAccID+"/export":
			writeJSON(w, `{"data":{"id":111,"status":"created","name":"export","accId":"`+sourceAccID+`"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/"+sourceAccID+"/export/111":
			writeJSON(w, `{"data":{"id":111,"status":"completed","name":"export","accId":"`+sourceAccID+`","details":"{\"file\":{\"fileName\":\"bucket/source.graph\",\"title\":\"source.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/source.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/"+targetAccID:
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			writeJSON(w, `{"data":{"id":222,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/"+targetAccID+"/import":
			writeJSON(w, `{"data":{"id":333,"status":"created","name":"import","accId":"`+targetAccID+`"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/"+targetAccID+"/import/333":
			writeJSON(w, `{"data":{"id":333,"status":"completed","name":"import","accId":"`+targetAccID+`","createdAt":2000,"timeStart":2001,"details":"{\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/actors/"+layerID:
			writeJSON(w, `{"data":{"id":"`+layerID+`","title":"`+layerTitle+`","formId":26741,"createdAt":1000}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/papi/1.0/actors_filters/search/"+targetAccID+"/"):
			if r.URL.Query().Get("formId") != "26741" {
				t.Fatalf("expected formId search narrowing, got query %s", r.URL.RawQuery)
			}
			searchCalls++
			decodedPath, _ := url.PathUnescape(r.URL.EscapedPath())
			if strings.Contains(decodedPath, layerTitle) {
				writeJSON(w, `{"data":{"list":[],"total":0}}`)
				return
			}
			writeJSON(w, `{"data":{"list":[{"id":"`+layerID+`","title":"`+layerTitle+`","formId":26741,"createdAt":1000},{"id":"`+newLayerID+`","title":"`+layerTitle+`","formId":26741,"createdAt":2001}],"total":2}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sourceUrl":        "https://mw.simulator.company/actors_graph/11111111/graph/" + graphID + "/layers/" + layerID,
		"targetAccId":      targetAccID,
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	ctx := apiclient.WithWorkspaceID(context.Background(), sourceAccID)
	ctx = apiclient.WithBaseURL(ctx, srv.URL+"/papi/1.0")
	res, err := handleCloneGraphObjects(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if searchCalls != 2 {
		t.Fatalf("expected exact search plus shortened fallback search, got %d calls", searchCalls)
	}
	text := textResult(t, res)
	if !strings.Contains(text, `"layerId":"`+newLayerID+`"`) {
		t.Fatalf("expected target layer id resolved by fallback, got %s", text)
	}
	wantURL := srv.URL + "/actors_graph/target/graph/" + newLayerID + "/layers"
	if !strings.Contains(text, `"url":"`+wantURL+`"`) {
		t.Fatalf("expected target URL %s, got %s", wantURL, text)
	}
	if !strings.Contains(text, "resolved by title/formId fallback") {
		t.Fatalf("expected fallback warning, got %s", text)
	}
	if strings.Contains(text, "source actor metadata was not present in the export archive") {
		t.Fatalf("layer clone must not warn about optional graph root metadata, got %s", text)
	}
}

func TestImportGraphArchiveRequiresConfirmBeforeLocalUpload(t *testing.T) {
	tmp := t.TempDir()
	archivePath := tmp + "/sample.graph"
	if err := os.WriteFile(archivePath, makeGraphArchive(t, true, true, true), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("import without confirm must not call backend, got %s %s", r.Method, r.URL.String())
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"accId":            "target",
		"localPath":        archivePath,
		"refReplacePrefix": "clone_20260724_",
	}
	res, err := handleImportGraphArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected confirm error, got %#v", res)
	}
	if !strings.Contains(textResult(t, res), "confirmImport") {
		t.Fatalf("expected confirmImport error, got %s", textResult(t, res))
	}
}

func TestCloneGraphLayerReportsFailedExport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/source/export":
			writeJSON(w, `{"data":{"id":101,"status":"created","name":"export","accId":"source"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"failed","name":"export","accId":"source","details":"{\"error\":true,\"errMsg\":\"Failed to get Actor\"}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/source/export":
			writeJSON(w, `{"data":[{"id":101,"status":"failed","name":"export","accId":"source","details":"{\"error\":true,\"errMsg\":\"Failed to get Actor\"}"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"layerId":          "11111111-1111-4111-8111-111111111111",
		"sourceAccId":      "source",
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	res, err := handleCloneGraphLayer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %#v", res)
	}
	if !strings.Contains(textResult(t, res), "Failed to get Actor") {
		t.Fatalf("expected export error details, got %s", textResult(t, res))
	}
}

func TestCloneGraphLayerReportsAccessDeniedExportHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/source/export":
			writeJSON(w, `{"data":{"id":101,"status":"created","name":"export","accId":"source"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"failed","name":"export","accId":"source","details":"{\"error\":true,\"errMsg\":\"Access denied to actor on layer\"}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/source/export":
			writeJSON(w, `{"data":[{"id":101,"status":"failed","name":"export","accId":"source","details":"{\"error\":true,\"errMsg\":\"Access denied to actor on layer\"}"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"layerId":          "11111111-1111-4111-8111-111111111111",
		"sourceAccId":      "source",
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	res, err := handleCloneGraphLayer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %#v", res)
	}
	text := textResult(t, res)
	if !strings.Contains(text, "Access denied to actor on layer") {
		t.Fatalf("expected export error details, got %s", text)
	}
	if !strings.Contains(text, "Export requires access to every object") {
		t.Fatalf("expected access failure hint, got %s", text)
	}
}

func TestCloneGraphObjectsReportsFailedImport(t *testing.T) {
	graphBytes := makeGraphArchive(t, true, true, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/source/export":
			writeJSON(w, `{"data":{"id":101,"status":"created","name":"export","accId":"source"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/source/export/101":
			writeJSON(w, `{"data":{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/source/export":
			writeJSON(w, `{"data":[{"id":101,"status":"completed","name":"export","accId":"source","details":"{\"file\":{\"fileName\":\"bucket/export.graph\",\"title\":\"export.graph\",\"size\":123},\"manifest\":{\"stats\":{\"counters\":{\"actors\":1,\"forms\":1,\"edges\":0}}}}"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/download/bucket/export.graph":
			_, _ = w.Write(graphBytes)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/upload/target":
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatalf("read upload: %v", err)
			}
			writeJSON(w, `{"data":{"id":201,"fileName":"bucket/import.graph","title":"import.graph","size":123,"type":"application/octet-stream"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/papi/1.0/tasks/target/import":
			writeJSON(w, `{"data":{"id":301,"status":"created","name":"import","accId":"target"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/target/import/301":
			writeJSON(w, `{"data":{"id":301,"status":"failed","name":"import","accId":"target","details":"{\"error\":true,\"errMsg\":\"Not unique cell id\"}"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/papi/1.0/tasks/list/target/import":
			writeJSON(w, `{"data":[{"id":301,"status":"failed","name":"import","accId":"target","details":"{\"error\":true,\"errMsg\":\"Not unique cell id\"}"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	ecore.Configure(srv.URL+"/papi/1.0", false)
	ecore.Cfg.Authorization = "Simulator test-token"
	t.Setenv("ACCESS_TOKEN", "test-token")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"actorIds":         []any{"22222222-2222-4222-8222-222222222222"},
		"sourceAccId":      "source",
		"targetAccId":      "target",
		"refReplacePrefix": "clone_20260724_",
		"confirmClone":     true,
		"waitSec":          1,
	}
	res, err := handleCloneGraphObjects(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %#v", res)
	}
	if !strings.Contains(textResult(t, res), "Not unique cell id") {
		t.Fatalf("expected import error details, got %s", textResult(t, res))
	}
}

func makeGraphArchive(t *testing.T, manifest, actor, form bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if manifest {
		add("GraphManifest.yaml", "version: 1.0.0\nstats:\n  counters:\n    actors: 1\n")
	}
	if actor {
		add("topology/actors/11111111-1111-4111-8111-111111111111.yaml", "id: 11111111-1111-4111-8111-111111111111\n")
	}
	if form {
		add("forms/123.yaml", "id: 123\n")
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeGraphArchiveWithActors(t *testing.T, actors map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("GraphManifest.yaml", "version: 1.0.0\nstats:\n  counters:\n    actors: 2\n")
	add("forms/100.yaml", "id: 100\n")
	for id, body := range actors {
		add("topology/actors/"+id+".yaml", body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func textResult(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty result")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text result, got %T", res.Content[0])
	}
	return tc.Text
}

func osWriteFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0o600)
}
