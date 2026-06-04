package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type capture struct {
	method string
	path   string
	query string
	auth   string
	body   map[string]any
}

func newServer(status int, capt *capture) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capt.method = r.Method
		capt.path = r.URL.Path
		capt.query = r.URL.RawQuery
		capt.auth = r.Header.Get("Authorization")
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &capt.body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

func TestDoSendsMethodPathQueryBodyAuth(t *testing.T) {
	var capt capture
	srv := newServer(200, &capt)
	defer srv.Close()

	c := New(srv.URL, "ws1", func() (string, error) { return "Simulator tok123", nil }, false)
	q := url.Values{}
	q.Set("withRelations", "true")
	resp, err := c.Do(context.Background(), "POST", "/forms/ws1/true", q, map[string]any{"title": "Car"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(resp) != `{"ok":true}` {
		t.Errorf("resp = %s", resp)
	}
	if capt.method != "POST" {
		t.Errorf("method = %s", capt.method)
	}
	if capt.path != "/forms/ws1/true" {
		t.Errorf("path = %s", capt.path)
	}
	if capt.query != "withRelations=true" {
		t.Errorf("query = %s", capt.query)
	}
	if capt.auth != "Simulator tok123" {
		t.Errorf("auth = %s", capt.auth)
	}
	if capt.body["title"] != "Car" {
		t.Errorf("body = %v", capt.body)
	}
}

func TestDoNon2xxReturnsAPIError(t *testing.T) {
	var capt capture
	srv := newServer(400, &capt)
	defer srv.Close()

	c := New(srv.URL, "", nil, false)
	_, err := c.Do(context.Background(), "GET", "/forms/1", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 400 {
		t.Errorf("status = %d, want 400", apiErr.Status)
	}
}
