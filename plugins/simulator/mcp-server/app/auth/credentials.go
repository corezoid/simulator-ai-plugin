package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// envMu serialises all .env read-modify-write operations. updateEnvFile and
// removeEnvKey are NOT self-locking (so a caller can hold the lock across
// several mutations); the exported functions below acquire it.
var envMu sync.Mutex

// Credentials holds the Simulator JWT token.
type Credentials struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	TokenType   string    `json:"token_type"` // always "Simulator"
}

// AuthorizationHeader returns the value to use for the Authorization header.
func (c *Credentials) AuthorizationHeader() string {
	tokenType := c.TokenType
	if tokenType == "" {
		tokenType = "Simulator"
	}
	return tokenType + " " + c.AccessToken
}

// envFilePath returns the path to the .env file in the current working directory.
func envFilePath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".env")
}

// updateEnvFileMulti reads the .env file once, applies all key=value updates,
// and writes it back once. Writing several keys in a single pass avoids the
// crash-window where a multi-write left the file half-updated (e.g. a token
// with no expiry line). Not self-locking — callers hold envMu.
func updateEnvFileMulti(path string, kv [][2]string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(string(data), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	for _, pair := range kv {
		prefix := pair[0] + "="
		found := false
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				lines[i] = prefix + pair[1]
				found = true
				break
			}
		}
		if !found {
			lines = append(lines, prefix+pair[1])
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

// updateEnvFile writes or updates a single key=value. Not self-locking.
func updateEnvFile(path, key, value string) error {
	return updateEnvFileMulti(path, [][2]string{{key, value}})
}

// removeEnvKey removes a key from the .env file.
// Returns nil if the file does not exist.
func removeEnvKey(path, key string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	prefix := key + "="
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, prefix) {
			kept = append(kept, line)
		}
	}

	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}

	content := ""
	if len(kept) > 0 {
		content = strings.Join(kept, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(content), 0600)
}

// Load reads credentials from environment variables.
// The env vars are populated from .env by FindAndLoadDotEnv() at startup.
// Returns nil, nil if ACCESS_TOKEN is not set.
func Load() (*Credentials, error) {
	token := os.Getenv("ACCESS_TOKEN")
	if token == "" {
		return nil, nil
	}
	creds := &Credentials{
		AccessToken: token,
		TokenType:   "Simulator",
	}
	if expiryStr := os.Getenv("ACCESS_TOKEN_EXPIRES_AT"); expiryStr != "" {
		if t, err := time.Parse(time.RFC3339, expiryStr); err == nil {
			creds.ExpiresAt = t
		}
	}
	return creds, nil
}

// Save writes ACCESS_TOKEN (and optionally ACCESS_TOKEN_EXPIRES_AT)
// to the .env file in the current working directory, and updates the in-process env vars.
func Save(creds *Credentials) error {
	envMu.Lock()
	defer envMu.Unlock()

	kv := [][2]string{{"ACCESS_TOKEN", creds.AccessToken}}
	var expStr string
	if !creds.ExpiresAt.IsZero() {
		expStr = creds.ExpiresAt.Format(time.RFC3339)
		kv = append(kv, [2]string{"ACCESS_TOKEN_EXPIRES_AT", expStr})
	}
	if err := updateEnvFileMulti(envFilePath(), kv); err != nil {
		return fmt.Errorf("failed to save token to .env: %w", err)
	}
	os.Setenv("ACCESS_TOKEN", creds.AccessToken)
	if expStr != "" {
		os.Setenv("ACCESS_TOKEN_EXPIRES_AT", expStr)
	}
	return nil
}

// Delete removes ACCESS_TOKEN and ACCESS_TOKEN_EXPIRES_AT
// from the .env file and from the in-process environment.
func Delete() error {
	envMu.Lock()
	defer envMu.Unlock()
	path := envFilePath()
	if err := removeEnvKey(path, "ACCESS_TOKEN"); err != nil {
		return err
	}
	if err := removeEnvKey(path, "ACCESS_TOKEN_EXPIRES_AT"); err != nil {
		return err
	}
	os.Unsetenv("ACCESS_TOKEN")
	os.Unsetenv("ACCESS_TOKEN_EXPIRES_AT")
	return nil
}

// SaveAccountURL saves ACCOUNT_URL to the .env file.
func SaveAccountURL(accountURL string) error {
	envMu.Lock()
	defer envMu.Unlock()
	path := envFilePath()
	if err := updateEnvFile(path, "ACCOUNT_URL", accountURL); err != nil {
		return fmt.Errorf("failed to save ACCOUNT_URL to .env: %w", err)
	}
	os.Setenv("ACCOUNT_URL", accountURL)
	return nil
}

// SaveEnvironment saves the chosen environment — SIMULATOR_API_BASE_URL and
// ACCOUNT_URL — to the .env file in a single read-modify-write pass, so it can't
// leave .env with a new base URL but a stale account URL. config.Resolve reads both
// on startup, so the choice survives a restart.
func SaveEnvironment(apiBaseURL, accountURL string) error {
	envMu.Lock()
	defer envMu.Unlock()
	kv := [][2]string{
		{"SIMULATOR_API_BASE_URL", apiBaseURL},
		{"ACCOUNT_URL", accountURL},
	}
	if err := updateEnvFileMulti(envFilePath(), kv); err != nil {
		return fmt.Errorf("failed to save environment to .env: %w", err)
	}
	os.Setenv("SIMULATOR_API_BASE_URL", apiBaseURL)
	os.Setenv("ACCOUNT_URL", accountURL)
	return nil
}

// ClearWorkspaceID removes WORKSPACE_ID from the .env file and the process env.
// Used when switching environment, since workspaces are per-environment.
func ClearWorkspaceID() error {
	envMu.Lock()
	defer envMu.Unlock()
	if err := removeEnvKey(envFilePath(), "WORKSPACE_ID"); err != nil {
		return err
	}
	os.Unsetenv("WORKSPACE_ID")
	return nil
}

// SaveWorkspaceID saves WORKSPACE_ID to the .env file.
func SaveWorkspaceID(accID string) error {
	envMu.Lock()
	defer envMu.Unlock()
	path := envFilePath()
	if err := updateEnvFile(path, "WORKSPACE_ID", accID); err != nil {
		return fmt.Errorf("failed to save workspace ID to .env: %w", err)
	}
	os.Setenv("WORKSPACE_ID", accID)
	return nil
}

// IsExpired reports whether the credentials are expired.
func IsExpired(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	if creds.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(creds.ExpiresAt)
}
