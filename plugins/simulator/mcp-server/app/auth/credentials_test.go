package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolateEnv points .env at a temp dir and clears every credential var, so a
// developer with SIMULATOR_API_SECRET exported doesn't silently take a different
// branch through Load().
func isolateEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	t.Setenv(APISecretEnv, "")
	t.Setenv("ACCESS_TOKEN", "")
	t.Setenv("ACCESS_TOKEN_EXPIRES_AT", "")
	return dir
}

func TestLoadPrecedenceAPISecretWins(t *testing.T) {
	isolateEnv(t)
	t.Setenv(APISecretEnv, "wsk_key")
	t.Setenv("ACCESS_TOKEN", "jwt_token")

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if creds == nil {
		t.Fatal("Load() = nil, want API-key credentials")
	}
	if creds.AccessToken != "wsk_key" {
		t.Errorf("AccessToken = %q, want the API key", creds.AccessToken)
	}
	if creds.TokenType != TokenTypeBearer {
		t.Errorf("TokenType = %q, want %q", creds.TokenType, TokenTypeBearer)
	}
	if !creds.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero", creds.ExpiresAt)
	}
	if got := creds.AuthorizationHeader(); got != "Bearer wsk_key" {
		t.Errorf("AuthorizationHeader() = %q, want %q", got, "Bearer wsk_key")
	}
}

func TestLoadOAuthWhenNoAPISecret(t *testing.T) {
	isolateEnv(t)
	t.Setenv("ACCESS_TOKEN", "jwt_token")

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if creds == nil {
		t.Fatal("Load() = nil, want OAuth credentials")
	}
	if creds.TokenType != TokenTypeSimulator {
		t.Errorf("TokenType = %q, want %q", creds.TokenType, TokenTypeSimulator)
	}
	if got := creds.AuthorizationHeader(); got != "Simulator jwt_token" {
		t.Errorf("AuthorizationHeader() = %q, want %q", got, "Simulator jwt_token")
	}
}

func TestLoadNilWhenNeitherSet(t *testing.T) {
	isolateEnv(t)

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if creds != nil {
		t.Fatalf("Load() = %+v, want nil", creds)
	}
}

// AuthorizationHeader is the single point where the scheme is chosen for ~20
// header call sites across both HTTP stacks; the empty-TokenType default must
// keep meaning "Simulator".
func TestAuthorizationHeaderScheme(t *testing.T) {
	cases := []struct {
		name  string
		creds Credentials
		want  string
	}{
		{"bearer", Credentials{AccessToken: "k", TokenType: TokenTypeBearer}, "Bearer k"},
		{"simulator", Credentials{AccessToken: "j", TokenType: TokenTypeSimulator}, "Simulator j"},
		{"empty token type defaults to simulator", Credentials{AccessToken: "j"}, "Simulator j"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.creds.AuthorizationHeader(); got != tc.want {
				t.Errorf("AuthorizationHeader() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A stale ACCESS_TOKEN_EXPIRES_AT left in .env by a previous login must not be
// applied to API-key credentials — otherwise the engine stack would report "not
// authenticated" while the curated stack (which never checks expiry) kept working.
func TestIsExpiredAPIKeyIgnoresStaleExpiry(t *testing.T) {
	isolateEnv(t)
	t.Setenv(APISecretEnv, "wsk_key")
	t.Setenv("ACCESS_TOKEN_EXPIRES_AT", time.Now().Add(-24*time.Hour).Format(time.RFC3339))

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !creds.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero (stale expiry must not bleed onto the key)", creds.ExpiresAt)
	}
	if IsExpired(creds) {
		t.Error("IsExpired() = true for API-key credentials, want false")
	}
}

func TestIsExpired(t *testing.T) {
	cases := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{"nil", nil, true},
		{"empty token", &Credentials{}, true},
		{"zero expiry never expires", &Credentials{AccessToken: "t"}, false},
		{"past", &Credentials{AccessToken: "t", ExpiresAt: time.Now().Add(-time.Hour)}, true},
		{"future", &Credentials{AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsExpired(tc.creds); got != tc.want {
				t.Errorf("IsExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSaveRefusesInAPIKeyMode(t *testing.T) {
	dir := isolateEnv(t)
	t.Setenv(APISecretEnv, "wsk_key")

	err := Save(&Credentials{AccessToken: "jwt", TokenType: TokenTypeSimulator})
	if err == nil {
		t.Fatal("Save() error = nil, want a refusal in API-key mode")
	}
	if !strings.Contains(err.Error(), APISecretEnv) {
		t.Errorf("Save() error = %q, should name %s", err, APISecretEnv)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(statErr) {
		t.Errorf(".env was created despite the refusal (stat err = %v)", statErr)
	}
	if os.Getenv("ACCESS_TOKEN") != "" {
		t.Error("Save() set ACCESS_TOKEN in the process env despite refusing")
	}
}

// set-environment calls Delete(); a user-managed API key must survive it.
func TestDeleteKeepsAPISecret(t *testing.T) {
	dir := isolateEnv(t)
	envPath := filepath.Join(dir, ".env")
	original := APISecretEnv + "=wsk_key\nACCESS_TOKEN=jwt\nACCESS_TOKEN_EXPIRES_AT=2026-01-01T00:00:00Z\nWORKSPACE_ID=ws1\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	if err := Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, APISecretEnv+"=wsk_key") {
		t.Errorf("Delete() removed the API key; .env = %q", got)
	}
	if !strings.Contains(got, "WORKSPACE_ID=ws1") {
		t.Errorf("Delete() removed WORKSPACE_ID; .env = %q", got)
	}
	if strings.Contains(got, "ACCESS_TOKEN") {
		t.Errorf("Delete() left an ACCESS_TOKEN line; .env = %q", got)
	}
}

func TestEnvFilePathUsesWorkDir(t *testing.T) {
	dir := isolateEnv(t)
	if got, want := envFilePath(), filepath.Join(dir, ".env"); got != want {
		t.Errorf("envFilePath() = %q, want %q", got, want)
	}
}
