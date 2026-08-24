package auth

import (
	"os"
	"strconv"
	"strings"
)

// Authorization header token types. The platform's own OAuth flow issues a JWT
// sent as "Simulator <jwt>"; a user-supplied workspace API key is sent as the
// standard "Bearer <key>". pong-server accepts both (checkPublicApiAuth.js —
// see docs/INTEGRATION.md §1), and the dumped spec declares the header as a
// generic apiKey scheme with a "Bearer #TOKEN#" sample.
const (
	TokenTypeSimulator = "Simulator"
	TokenTypeBearer    = "Bearer"
)

// APISecretEnv is the env var that switches the server into API-key mode.
//
// It is deliberately prefixed: .mcp.json declares no env allow-list, so the
// server inherits the host's whole environment. An unprefixed name like
// API_SECRET is common enough in developer shells that an unrelated third-party
// secret would silently be sent to the Simulator gateway as a Bearer token.
const APISecretEnv = "SIMULATOR_API_SECRET" // #nosec G101 — the variable's name, not a credential

// AllowInsecureAPISecretEnv opts out of the refusal to send a long-lived API key
// over plaintext HTTP to a non-local host. Loopback is already exempt, so this
// exists only for trusted-network on-prem gateways with no TLS.
const AllowInsecureAPISecretEnv = "SIMULATOR_ALLOW_INSECURE_API_SECRET"

// Auth mode names, reported by Mode() for startup logging and diagnostics.
const (
	ModeAPIKey    = "api-key"
	ModeOAuth     = "oauth"
	ModeStateless = "stateless"
)

// APISecret returns the configured workspace API key, or "" when unset.
//
// The value is user-supplied — hand-written into .env or exported into the
// environment. It is never written back to .env, never logged, and never
// included in telemetry.
func APISecret() string { return strings.TrimSpace(os.Getenv(APISecretEnv)) }

// IsAPIKeyMode reports whether an API key is configured. In this mode the OAuth
// PKCE flow is disabled end to end: `login` refuses and explains itself, Save
// refuses to write a token, set-environment refuses to re-point the gateway,
// and a 401 is reported as a rejected key rather than an expired session.
func IsAPIKeyMode() bool { return APISecret() != "" }

// Mode returns the active auth mode name for logging.
func Mode() string {
	if IsAPIKeyMode() {
		return ModeAPIKey
	}
	return ModeOAuth
}

// HintFor returns a short, actionable remediation for a rejected request, or ""
// when the status is not credential-related. It never contains any part of the
// credential.
//
// 401 and 403 need different wording. A 401 is unambiguous: the credential
// itself was refused. A 403 usually means the object is not yours to touch —
// the backend returns it for a missing or foreign resource — so it must not
// assert that the credential is wrong; a workspace-scoped key pointed at
// another workspace's object produces exactly the same status, which is why the
// hint is offered at all rather than dropped.
func HintFor(status int) string {
	switch status {
	case 401:
		if IsAPIKeyMode() {
			return "the Simulator API rejected your " + APISecretEnv + " — check that the key is correct " +
				"and not revoked, that WORKSPACE_ID names the workspace the key was issued for, and that " +
				"the key belongs to this environment (workspace access keys are created at " +
				"account.corezoid.com → workspace, and are scoped to one workspace on one gateway)"
		}
		return "your Simulator session is invalid or expired — run the `login` tool again"
	case 403:
		if IsAPIKeyMode() {
			return "access denied — usually the object does not exist or belongs to another workspace. " +
				"If you expected access, check that WORKSPACE_ID matches the workspace your " + APISecretEnv +
				" was issued for, since a key grants nothing outside its own workspace"
		}
		return "access denied — usually the object does not exist or you lack permission on it; " +
			"if you expected access, check that WORKSPACE_ID is the right workspace"
	default:
		return ""
	}
}

// InsecureAPISecretAllowed reports whether the plaintext-HTTP refusal is waived.
//
// Parsed as a bool rather than "any non-empty value": a guard that
// SIMULATOR_ALLOW_INSECURE_API_SECRET=0 or =false would switch OFF is a trap, so
// only a value that actually says yes counts, and anything unparseable is a no.
func InsecureAPISecretAllowed() bool {
	v := strings.TrimSpace(os.Getenv(AllowInsecureAPISecretEnv))
	if v == "" {
		return false
	}
	allowed, err := strconv.ParseBool(v)
	return err == nil && allowed
}
