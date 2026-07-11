package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGateway serves the public config endpoint ({base}/config) declaring the
// given saUrl — the shape login's self-heal reads via FetchPublicConfig.
func fakeGateway(t *testing.T, saURL string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"saUrl":"` + saURL + `"}}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func setupEnvFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	envPath := filepath.Join(dir, ".env")
	if content != "" {
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return envPath
}

func TestResolveAccountURL_HealsPoisonedEnv(t *testing.T) {
	sa := "https://account.example.com"
	gw := fakeGateway(t, sa)
	envPath := setupEnvFile(t, "ACCOUNT_URL=https://admin.corezoid.com\n")
	t.Setenv("ACCOUNT_URL", "https://admin.corezoid.com")

	got, note := resolveAccountURL(context.Background(), gw.URL, "https://admin.corezoid.com", false)
	if got != sa {
		t.Fatalf("want healed %q, got %q", sa, got)
	}
	if !strings.Contains(note, "Self-healed") || !strings.Contains(note, "admin.corezoid.com") || !strings.Contains(note, sa) {
		t.Fatalf("note must explain what was wrong and what was done, got: %q", note)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ACCOUNT_URL="+sa) {
		t.Fatalf(".env not healed: %s", data)
	}
}

func TestResolveAccountURL_MatchIsSilent(t *testing.T) {
	sa := "https://account.example.com"
	gw := fakeGateway(t, sa)
	setupEnvFile(t, "")

	// Trailing slash and case must not count as a mismatch.
	got, note := resolveAccountURL(context.Background(), gw.URL, "https://ACCOUNT.example.com/", false)
	if got != "https://ACCOUNT.example.com/" {
		t.Fatalf("matching env value must be kept as-is, got %q", got)
	}
	if note != "" {
		t.Fatalf("no note expected when env matches the gateway, got: %q", note)
	}
}

func TestResolveAccountURL_GatewayUnreachableKeepsEnv(t *testing.T) {
	setupEnvFile(t, "")
	env := "https://account.onprem.example"
	got, note := resolveAccountURL(context.Background(), "http://127.0.0.1:1/papi/1.0", env, false)
	if got != env {
		t.Fatalf("unreachable gateway must keep env value, got %q", got)
	}
	if !strings.Contains(note, "Could not verify") || !strings.Contains(note, env) {
		t.Fatalf("note must say verification was skipped, got: %q", note)
	}
}

func TestResolveAccountURL_EmptyEnvDerivesFromGateway(t *testing.T) {
	sa := "https://account.onprem.example"
	gw := fakeGateway(t, sa)
	envPath := setupEnvFile(t, "")

	got, note := resolveAccountURL(context.Background(), gw.URL, "", false)
	if got != sa {
		t.Fatalf("want derived %q, got %q", sa, got)
	}
	if !strings.Contains(note, "was not set") || !strings.Contains(note, sa) {
		t.Fatalf("note must explain the derivation, got: %q", note)
	}
	data, _ := os.ReadFile(envPath)
	if !strings.Contains(string(data), "ACCOUNT_URL="+sa) {
		t.Fatalf(".env not persisted: %s", data)
	}
}

func TestLoginFailureMsg_OffersManualURLAndNextSteps(t *testing.T) {
	authURL := "https://account.example.com/oauth2/authorize?client_id=x"
	msg := loginFailureMsg(context.DeadlineExceeded, authURL,
		"https://account.example.com", "https://gw.example.com/papi/1.0", "🔧 healed note")
	for _, want := range []string{authURL, "set-environment", "healed note", "consent"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("failure message missing %q:\n%s", want, msg)
		}
	}
}

func TestProbeRequested(t *testing.T) {
	cases := []struct {
		args map[string]any
		want bool
	}{
		{map[string]any{"probe": true}, true},
		{map[string]any{"probe": false}, false},
		{map[string]any{"probe": "true"}, true},
		{map[string]any{"probe": "1"}, true},
		{map[string]any{"probe": "no"}, false},
		{map[string]any{}, false},
	}
	for _, c := range cases {
		if got := probeRequested(c.args); got != c.want {
			t.Fatalf("probeRequested(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}
