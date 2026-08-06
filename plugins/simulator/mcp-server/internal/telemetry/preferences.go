package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Preferences holds user-level opt-in settings persisted in
// ~/.simulator/preferences.json.
type Preferences struct {
	TelemetryEmail      string `json:"telemetry_email,omitempty"`
	TelemetryEmailAsked bool   `json:"telemetry_email_asked,omitempty"`
}

func preferencesFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".simulator", "preferences.json"), nil
}

// LoadPreferences reads ~/.simulator/preferences.json, returning the zero
// value if the file is missing or unreadable.
func LoadPreferences() Preferences {
	path, err := preferencesFilePath()
	if err != nil {
		return Preferences{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Preferences{}
	}
	var p Preferences
	_ = json.Unmarshal(data, &p)
	return p
}

// SavePreferences writes p to ~/.simulator/preferences.json, creating the
// directory if needed.
func SavePreferences(p Preferences) error {
	path, err := preferencesFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// AskForEmailOnce elicits the user's email for inclusion in telemetry, once
// per installation. It is a no-op when telemetry is disabled, the connected
// client did not declare elicitation support during initialize, or the user
// has already been asked (regardless of whether they accepted or declined).
// Call it after a successful login — errors are swallowed, since email
// collection is optional and best-effort.
func AskForEmailOnce(ctx context.Context, s *server.MCPServer) {
	if !enabled.Load() {
		return
	}
	session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo)
	if !ok || session.GetClientCapabilities().Elicitation == nil {
		return
	}
	prefs := LoadPreferences()
	if prefs.TelemetryEmailAsked {
		return
	}
	prefs.TelemetryEmailAsked = true

	result, err := s.RequestElicitation(ctx, mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: "Would you like to share your email with the Corezoid team? It helps them contact you if issues arise. This is optional — press Cancel to skip.",
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{
						"type":        "string",
						"title":       "Email address",
						"description": "Optional — leave blank or press Cancel to skip",
					},
				},
			},
		},
	})
	if err == nil && result != nil && result.Action == mcp.ElicitationResponseActionAccept {
		if content, ok := result.Content.(map[string]any); ok {
			if email, _ := content["email"].(string); email != "" {
				prefs.TelemetryEmail = email
				setTelemetryEmail(email)
			}
		}
	}
	_ = SavePreferences(prefs)
}
