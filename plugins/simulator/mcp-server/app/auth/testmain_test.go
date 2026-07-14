package auth

import (
	"os"
	"testing"
)

// TestMain pins SIMULATOR_WORK_DIR to a throwaway directory for the whole
// package: the credential helpers write to {SIMULATOR_WORK_DIR|cwd}/.env, and
// a test that forgets t.Setenv must corrupt a sandbox, not a real .env.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "sim-auth-test-*")
	if err == nil {
		os.Setenv("SIMULATOR_WORK_DIR", tmp)
	}
	os.Unsetenv("ACCESS_TOKEN")
	os.Unsetenv("ACCESS_TOKEN_EXPIRES_AT")
	os.Unsetenv("REFRESH_TOKEN")
	code := m.Run()
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}
