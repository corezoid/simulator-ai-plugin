package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/hosted"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
)

var prodInfo = mcpserver.Info{
	Profile:    "prod",
	APIBaseURL: "https://mw.simulator.company/papi/1.0",
	AccountURL: "https://account.corezoid.com",
}

func TestHostedConfigDefaults(t *testing.T) {
	for _, k := range []string{"SIMULATOR_RESOURCE_URL", "SIMULATOR_AUTH_SERVER_URL", "SIMULATOR_RESOLVER_ACCOUNT_URL", "SIMULATOR_RESOLVER_ALLOWED_ORIGINS"} {
		t.Setenv(k, "")
	}
	cfg := hostedConfig(prodInfo, false)
	if cfg.ResourceURL != "" {
		t.Errorf("ResourceURL defaulted to %q; discovery must be opted into", cfg.ResourceURL)
	}
	if cfg.AuthServerURL != prodInfo.AccountURL || cfg.AccountURL != prodInfo.AccountURL {
		t.Errorf("auth/account defaults = %q / %q, want the profile account URL", cfg.AuthServerURL, cfg.AccountURL)
	}
	if cfg.FallbackURL != prodInfo.APIBaseURL {
		t.Errorf("FallbackURL = %q, want the profile API URL", cfg.FallbackURL)
	}
	if len(cfg.ResolverAllowedOrigins) != 0 {
		t.Errorf("ResolverAllowedOrigins = %v, want none", cfg.ResolverAllowedOrigins)
	}
}

func TestHostedConfigFromEnv(t *testing.T) {
	t.Setenv("SIMULATOR_RESOURCE_URL", "https://mcp.simulator.company")
	t.Setenv("SIMULATOR_AUTH_SERVER_URL", "")
	t.Setenv("SIMULATOR_RESOLVER_ACCOUNT_URL", "off")
	t.Setenv("SIMULATOR_RESOLVER_ALLOWED_ORIGINS", " https://sim.customer.com , ,https://*.customer.com")
	cfg := hostedConfig(prodInfo, false)
	if cfg.ResourceURL != "https://mcp.simulator.company" {
		t.Errorf("ResourceURL = %q", cfg.ResourceURL)
	}
	if cfg.AccountURL != "" {
		t.Errorf(`"off" must disable the resolver, got %q`, cfg.AccountURL)
	}
	if strings.Join(cfg.ResolverAllowedOrigins, ",") != "https://sim.customer.com,https://*.customer.com" {
		t.Errorf("ResolverAllowedOrigins = %v", cfg.ResolverAllowedOrigins)
	}
}

// TestHostedServerEndToEnd wires the real stateless simulator server exactly
// as runHTTPMode does and lists its tools over HTTP without a session.
// Nothing reaches the Simulator API: initialize and tools/list are local.
func TestHostedServerEndToEnd(t *testing.T) {
	t.Setenv(mcpserver.APISecretEnv, "")
	s, info, err := mcpserver.New(mcpserver.Options{Profile: "prod", Version: version, Stateless: true})
	if err != nil {
		t.Fatal(err)
	}
	cfg := hosted.Config{ResourceURL: "https://mcp.example.test", AuthServerURL: info.AccountURL, FallbackURL: info.APIBaseURL}
	ts := httptest.NewServer(http.MaxBytesHandler(hosted.NewHandler(s, cfg), maxRequestBytes))
	defer ts.Close()

	call := func(auth, body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		raw := string(b)
		if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
			if i := strings.Index(raw, "data: "); i >= 0 {
				raw = strings.SplitN(raw[i+6:], "\n", 2)[0]
			}
		}
		return resp.StatusCode, raw
	}

	if code, _ := call("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: status %d, want 401", code)
	}
	if code, body := call("Bearer t", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`); code != http.StatusOK {
		t.Fatalf("initialize: %d %.200s", code, body)
	}
	code, body := call("Bearer t", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if code != http.StatusOK {
		t.Fatalf("tools/list without session: %d %.200s", code, body)
	}
	var out struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Result.Tools) < 100 {
		t.Errorf("%d tools, want the full curated set", len(out.Result.Tools))
	}
	for _, tool := range out.Result.Tools {
		switch tool.Name {
		case "login", "set-workspace", "set-environment":
			t.Errorf("stateful helper %q exposed on the hosted server", tool.Name)
		}
	}

	big := bytes.Repeat([]byte("x"), maxRequestBytes+1)
	if code, _ := call("Bearer t", string(big)); code < 400 {
		t.Errorf("oversized body: status %d, want a 4xx", code)
	}
}
