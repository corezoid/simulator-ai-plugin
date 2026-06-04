package config

import "testing"

// clearEnv blanks every SIMULATOR_* var the resolver reads so a test starts clean.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SIMULATOR_PROFILE", "SIMULATOR_API_BASE_URL", "SIMULATOR_ACCOUNT_URL", "SIMULATOR_OAUTH_CLIENT_ID"} {
		t.Setenv(k, "")
	}
}

func TestResolveDefaultsToProd(t *testing.T) {
	clearEnv(t)
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name != "prod" {
		t.Errorf("name = %q, want prod", p.Name)
	}
	if p.APIBaseURL != "https://mw.simulator.company/papi/1.0" {
		t.Errorf("apiBaseUrl = %q", p.APIBaseURL)
	}
}

func TestResolveLocal(t *testing.T) {
	clearEnv(t)
	p, err := Resolve("local")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.APIBaseURL != "http://localhost:9000/papi/1.0" {
		t.Errorf("apiBaseUrl = %q, want local :9000", p.APIBaseURL)
	}
	if p.AccountURL != "https://account.pre.corezoid.com" {
		t.Errorf("accountUrl = %q, want pre SA", p.AccountURL)
	}
}

func TestProfileFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("SIMULATOR_PROFILE", "local")
	p, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name != "local" {
		t.Errorf("name = %q, want local (from env)", p.Name)
	}
}

func TestEnvOverridesBaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("SIMULATOR_API_BASE_URL", "http://example.test/papi/1.0/")
	p, err := Resolve("local")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.APIBaseURL != "http://example.test/papi/1.0" { // trailing slash trimmed
		t.Errorf("apiBaseUrl = %q, want override without trailing slash", p.APIBaseURL)
	}
}

func TestUnknownProfileErrors(t *testing.T) {
	clearEnv(t)
	if _, err := Resolve("does-not-exist"); err == nil {
		t.Error("expected error for unknown profile, got nil")
	}
}
