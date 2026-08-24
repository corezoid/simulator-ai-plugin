package auth

import (
	"strings"
	"testing"
)

func TestIsAPIKeyModeAndMode(t *testing.T) {
	cases := []struct {
		name     string
		secret   string
		wantOn   bool
		wantMode string
	}{
		{"unset", "", false, ModeOAuth},
		{"set", "wsk_abc123", true, ModeAPIKey},
		{"whitespace only", "   ", false, ModeOAuth},
		{"padded value", "  wsk_abc123  ", true, ModeAPIKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(APISecretEnv, tc.secret)
			if got := IsAPIKeyMode(); got != tc.wantOn {
				t.Errorf("IsAPIKeyMode() = %v, want %v", got, tc.wantOn)
			}
			if got := Mode(); got != tc.wantMode {
				t.Errorf("Mode() = %q, want %q", got, tc.wantMode)
			}
		})
	}
}

func TestAPISecretTrimsSurroundingSpace(t *testing.T) {
	t.Setenv(APISecretEnv, "  wsk_abc123\t")
	if got := APISecret(); got != "wsk_abc123" {
		t.Errorf("APISecret() = %q, want %q", got, "wsk_abc123")
	}
}

// The hint is user-facing and lands in tool results and telemetry-adjacent error
// strings, so it must never carry credential material — in any mode, any status.
func TestHintForNeverContainsSecret(t *testing.T) {
	const secret = "SUPERSECRET-do-not-leak"
	for _, mode := range []string{secret, ""} {
		t.Setenv(APISecretEnv, mode)
		for _, status := range []int{401, 403, 404, 500} {
			if h := HintFor(status); strings.Contains(h, secret) {
				t.Fatalf("HintFor(%d) leaked the secret: %q", status, h)
			}
		}
	}
}

func TestHintForOnlyCoversCredentialStatuses(t *testing.T) {
	t.Setenv(APISecretEnv, "wsk_key")
	for _, status := range []int{200, 400, 404, 409, 429, 500} {
		if h := HintFor(status); h != "" {
			t.Errorf("HintFor(%d) = %q, want no hint", status, h)
		}
	}
}

func TestHintFor401NamesTheCredential(t *testing.T) {
	t.Setenv(APISecretEnv, "wsk_key")
	h := HintFor(401)
	if !strings.Contains(h, APISecretEnv) {
		t.Errorf("HintFor(401) should name %s, got %q", APISecretEnv, h)
	}
	if !strings.Contains(h, "WORKSPACE_ID") {
		t.Errorf("HintFor(401) should name WORKSPACE_ID as a suspect, got %q", h)
	}

	t.Setenv(APISecretEnv, "")
	if h := HintFor(401); !strings.Contains(h, "login") {
		t.Errorf("OAuth HintFor(401) should point at `login`, got %q", h)
	}
}

// A 403 is returned for a missing or foreign object far more often than for a
// bad credential, so it must not assert the credential is wrong — that sends the
// user hunting in the wrong place.
func TestHintFor403DoesNotBlameTheCredential(t *testing.T) {
	for _, mode := range []string{"wsk_key", ""} {
		t.Setenv(APISecretEnv, mode)
		h := HintFor(403)
		if h == "" {
			t.Fatal("HintFor(403) = \"\", want guidance")
		}
		if strings.Contains(h, "rejected your") {
			t.Errorf("HintFor(403) = %q, must not assert the credential was rejected", h)
		}
		if !strings.Contains(h, "does not exist") {
			t.Errorf("HintFor(403) = %q, should lead with the likely cause", h)
		}
	}
}
