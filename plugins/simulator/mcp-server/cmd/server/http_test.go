package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestHTTPContextFunc(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, mcpEndpointPath, nil)
	r.Header.Set("Authorization", "Simulator test-jwt")
	r.Header.Set("X-Simulator-Workspace-Id", "ws-123")
	r.Header.Set("X-Simulator-Actor-Id", "actor-456")

	ctx := httpContextFunc(context.Background(), r)

	if got := apiclient.AuthorizationFromContext(ctx); got != "Simulator test-jwt" {
		t.Errorf("Authorization: got %q, want it forwarded verbatim", got)
	}
	if got := apiclient.WorkspaceIDFromContext(ctx); got != "ws-123" {
		t.Errorf("workspace id: got %q, want %q", got, "ws-123")
	}
	if got := apiclient.ActorIDFromContext(ctx); got != "actor-456" {
		t.Errorf("actor id: got %q, want %q", got, "actor-456")
	}
}

func TestHTTPContextFuncMissingHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, mcpEndpointPath, nil)
	ctx := httpContextFunc(context.Background(), r)

	if got := apiclient.AuthorizationFromContext(ctx); got != "" {
		t.Errorf("Authorization: got %q, want empty (auth failure must surface in the API client, not here)", got)
	}
	if got := apiclient.WorkspaceIDFromContext(ctx); got != "" {
		t.Errorf("workspace id: got %q, want empty", got)
	}
}

func TestHealthz(t *testing.T) {
	h := newHTTPHandler(server.NewMCPServer("test", "0.0.0"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, healthzPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: got %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("healthz body: got %q, want %q", body, "ok")
	}
}

// postMCP sends one JSON-RPC request to the streamable endpoint and returns the
// decoded response. Headers are applied to the HTTP request, mimicking a remote
// client (e.g. an OpenAI connector) attaching credentials per request.
func postMCP(t *testing.T, ts *httptest.Server, headers map[string]string, body string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+mcpEndpointPath, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s: status %d", mcpEndpointPath, resp.StatusCode)
	}

	// Stateless streamable responses arrive either as plain JSON or as a single
	// SSE event; accept both shapes.
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	if i := strings.Index(raw, "data: "); i >= 0 {
		raw = raw[i+len("data: "):]
		if j := strings.Index(raw, "\n"); j >= 0 {
			raw = raw[:j]
		}
	}
	var out map[string]any
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
	}
	return out
}

// TestMCPOverHTTPEndToEnd drives the real handler stack: initialize, then a
// tools/call whose handler echoes the per-request ctx values — proving the
// HTTP headers reach tool handlers through WithHTTPContextFunc, and that two
// bare requests work without any session state (stateless mode).
func TestMCPOverHTTPEndToEnd(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(false))
	s.AddTool(
		mcp.NewTool("echo-ctx", mcp.WithDescription("echo per-request ctx values")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(fmt.Sprintf("auth=%s ws=%s",
				apiclient.AuthorizationFromContext(ctx),
				apiclient.WorkspaceIDFromContext(ctx))), nil
		},
	)
	ts := httptest.NewServer(newHTTPHandler(s))
	defer ts.Close()

	headers := map[string]string{
		"Authorization":            "Simulator jwt-abc",
		"X-Simulator-Workspace-Id": "ws-777",
	}

	initResp := postMCP(t, ts, headers,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if initResp["error"] != nil {
		t.Fatalf("initialize error: %v", initResp["error"])
	}

	// No Mcp-Session-Id is carried over — stateless mode must accept this.
	callResp := postMCP(t, ts, headers,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo-ctx","arguments":{}}}`)
	if callResp["error"] != nil {
		t.Fatalf("tools/call error: %v", callResp["error"])
	}
	b, _ := json.Marshal(callResp)
	if want := "auth=Simulator jwt-abc ws=ws-777"; !strings.Contains(string(b), want) {
		t.Errorf("tool did not see per-request headers: response %s, want substring %q", b, want)
	}
}
