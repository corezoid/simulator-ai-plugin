package config

import "testing"

// clearEnv blanks every SIMULATOR_* var the resolver reads so a test starts clean.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SIMULATOR_PROFILE", "SIMULATOR_API_BASE_URL", "SIMULATOR_ACCOUNT_URL", "SIMULATOR_OAUTH_CLIENT_ID", "ACCOUNT_URL"} {
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

func TestAccountURLFallback(t *testing.T) {
	clearEnv(t)
	// ACCOUNT_URL (written by set-environment) is honoured on restart when no
	// SIMULATOR_ACCOUNT_URL override is set.
	t.Setenv("ACCOUNT_URL", "https://account.derived.example")
	p, err := Resolve("prod")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.AccountURL != "https://account.derived.example" {
		t.Errorf("accountUrl = %q, want ACCOUNT_URL fallback", p.AccountURL)
	}

	// SIMULATOR_ACCOUNT_URL wins over ACCOUNT_URL.
	t.Setenv("SIMULATOR_ACCOUNT_URL", "https://account.override.example")
	p, err = Resolve("prod")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.AccountURL != "https://account.override.example" {
		t.Errorf("accountUrl = %q, want SIMULATOR_ACCOUNT_URL to win", p.AccountURL)
	}
}

func TestNormalizeAPIBaseURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://mw.simulator.company/papi/1.0", "https://mw.simulator.company/papi/1.0"},
		{"https://mw.simulator.company/papi/1.0/", "https://mw.simulator.company/papi/1.0"},
		{"my-onprem.example.com", "https://my-onprem.example.com/papi/1.0"},
		{"https://my-onprem.example.com", "https://my-onprem.example.com/papi/1.0"},
		{"localhost:9000", "http://localhost:9000/papi/1.0"},
		{"http://localhost:9000", "http://localhost:9000/papi/1.0"},
		{"127.0.0.1:9000", "http://127.0.0.1:9000/papi/1.0"},
		{"[::1]:9000", "http://[::1]:9000/papi/1.0"},
		{"[2001:db8::1]:8443", "https://[2001:db8::1]:8443/papi/1.0"},
		{"https://gw.example.com/papi/2.0", "https://gw.example.com/papi/2.0"},
		{"  https://gw.example.com  ", "https://gw.example.com/papi/1.0"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeAPIBaseURL(tc.in); got != tc.want {
			t.Errorf("NormalizeAPIBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOfferedEnvironmentsGatesLocal(t *testing.T) {
	// Cloud-only when not a local-dev session.
	cloud := OfferedEnvironments(false)
	if len(cloud) != len(CloudEnvironments) {
		t.Errorf("non-dev offered %d presets, want %d (cloud only)", len(cloud), len(CloudEnvironments))
	}
	if _, ok := PresetByName(cloud, "local"); ok {
		t.Error("non-dev session must NOT offer the local preset")
	}

	// Local preset appended in a dev session.
	dev := OfferedEnvironments(true)
	if len(dev) != len(CloudEnvironments)+1 {
		t.Fatalf("dev offered %d presets, want %d", len(dev), len(CloudEnvironments)+1)
	}
	e, ok := PresetByName(dev, "local")
	if !ok || e.APIBaseURL != "http://localhost:9000/papi/1.0" {
		t.Errorf("dev session local preset = %+v, ok=%v", e, ok)
	}

	// mw resolves in both; bogus never does. And OfferedEnvironments(false) must
	// not be mutated by the dev append (shared-backing-array guard).
	if _, ok := PresetByName(cloud, "local"); ok {
		t.Error("cloud slice leaked the local preset after building the dev slice")
	}
	if e, ok := PresetByName(dev, "mw"); !ok || e.APIBaseURL != "https://mw.simulator.company/papi/1.0" {
		t.Errorf("PresetByName(mw) = %+v, ok=%v", e, ok)
	}
	if _, ok := PresetByName(dev, "bogus"); ok {
		t.Error("PresetByName(bogus) should be !ok")
	}
}
