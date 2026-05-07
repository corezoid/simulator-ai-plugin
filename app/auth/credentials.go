package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

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

// credentialsFilePath returns the path to the credentials file (~/.simulator/credentials.json).
func credentialsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".simulator", "credentials.json"), nil
}

// Load reads credentials from ~/.simulator/credentials.json.
// Returns nil, nil if the file does not exist.
func Load() (*Credentials, error) {
	path, err := credentialsFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// Save writes credentials to ~/.simulator/credentials.json with permissions 0600.
func Save(creds *Credentials) error {
	path, err := credentialsFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// IsExpired reports whether the credentials are expired.
func IsExpired(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	if creds.ExpiresAt.IsZero() {
		return false // no expiry set — assume still valid
	}
	return time.Now().After(creds.ExpiresAt)
}

// Delete removes the saved credentials file.
func Delete() error {
	path, err := credentialsFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
