// Package config resolves the runtime environment ("profile") for the MCP server:
// the API base URL, the OAuth account (SA) URL, and the OAuth client id.
//
// Resolution order (highest precedence first):
//
//  1. explicit per-field env vars (SIMULATOR_API_BASE_URL, SIMULATOR_ACCOUNT_URL,
//     SIMULATOR_OAUTH_CLIENT_ID)
//  2. a profiles.json file in SIMULATOR_WORK_DIR (or cwd), keyed by the active profile name
//  3. the built-in profile (local / prod)
//
// The active profile name comes from the --profile flag (passed to Resolve) or the
// SIMULATOR_PROFILE env var, defaulting to "prod".
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// CloudEnv is a named, ready-to-pick cloud environment offered to the user before
// login. The account/SA URL is not stored here — it is derived per environment from
// the gateway's public config (getConfigReq → saUrl).
type CloudEnv struct {
	Name       string // short selector, e.g. "mw"
	Label      string // human-facing description
	APIBaseURL string // public API root incl. /papi/1.0 prefix
}

// CloudEnvironments are the shipped cloud gateways, in presentation order (mw is the
// default). Users may also enter a custom/local URL instead of picking a preset.
var CloudEnvironments = []CloudEnv{
	{Name: "mw", Label: "Simulator Cloud — mw.simulator.company", APIBaseURL: "https://mw.simulator.company/papi/1.0"},
	{Name: "sim", Label: "Simulator Cloud — sim.simulator.company", APIBaseURL: "https://sim.simulator.company/papi/1.0"},
}

// LocalEnvironment is the dev preset for a pong-server running on :9000. It is
// offered by set-environment only in a local-dev session (see OfferedEnvironments),
// so regular cloud users are never prompted with localhost.
var LocalEnvironment = CloudEnv{
	Name:       "local",
	Label:      "Local dev — localhost:9000",
	APIBaseURL: "http://localhost:9000/papi/1.0",
}

// OfferedEnvironments returns the presets set-environment should advertise. The
// local preset is appended only when localDev is true (the server was started with
// the "local" profile, i.e. SIMULATOR_PROFILE=local / --profile local) — the same
// signal the repo's dev loop (make run-local) already uses.
func OfferedEnvironments(localDev bool) []CloudEnv {
	if localDev {
		return append(append([]CloudEnv{}, CloudEnvironments...), LocalEnvironment)
	}
	return CloudEnvironments
}

// PresetByName returns the preset with the given name among the supplied list, or
// false if unknown. Pass the result of OfferedEnvironments so "local" only resolves
// in a dev session.
func PresetByName(presets []CloudEnv, name string) (CloudEnv, bool) {
	for _, e := range presets {
		if e.Name == name {
			return e, true
		}
	}
	return CloudEnv{}, false
}

// NormalizeAPIBaseURL turns a user-entered environment (a bare host, a host:port, or
// a full URL — with or without the /papi/<version> prefix) into a canonical API base
// URL. It defaults to https:// when no scheme is given (except for localhost /
// 127.0.0.1, which default to http://), and appends /papi/1.0 when no /papi/ segment
// is present. The result has no trailing slash.
func NormalizeAPIBaseURL(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		host := hostOf(s)
		scheme := "https"
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			scheme = "http"
		}
		s = scheme + "://" + s
	}
	s = strings.TrimRight(s, "/")
	if !strings.Contains(s, "/papi/") {
		s += "/papi/1.0"
	}
	return s
}

// hostOf extracts the bare host from a scheme-less authority, handling the
// bracketed IPv6 form ("[::1]:9000" → "::1") as well as "host", "host:port",
// and "host/path".
func hostOf(s string) string {
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end > 0 {
			return s[1:end]
		}
	}
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		return s[:i]
	}
	return s
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
	// SIMULATOR_API_BASE_URL and ACCOUNT_URL are written by the set-environment tool;
	// honouring them here means a chosen environment survives a server restart.
	p.APIBaseURL = firstNonEmpty(os.Getenv("SIMULATOR_API_BASE_URL"), p.APIBaseURL)
	p.AccountURL = firstNonEmpty(os.Getenv("SIMULATOR_ACCOUNT_URL"), os.Getenv("ACCOUNT_URL"), p.AccountURL)
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
	path := "profiles.json"
	if dir := os.Getenv("SIMULATOR_WORK_DIR"); dir != "" {
		path = filepath.Join(dir, "profiles.json")
	}
	data, err := os.ReadFile(path)
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
