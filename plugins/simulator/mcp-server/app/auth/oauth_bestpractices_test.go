package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func testJWTWithExp(t *testing.T, exp int64) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":` + strconv.FormatInt(exp, 10) + `}`))
	return header + "." + payload + ".sig"
}

func TestValidateAccountURLScheme(t *testing.T) {
	cases := []struct {
		url string
		ok  bool
	}{
		{"https://account.corezoid.com", true},
		{"http://localhost:9000", true},
		{"http://127.0.0.1:9000", true},
		{"http://account.onprem.example", false},
		{"ftp://account.corezoid.com", false},
	}
	for _, c := range cases {
		if err := validateAccountURLScheme(c.url); (err == nil) != c.ok {
			t.Errorf("validateAccountURLScheme(%q): err=%v, want ok=%v", c.url, err, c.ok)
		}
	}
}

func TestDiscoverOAuthEndpoints(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_endpoint":"` + srvURL + `/a","token_endpoint":"` + srvURL + `/t"}`))
	}))
	defer srv.Close()
	srvURL = srv.URL

	authz, token := discoverOAuthEndpoints(context.Background(), srv.URL)
	if authz != srv.URL+"/a" || token != srv.URL+"/t" {
		t.Errorf("metadata not used: %q %q", authz, token)
	}

	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	authz, token = discoverOAuthEndpoints(context.Background(), bad.URL)
	if authz != bad.URL+"/oauth2/authorize" || token != bad.URL+"/oauth2/token" {
		t.Errorf("want conventional fallback, got %q %q", authz, token)
	}
}

func TestRefresh_ExchangesAndRotates(t *testing.T) {
	tok := testJWTWithExp(t, 4102444800) // 2100-01-01
	var gotGrant, gotRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"simulator_token":"` + tok + `","refresh_token":"rt-rotated"}`))
	}))
	defer srv.Close()

	creds, err := Refresh(context.Background(), srv.URL, "client-1", "rt-old")
	if err != nil {
		t.Fatal(err)
	}
	if gotGrant != "refresh_token" || gotRefresh != "rt-old" {
		t.Errorf("wrong grant sent: %q %q", gotGrant, gotRefresh)
	}
	if creds.AccessToken != tok || creds.RefreshToken != "rt-rotated" {
		t.Errorf("rotated pair not captured: %+v", creds)
	}
}

func TestPostTokenRequest_StandardAccessTokenFallback(t *testing.T) {
	tok := testJWTWithExp(t, 4102444800)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + tok + `"}`))
	}))
	defer srv.Close()

	creds, err := postTokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"authorization_code"}})
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != tok {
		t.Errorf("standard access_token field must be accepted: %+v", creds)
	}
}

func TestPKCEFlow_RejectsInsecureAccountURL(t *testing.T) {
	orig := openBrowserFn
	opened := false
	openBrowserFn = func(string) error { opened = true; return nil }
	defer func() { openBrowserFn = orig }()

	_, _, err := PKCEFlow(context.Background(), "http://account.onprem.example", "", nil)
	if err == nil || !strings.Contains(err.Error(), "unencrypted") {
		t.Fatalf("plain-http non-local account URL must be rejected, got %v", err)
	}
	if opened {
		t.Errorf("the browser must not open for a rejected account URL")
	}
}

func TestPostTokenRequest_ParsesServiceErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"x","result":"error","message":"Wrong refresh token"}`))
	}))
	defer srv.Close()
	_, err := postTokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"refresh_token"}})
	if err == nil || !strings.Contains(err.Error(), "Wrong refresh token") {
		t.Fatalf("service error envelope must surface its message, got %v", err)
	}
}

func TestPostTokenRequest_ParsesRefreshGrantShapeAndRedacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"x","result":"ok","new_access_token":"atn_abc","new_access_token_expire":1784061715}`))
	}))
	defer srv.Close()
	creds, err := postTokenRequest(context.Background(), srv.URL, url.Values{"grant_type": {"refresh_token"}})
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "atn_abc" || creds.ExpiresAt.Unix() != 1784061715 {
		t.Errorf("refresh grant shape not parsed: %+v", creds)
	}

	leak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok","mystery_field":"secret-value-42"}`))
	}))
	defer leak.Close()
	_, err = postTokenRequest(context.Background(), leak.URL, url.Values{})
	if err == nil || strings.Contains(err.Error(), "secret-value-42") {
		t.Fatalf("error must not leak response values: %v", err)
	}
	if !strings.Contains(err.Error(), "mystery_field") {
		t.Fatalf("error should name the fields seen: %v", err)
	}
}
