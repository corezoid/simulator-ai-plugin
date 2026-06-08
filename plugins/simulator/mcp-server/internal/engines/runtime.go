package engines

import "github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"

// Configure sets the API base URL and TLS mode. Called once at startup by cmd/server.
func Configure(baseURL string, insecure bool) { ecore.Configure(baseURL, insecure) }

// SetBaseURL updates the API base URL at runtime (used by set-environment).
func SetBaseURL(baseURL string) { ecore.SetBaseURL(baseURL) }

// ResetAuth clears the cached authorization (used when switching environment).
func ResetAuth() { ecore.ResetAuth() }
