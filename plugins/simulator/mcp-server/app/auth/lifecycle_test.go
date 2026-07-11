package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPKCEFlow_HonorsCtxCancel(t *testing.T) {
	orig := openBrowserFn
	openBrowserFn = func(string) error { return nil }
	defer func() { openBrowserFn = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, authURL, err := PKCEFlow(ctx, "https://account.example.com", "", nil)
	if err == nil || !strings.Contains(err.Error(), "cancelled by the client") {
		t.Fatalf("want cancellation error, got %v", err)
	}
	if !strings.Contains(authURL, "https://account.example.com/oauth2/authorize?") {
		t.Fatalf("authURL must be returned for a manual fallback, got %q", authURL)
	}
}

func TestBackupToken_SavesRemovedLines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	envPath := filepath.Join(dir, ".env")
	content := "SIMULATOR_API_BASE_URL=https://gw.example.com/papi/1.0\nACCESS_TOKEN=tok123\nACCESS_TOKEN_EXPIRES_AT=2026-12-01T00:00:00Z\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	bak, err := BackupToken()
	if err != nil {
		t.Fatal(err)
	}
	if bak != envPath+".bak" {
		t.Fatalf("unexpected backup path %q", bak)
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "ACCESS_TOKEN=tok123") || !strings.Contains(s, "ACCESS_TOKEN_EXPIRES_AT=2026-12-01T00:00:00Z") {
		t.Fatalf("backup missing token lines: %s", s)
	}
	if !strings.Contains(s, "# saved by simulator logout") {
		t.Fatalf("backup missing annotation: %s", s)
	}
	if strings.Contains(s, "SIMULATOR_API_BASE_URL") {
		t.Fatalf("backup must contain only the token lines: %s", s)
	}
}

func TestBackupToken_NothingToBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("WORKSPACE_ID=1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	bak, err := BackupToken()
	if err != nil {
		t.Fatal(err)
	}
	if bak != "" {
		t.Fatalf("want empty path when there is nothing to back up, got %q", bak)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.bak")); !os.IsNotExist(err) {
		t.Fatalf(".env.bak must not be created when there is nothing to back up")
	}
}

func TestBackupToken_MissingEnvFile(t *testing.T) {
	t.Setenv("SIMULATOR_WORK_DIR", t.TempDir())
	bak, err := BackupToken()
	if err != nil || bak != "" {
		t.Fatalf("missing .env must be a no-op, got path=%q err=%v", bak, err)
	}
}

func TestSaveDeleteRefreshTokenRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)
	t.Setenv("REFRESH_TOKEN", "")
	os.Unsetenv("REFRESH_TOKEN")

	if err := Save(&Credentials{AccessToken: "at-1", RefreshToken: "rt-1", TokenType: "Simulator"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(data), "REFRESH_TOKEN=rt-1") {
		t.Fatalf("refresh token not persisted: %s", data)
	}
	// A grant without a refresh token must not clobber the stored one.
	if err := Save(&Credentials{AccessToken: "at-2", TokenType: "Simulator"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(data), "REFRESH_TOKEN=rt-1") {
		t.Fatalf("token-only save must keep the refresh token: %s", data)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, ".env"))
	if strings.Contains(string(data), "REFRESH_TOKEN") || os.Getenv("REFRESH_TOKEN") != "" {
		t.Fatalf("logout must remove the refresh token: %s", data)
	}
}
