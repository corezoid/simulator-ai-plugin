// Package apiclient is the thin HTTP layer between MCP tools and the pong-server
// public API. It owns the base URL, the Authorization header, request timeouts,
// and turning non-2xx responses into errors. It knows nothing about specific
// operations — tools describe what to call; the client just calls it.
package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client performs authenticated requests against one API base URL.
type Client struct {
	AuthHeader func() (string, error) // returns the full Authorization value, e.g. "Simulator <jwt>"
	HTTP       *http.Client

	mu          sync.RWMutex // guards baseURL + workspaceID (set-environment / set-workspace mutate them at runtime)
	baseURL     string       // e.g. http://localhost:9000/papi/1.0
	workspaceID string       // accId default for {accId} path/query params
}

// New builds a Client with a sane default HTTP client (timeout + connection reuse).
// insecure skips TLS verification — only for self-signed on-prem gateways.
func New(baseURL, workspaceID string, authHeader func() (string, error), insecure bool) *Client {
	tr := &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — opt-in via --insecure
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		workspaceID: workspaceID,
		AuthHeader:  authHeader,
		HTTP:        &http.Client{Timeout: 60 * time.Second, Transport: tr},
	}
}

// BaseURL returns the current API base URL, safe for concurrent reads.
func (c *Client) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// SetBaseURL updates the API base URL at runtime (used by set-environment).
func (c *Client) SetBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = strings.TrimRight(baseURL, "/")
}

// WorkspaceID returns the current default workspace (accId), safe for concurrent reads.
func (c *Client) WorkspaceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.workspaceID
}

// SetWorkspaceID updates the default workspace at runtime (used by set-workspace).
func (c *Client) SetWorkspaceID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workspaceID = id
}

// IsInsecureCredentialTransport reports whether baseURL would send the bearer
// token over plaintext HTTP to a non-loopback host. The token is attached to every
// request, so a remote http:// endpoint would expose it on the wire.
func IsInsecureCredentialTransport(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return false
	}
	return true
}

// APIError carries the HTTP status and response body of a non-2xx reply.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned %d: %s", e.Status, e.Body)
}

// Do issues a request. path is relative to BaseURL (e.g. "/forms/123"); query may
// be nil; body may be nil, or any JSON-serialisable value. On a 2xx it returns the
// raw response body; on a non-2xx it returns an *APIError.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	full := c.BaseURL() + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, full, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.AuthHeader != nil {
		auth, err := c.AuthHeader()
		if err != nil {
			return nil, err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(respBody)}
	}
	return respBody, nil
}
