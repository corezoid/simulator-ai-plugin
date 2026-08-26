package apiclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The client must be scheme-agnostic: whatever AuthHeader returns goes on the
// wire verbatim, so API-key mode needs no change here.
func TestBearerSchemeIsForwardedVerbatim(t *testing.T) {
	var capt capture
	srv := newServer(200, &capt)
	defer srv.Close()

	c := New(srv.URL, "ws1", func() (string, error) { return "Bearer wsk_sekret", nil }, false)
	if _, err := c.Do(context.Background(), "GET", "/forms", nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if capt.auth != "Bearer wsk_sekret" {
		t.Errorf("auth = %q, want %q", capt.auth, "Bearer wsk_sekret")
	}
}

func TestAuthHintAppendedOnAuthFailures(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(http1Name(status), func(t *testing.T) {
			var capt capture
			srv := newServer(status, &capt)
			defer srv.Close()

			c := New(srv.URL, "ws1", func() (string, error) { return "Bearer k", nil }, false)
			c.AuthHint = func(int) string { return "check your API key" }

			_, err := c.Do(context.Background(), "GET", "/forms", nil, nil)
			if err == nil {
				t.Fatal("Do() error = nil, want an APIError")
			}
			if !strings.Contains(err.Error(), "check your API key") {
				t.Errorf("error = %q, want it to carry the hint", err)
			}

			// Downstream code type-asserts on *APIError; the hint must not disturb that.
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %T, want *APIError", err)
			}
			if apiErr.Status != status {
				t.Errorf("Status = %d, want %d", apiErr.Status, status)
			}
			if apiErr.Hint == "" {
				t.Error("Hint is empty")
			}
		})
	}
}

// The Client asks for every status and appends only what it gets back, so a
// hint function that declines (returns "") leaves the error untouched.
func TestAuthHintNotAppendedWhenHintDeclines(t *testing.T) {
	var capt capture
	srv := newServer(404, &capt)
	defer srv.Close()

	c := New(srv.URL, "ws1", func() (string, error) { return "Bearer k", nil }, false)
	c.AuthHint = func(status int) string {
		if status == 404 {
			return ""
		}
		return "check your API key"
	}

	_, err := c.Do(context.Background(), "GET", "/forms", nil, nil)
	if err == nil {
		t.Fatal("Do() error = nil, want an APIError")
	}
	if strings.Contains(err.Error(), "check your API key") {
		t.Errorf("error = %q, want no hint when the hint function declines", err)
	}
}

func TestNoHintWhenAuthHintNil(t *testing.T) {
	var capt capture
	srv := newServer(401, &capt)
	defer srv.Close()

	c := New(srv.URL, "ws1", func() (string, error) { return "Bearer k", nil }, false)

	_, err := c.Do(context.Background(), "GET", "/forms", nil, nil)
	if err == nil {
		t.Fatal("Do() error = nil, want an APIError")
	}
	if got, want := err.Error(), `API returned 401: {"ok":true}`; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestDoRawAppendsAuthHint(t *testing.T) {
	var capt capture
	srv := newServer(403, &capt)
	defer srv.Close()

	c := New(srv.URL, "ws1", func() (string, error) { return "Bearer k", nil }, false)
	c.AuthHint = func(int) string { return "check your API key" }

	_, err := c.DoRaw(context.Background(), "GET", "/files/1", nil, nil, 1024)
	if err == nil {
		t.Fatal("DoRaw() error = nil, want an APIError")
	}
	if !strings.Contains(err.Error(), "check your API key") {
		t.Errorf("error = %q, want it to carry the hint", err)
	}
}

func http1Name(status int) string {
	if status == 401 {
		return "401 unauthorized"
	}
	return "403 forbidden"
}
