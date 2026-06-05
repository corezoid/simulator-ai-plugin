package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPublicConfigReturnsSaURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "" {
			t.Errorf("config request should be unauthenticated, got Authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"saUrl":"https://acct.example.com/","apiUrl":"x"}}`))
	}))
	defer srv.Close()

	saURL, err := FetchPublicConfig(context.Background(), srv.URL+"/papi/1.0", false)
	if err != nil {
		t.Fatalf("FetchPublicConfig: %v", err)
	}
	if saURL != "https://acct.example.com" { // trailing slash trimmed
		t.Errorf("saURL = %q, want https://acct.example.com", saURL)
	}
	if gotPath != "/papi/1.0/config" {
		t.Errorf("path = %q, want /papi/1.0/config", gotPath)
	}
}

func TestFetchPublicConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"empty saUrl", `{"data":{"saUrl":""}}`, 200},
		{"missing data", `{"foo":"bar"}`, 200},
		{"malformed json", `not json`, 200},
		{"non-2xx", `{"error":"nope"}`, 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if _, err := FetchPublicConfig(context.Background(), srv.URL, false); err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}

func TestFetchPublicConfigUnreachable(t *testing.T) {
	// Closed server → connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := FetchPublicConfig(context.Background(), url, false)
	if err == nil {
		t.Fatalf("want error for unreachable host, got nil")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error = %v, want a request-failed error", err)
	}
}
