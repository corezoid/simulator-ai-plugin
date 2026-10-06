package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// echoServer has one tool that reports the per-request values tool handlers
// see, so tests can assert what the transport put on ctx.
func echoServer() *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(false))
	s.AddTool(mcp.NewTool("echo-ctx"), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(fmt.Sprintf("auth=%s|ws=%s|actor=%s|base=%s",
			apiclient.AuthorizationFromContext(ctx),
			apiclient.WorkspaceIDFromContext(ctx),
			apiclient.ActorIDFromContext(ctx),
			apiclient.BaseURLFromContext(ctx))), nil
	})
	return s
}

func post(t *testing.T, url string, headers map[string]string, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, jsonPayload(string(b))
}

// jsonPayload returns a plain JSON body as is, or the data line of a single
// SSE event.
func jsonPayload(raw string) string {
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return raw
	}
	for _, line := range strings.Split(raw, "\n") {
		if rest, ok := strings.CutPrefix(line, "data: "); ok {
			return rest
		}
	}
	return raw
}

const (
	initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	callBody = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo-ctx","arguments":{}}}`
)

func newTestServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(NewHandler(echoServer(), cfg))
	t.Cleanup(ts.Close)
	return ts
}

var prodLike = Config{
	ResourceURL:   "https://mcp.simulator.company",
	AuthServerURL: "https://account.corezoid.com",
	FallbackURL:   "https://mw.simulator.company",
}

func TestAnonymousRequestsGetOAuthChallenge(t *testing.T) {
	ts := newTestServer(t, prodLike)
	for _, path := range []string{"/mcp", "/mcp/workspaces/ws-1", "/mcp/workspaces/ws-1/actors/a-1"} {
		resp, _ := post(t, ts.URL+path, nil, initBody)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, resp.StatusCode)
		}
		want := `Bearer resource_metadata="https://mcp.simulator.company/.well-known/oauth-protected-resource"`
		if got := resp.Header.Get("WWW-Authenticate"); got != want {
			t.Errorf("%s: WWW-Authenticate = %q, want %q", path, got, want)
		}
	}
}

// Without discovery configured the hosted server still never serves an
// anonymous request.
func TestAnonymousRefusedWithoutDiscovery(t *testing.T) {
	ts := newTestServer(t, Config{})
	resp, _ := post(t, ts.URL+"/mcp", map[string]string{"Authorization": "Bearer "}, initBody)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") != "" {
		t.Error("no discovery configured, but a challenge was sent")
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	ts := newTestServer(t, prodLike)
	resp, err := http.Get(ts.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.Resource != "https://mcp.simulator.company" || len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != "https://account.corezoid.com" {
		t.Errorf("metadata = %+v", meta)
	}
}

// Two bare POSTs with no Mcp-Session-Id: initialize, then a tool call that
// reports what it saw. Covers stateless transport, token normalisation, the
// path-derived scope and the pinned base URL — including that a client header
// cannot move the base URL.
func TestToolCallsSeeCallerScope(t *testing.T) {
	ts := newTestServer(t, prodLike)
	cases := []struct {
		path, auth, want string
	}{
		{"/mcp", "Bearer tok", "auth=Simulator tok|ws=|actor=|base=https://mw.simulator.company/papi/1.0"},
		{"/mcp", "tok", "auth=Simulator tok|"},
		{"/mcp/workspaces/ws-1", "Simulator tok", "auth=Simulator tok|ws=ws-1|actor=|base=https://mw.simulator.company/papi/1.0"},
		{"/mcp/workspaces/ws-1/actors/a-9", "Bearer tok", "ws=ws-1|actor=a-9|"},
	}
	for _, c := range cases {
		h := map[string]string{"Authorization": c.auth, "X-Simulator-Base-Url": "https://evil.example", "X-Simulator-Workspace-Id": "spoofed"}
		if resp, _ := post(t, ts.URL+c.path, h, initBody); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s initialize: status %d", c.path, resp.StatusCode)
		}
		resp, body := post(t, ts.URL+c.path, h, callBody)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s tools/call without session: status %d body %.200s", c.path, resp.StatusCode, body)
		}
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: tool saw %.300s, want substring %q", c.path, body, c.want)
		}
		if strings.Contains(body, "evil.example") || strings.Contains(body, "spoofed") {
			t.Errorf("%s: a client header changed the request scope: %.300s", c.path, body)
		}
	}
}

// A workspace whose resolved origin is outside the allowlist is served
// against the fallback, never the resolved host.
func TestScopedRouteFallsBackOnRefusedOrigin(t *testing.T) {
	acct := &fakeAccount{
		workspaces: map[string]string{"Simulator tok|ws-x": "c"},
		clients:    map[string][]accountClient{"Simulator tok": {{ClientID: "c", RedirectURI: "https://evil.example/cb"}}},
	}
	cfg := prodLike
	cfg.AccountURL = acct.server(t).URL
	ts := newTestServer(t, cfg)

	h := map[string]string{"Authorization": "Bearer tok"}
	post(t, ts.URL+"/mcp/workspaces/ws-x", h, initBody)
	_, body := post(t, ts.URL+"/mcp/workspaces/ws-x", h, callBody)
	if !strings.Contains(body, "base=https://mw.simulator.company/papi/1.0") || strings.Contains(body, "evil.example") {
		t.Errorf("tool saw %.300s, want the fallback base URL", body)
	}
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t, prodLike)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz: %d", resp.StatusCode)
	}
}

func TestNormalizeAuthorization(t *testing.T) {
	for in, want := range map[string]string{
		"":                "",
		"   ":             "",
		"Bearer ":         "",
		"Bearer":          "",
		"BEARER":          "",
		"Simulator":       "",
		"SIMULATOR abc":   "Simulator abc",
		"Bearer abc":      "Simulator abc",
		"bearer abc":      "Simulator abc",
		"Simulator abc":   "Simulator abc",
		"abc":             "Simulator abc",
		"  Bearer  abc  ": "Simulator abc",
	} {
		if got := normalizeAuthorization(in); got != want {
			t.Errorf("normalizeAuthorization(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeSimulatorAPIURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                                      "",
		"https://mw.simulator.company":          "https://mw.simulator.company/papi/1.0",
		"https://mw.simulator.company/":         "https://mw.simulator.company/papi/1.0",
		"mw.simulator.company":                  "https://mw.simulator.company/papi/1.0",
		"https://mw.simulator.company/papi/1.0": "https://mw.simulator.company/papi/1.0",
	} {
		if got := normalizeSimulatorAPIURL(in); got != want {
			t.Errorf("normalizeSimulatorAPIURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenAIAppsChallenge(t *testing.T) {
	ts := newTestServer(t, prodLike)
	resp, err := http.Get(ts.URL + "/.well-known/openai-apps-challenge")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unset challenge: status %d, want 404", resp.StatusCode)
	}

	cfg := prodLike
	cfg.OpenAIAppsChallenge = " tok-123 "
	ts2 := newTestServer(t, cfg)
	resp, err = http.Get(ts2.URL + "/.well-known/openai-apps-challenge")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "tok-123" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("challenge: %d %q %q, want 200 text/plain exact token", resp.StatusCode, body, resp.Header.Get("Content-Type"))
	}
}

func TestHumanizeToolName(t *testing.T) {
	for in, want := range map[string]string{
		"getWorkspaces":          "Get workspaces",
		"uploadActorPictureBulk": "Upload actor picture bulk",
		"set-workspace":          "Set workspace",
		"getURLForm":             "Get url form",
		"x":                      "X",
	} {
		if got := humanizeToolName(in); got != want {
			t.Errorf("humanizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEveryHostedToolHasATitle(t *testing.T) {
	s := echoServer()
	s.AddTool(mcp.NewTool("keepMine", mcp.WithTitleAnnotation("Custom")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) { return nil, nil })
	NewHandler(s, prodLike)
	for name, st := range s.ListTools() {
		if st.Tool.Title == "" && st.Tool.Annotations.Title == "" {
			t.Errorf("%s has no title", name)
		}
	}
	if got := s.ListTools()["keepMine"].Tool.Annotations.Title; got != "Custom" {
		t.Errorf("existing title overwritten: %q", got)
	}
}
