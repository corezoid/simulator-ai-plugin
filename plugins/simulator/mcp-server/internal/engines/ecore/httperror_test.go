package ecore

import (
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
)

func TestHTTPStatusErrorAddsHintOnAuthFailures(t *testing.T) {
	t.Setenv(auth.APISecretEnv, "wsk_key")

	for _, status := range []int{401, 403} {
		err := HTTPStatusError("GET", "https://example.test/papi/1.0/forms", status, []byte(`{"msg":"nope"}`))
		if !strings.Contains(err.Error(), auth.APISecretEnv) {
			t.Errorf("status %d: error = %q, want the API-key hint", status, err)
		}
		if !strings.Contains(err.Error(), "GET https://example.test/papi/1.0/forms") {
			t.Errorf("status %d: error = %q, lost the request context", status, err)
		}
	}
}

func TestHTTPStatusErrorNoHintOnServerError(t *testing.T) {
	t.Setenv(auth.APISecretEnv, "wsk_key")

	err := HTTPStatusError("POST", "https://example.test/x", 500, []byte("boom"))
	if strings.Contains(err.Error(), auth.APISecretEnv) {
		t.Errorf("error = %q, hint must be confined to 401/403", err)
	}
}

// In stateless mode the credential came from the caller's header, so this
// process has no advice to give — and must not hint at its own env vars.
func TestHTTPStatusErrorSuppressedWhenStateless(t *testing.T) {
	t.Setenv(auth.APISecretEnv, "wsk_key")
	SetStateless(true)
	t.Cleanup(func() { SetStateless(false) })

	err := HTTPStatusError("GET", "https://example.test/x", 401, []byte("nope"))
	if strings.Contains(err.Error(), auth.APISecretEnv) {
		t.Errorf("error = %q, want no hint in stateless mode", err)
	}
}

func TestHTTPStatusErrorOAuthHint(t *testing.T) {
	t.Setenv(auth.APISecretEnv, "")

	err := HTTPStatusError("GET", "https://example.test/x", 401, []byte("nope"))
	if !strings.Contains(err.Error(), "login") {
		t.Errorf("error = %q, want the OAuth hint to point at `login`", err)
	}
}
