package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// apiKeyEnv points .env at an empty temp dir and turns API-key mode on, so a
// handler that wrongly falls through to the OAuth path is caught by an
// unexpectedly non-empty directory rather than by opening a browser.
func apiKeyEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	t.Setenv("ACCESS_TOKEN", "")
	t.Setenv(auth.APISecretEnv, "wsk_key")
	return dir
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Errorf("%s should be untouched, found %d entries", dir, len(entries))
	}
}

func TestLoginRefusesInAPIKeyMode(t *testing.T) {
	dir := apiKeyEnv(t)

	// s is nil on purpose: the guard must return before telemetry.AskForEmailOnce.
	res, err := loginHandler(nil, config.Profile{Name: "prod"})(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("loginHandler error = %v", err)
	}
	if res.IsError {
		t.Error("login should return an explanation, not an error — an error invites the model to retry")
	}

	text := resultText(t, res)
	if !strings.Contains(text, auth.APISecretEnv) {
		t.Errorf("result = %q, should name %s", text, auth.APISecretEnv)
	}
	if !strings.Contains(text, "WORKSPACE_ID") {
		t.Errorf("result = %q, should mention the workspace-scoping caveat", text)
	}

	// No token written, no .env created.
	assertDirEmpty(t, dir)
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Errorf(".env exists after login in API-key mode (stat err = %v)", err)
	}
	if os.Getenv("ACCESS_TOKEN") != "" {
		t.Error("login set ACCESS_TOKEN in API-key mode")
	}
}

func TestSetEnvironmentRefusesInAPIKeyMode(t *testing.T) {
	dir := apiKeyEnv(t)
	envPath := filepath.Join(dir, ".env")
	original := auth.APISecretEnv + "=wsk_key\nSIMULATOR_API_BASE_URL=https://mw.simulator.company/papi/1.0\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	s := server.NewMCPServer("test", "0.0.0")
	c := apiclient.New("https://mw.simulator.company/papi/1.0", "ws1", func() (string, error) { return "Bearer k", nil }, false)
	registerSetEnvironment(s, c, config.Profile{Name: "prod"}, false)

	res, err := callTool(t, s, "set-environment", map[string]any{"url": "evil.example.com"})
	if err != nil {
		t.Fatalf("set-environment error = %v", err)
	}
	if !res.IsError {
		t.Fatal("set-environment should refuse in API-key mode")
	}
	if text := resultText(t, res); !strings.Contains(text, auth.APISecretEnv) {
		t.Errorf("result = %q, should explain the API-key constraint", text)
	}

	// The refusal must happen before anything is persisted or cleared.
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(data) != original {
		t.Errorf(".env was modified:\n got %q\nwant %q", data, original)
	}
	if got := c.BaseURL(); got != "https://mw.simulator.company/papi/1.0" {
		t.Errorf("base URL = %q, want it unchanged", got)
	}
}

// The tool list is fixed for the session, so `login` stays registered and
// explains itself rather than vanishing (a missing tool invites the model to
// claim it cannot authenticate at all).
func TestLoginStaysRegisteredAndIsMarkedDisabled(t *testing.T) {
	apiKeyEnv(t)

	s := server.NewMCPServer("test", "0.0.0")
	c := apiclient.New("https://mw.simulator.company/papi/1.0", "ws1", func() (string, error) { return "Bearer k", nil }, false)
	registerAuth(s, c, config.Profile{Name: "prod"}, false)

	desc := toolDescription(t, s, "login")
	if desc == "" {
		t.Fatal("login is not registered in API-key mode")
	}
	if !strings.Contains(desc, "DISABLED") {
		t.Errorf("login description = %q, want it marked DISABLED", desc)
	}
}

// newClient wires an in-process MCP client to s and initializes it, so tests can
// exercise a registered tool through the same path a host would.
func newClient(t *testing.T, s *server.MCPServer) *client.Client {
	t.Helper()
	cli, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	var init mcp.InitializeRequest
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "auth-mode-test", Version: "0"}
	if _, err := cli.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return cli
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = args
	return newClient(t, s).CallTool(context.Background(), req)
}

func toolDescription(t *testing.T, s *server.MCPServer, name string) string {
	t.Helper()
	list, err := newClient(t, s).ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for i := range list.Tools {
		if list.Tools[i].Name == name {
			return list.Tools[i].Description
		}
	}
	return ""
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestLoginDescriptionIsNormalInOAuthMode(t *testing.T) {
	t.Setenv("SIMULATOR_WORK_DIR", t.TempDir())
	t.Setenv(auth.APISecretEnv, "")

	s := server.NewMCPServer("test", "0.0.0")
	c := apiclient.New("https://mw.simulator.company/papi/1.0", "ws1", func() (string, error) { return "Simulator j", nil }, false)
	registerAuth(s, c, config.Profile{Name: "prod"}, false)

	desc := toolDescription(t, s, "login")
	if strings.Contains(desc, "DISABLED") {
		t.Errorf("login description = %q, should not be disabled in OAuth mode", desc)
	}
	if !strings.Contains(desc, "OAuth2 PKCE") {
		t.Errorf("login description = %q, want the normal OAuth description", desc)
	}
}
