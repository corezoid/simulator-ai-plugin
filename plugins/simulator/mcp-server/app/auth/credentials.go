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

// Credentials holds the active Simulator credential — either an OAuth JWT or a
// user-supplied workspace API key. TokenType selects the header scheme.
type Credentials struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	TokenType   string    `json:"token_type"` // TokenTypeSimulator (OAuth) or TokenTypeBearer (API key)
}

// AuthorizationHeader returns the value to use for the Authorization header.
func (c *Credentials) AuthorizationHeader() string {
	tokenType := c.TokenType
	if tokenType == "" {
		tokenType = TokenTypeSimulator
	}
	return tokenType + " " + c.AccessToken
}

// envFilePath returns the path to the .env file.
// It prefers SIMULATOR_WORK_DIR (the user's project directory, captured before the
// server cd-s into the plugin dir) and falls back to cwd for local dev runs.
func envFilePath() string {
	if dir := os.Getenv("SIMULATOR_WORK_DIR"); dir != "" {
		return filepath.Join(dir, ".env")
	}
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
		found := false
		for i, line := range lines {
			// Match on the parsed key, not a "KEY=" prefix: the loader trims a line
			// before splitting it, so it reads `  KEY=…`, `KEY = …` and a BOM'd
			// first line that the bare-prefix match missed — and a miss appended a
			// duplicate the loader then ignored in favour of the stale first
			// occurrence. The line's own prefix is preserved on rewrite.
			linePrefix, ok := envLineAssigns(line, pair[0])
			if !ok {
				continue
			}
			lines[i] = linePrefix + pair[0] + "=" + pair[1]
			found = true
			break
		}
		if !found {
			lines = append(lines, pair[0]+"="+pair[1])
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

	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		// Same normalisation as the loader — an indented or BOM'd ACCESS_TOKEN
		// line used to survive a logout, and the token came back on the next start.
		if _, assigns := envLineAssigns(line, key); !assigns {
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

// Load returns the active credentials, in precedence order:
//
//  1. SIMULATOR_API_SECRET — a user-managed workspace API key, sent as
//     "Bearer <key>". It wins over everything and disables the OAuth flow.
//  2. ACCESS_TOKEN — a Simulator JWT (written by `login`, or set by hand),
//     sent as "Simulator <jwt>".
//
// Returns nil, nil when neither is set. The env vars are populated from .env by
// cmd/server's loadDotEnv at startup, so an external edit needs a restart.
// Never returns a non-nil error; the signature is kept for callers.
func Load() (*Credentials, error) {
	if secret := APISecret(); secret != "" {
		// ExpiresAt is deliberately left zero: an API key has no client-visible
		// lifetime, so IsExpired reports false forever and EnsureAuth never
		// treats it as stale. The backend is the only authority on revocation,
		// and it surfaces that as a 401. Returning here (rather than falling
		// through) also keeps a stale ACCESS_TOKEN_EXPIRES_AT line left over
		// from a previous `login` from being applied to the key.
		return &Credentials{AccessToken: secret, TokenType: TokenTypeBearer}, nil
	}
	token := os.Getenv("ACCESS_TOKEN")
	if token == "" {
		return nil, nil
	}
	creds := &Credentials{
		AccessToken: token,
		TokenType:   TokenTypeSimulator,
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
//
// It refuses in API-key mode: an OAuth token saved there would be dead weight
// on disk (Load never reaches it) while telling the user they are authenticated
// by a credential that is not in use. The guard lives here, rather than only in
// the `login` tool, so the invariant holds for any caller.
func Save(creds *Credentials) error {
	if IsAPIKeyMode() {
		return fmt.Errorf("%s is set — refusing to write ACCESS_TOKEN to .env (unset %s to use OAuth login)",
			APISecretEnv, APISecretEnv)
	}

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
//
// SIMULATOR_API_SECRET is never touched: it is user-managed, and removeEnvKey
// matches whole keys, so only ACCESS_TOKEN lines go. Clearing a leftover OAuth token
// is still correct hygiene in API-key mode, so this is not gated on the mode.
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
//
// A zero ExpiresAt means "no known expiry" and reports false. API-key
// credentials always land in that branch: the key has no client-visible
// lifetime, so revocation can only surface as a 401 from the backend.
func IsExpired(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	if creds.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(creds.ExpiresAt)
}
