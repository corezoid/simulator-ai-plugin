// Package config resolves the runtime environment ("profile") for the MCP server:
// the API base URL, the OAuth account (SA) URL, and the OAuth client id.
//
// Resolution order (highest precedence first):
//
//	1. explicit per-field env vars (SIMULATOR_API_BASE_URL, SIMULATOR_ACCOUNT_URL,
//	   SIMULATOR_OAUTH_CLIENT_ID)
//	2. a profiles.json file in the working directory, keyed by the active profile name
//	3. the built-in profile (local / prod)
//
// The active profile name comes from the --profile flag (passed to Resolve) or the
// SIMULATOR_PROFILE env var, defaulting to "prod".
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Profile is the resolved set of endpoints for one environment.
type Profile struct {
	Name          string `json:"-"`
	APIBaseURL    string `json:"apiBaseUrl"`    // public API root incl. /papi/1.0 prefix
	AccountURL    string `json:"accountUrl"`    // OAuth2 / SA base
	OAuthClientID string `json:"oauthClientId"` // empty → auth package default
}

// builtins are the two shipped profiles. Local targets a dev pong-server on :9000
// authenticated against the pre SA; prod targets the public gateway.
var builtins = map[string]Profile{
	"local": {
		Name:       "local",
		APIBaseURL: "http://localhost:9000/papi/1.0",
		AccountURL: "https://account.pre.corezoid.com",
	},
	"prod": {
		Name:       "prod",
		APIBaseURL: "https://mw.simulator.company/papi/1.0",
		AccountURL: "https://account.corezoid.com",
	},
}

// profilesFile is the optional override file read from the working directory.
type profilesFile struct {
	Active   string             `json:"active"`
	Profiles map[string]Profile `json:"profiles"`
}

// Resolve picks the active profile by name (flag value; empty falls back to
// SIMULATOR_PROFILE, then profiles.json "active", then "prod") and layers
// profiles.json and env-var overrides on top of the built-in defaults.
func Resolve(flagProfile string) (Profile, error) {
	file, _ := loadProfilesFile() // best-effort; absent file is fine

	name := firstNonEmpty(flagProfile, os.Getenv("SIMULATOR_PROFILE"))
	if name == "" && file != nil {
		name = file.Active
	}
	if name == "" {
		name = "prod"
	}

	// Start from the built-in (if any), then layer profiles.json on top.
	p, known := builtins[name]
	p.Name = name
	if file != nil {
		if fp, ok := file.Profiles[name]; ok {
			p = mergeProfile(p, fp)
			p.Name = name
			known = true
		}
	}

	// Per-field env overrides win over everything.
	p.APIBaseURL = firstNonEmpty(os.Getenv("SIMULATOR_API_BASE_URL"), p.APIBaseURL)
	p.AccountURL = firstNonEmpty(os.Getenv("SIMULATOR_ACCOUNT_URL"), p.AccountURL)
	p.OAuthClientID = firstNonEmpty(os.Getenv("SIMULATOR_OAUTH_CLIENT_ID"), p.OAuthClientID)
	p.APIBaseURL = strings.TrimRight(p.APIBaseURL, "/")
	p.AccountURL = strings.TrimRight(p.AccountURL, "/")

	if !known && p.APIBaseURL == "" {
		return Profile{}, fmt.Errorf("unknown profile %q (not built-in, not in profiles.json, and no SIMULATOR_API_BASE_URL override)", name)
	}
	if p.APIBaseURL == "" {
		return Profile{}, fmt.Errorf("profile %q has no apiBaseUrl (set SIMULATOR_API_BASE_URL or add it to profiles.json)", name)
	}
	return p, nil
}

func loadProfilesFile() (*profilesFile, error) {
	data, err := os.ReadFile("profiles.json")
	if err != nil {
		return nil, err
	}
	var f profilesFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("profiles.json: %w", err)
	}
	return &f, nil
}

func mergeProfile(base, over Profile) Profile {
	base.APIBaseURL = firstNonEmpty(over.APIBaseURL, base.APIBaseURL)
	base.AccountURL = firstNonEmpty(over.AccountURL, base.AccountURL)
	base.OAuthClientID = firstNonEmpty(over.OAuthClientID, base.OAuthClientID)
	return base
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
