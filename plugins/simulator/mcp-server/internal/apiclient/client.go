// Package apiclient is the thin HTTP layer between MCP tools and the pong-server
// public API. It owns the base URL, the Authorization header, request timeouts,
// and turning non-2xx responses into errors. It knows nothing about specific
// operations — tools describe what to call; the client just calls it.
package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
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

// ctxKey is the unexported type for per-request context override keys.
// Use the WithXxx / XxxFromContext helpers below — callers never touch this directly.
type ctxKey int

const (
	authCtxKey ctxKey = iota
	baseURLCtxKey
	workspaceCtxKey
	actorCtxKey
	uiContextCtxKey
)

// UIContext is the decoded `control-events-context` — where the user is in the
// Simulator web UI when they triggered the AI agent. Carried per request so
// tools (e.g. buildLink) can default to the user's current view. All fields are
// optional; absent ones are "".
type UIContext struct {
	HostOrigin  string `json:"hostOrigin"`  // web-app origin, e.g. https://mw.simulator.company
	WorkspaceID string `json:"workspaceId"` // active workspace (full UUID)
	ActiveActor string `json:"activeActor"` // UUID of the open/focused actor
	ActiveLayer string `json:"activeLayer"` // UUID of the open graph layer
	ActiveGraph string `json:"activeGraph"` // UUID of the open graph (folder)
}

// ParseUIContext decodes a `control-events-context` header value into a UIContext.
// The value is base64-encoded JSON (as pong-server sends it); a plain-JSON value
// is also accepted as a fallback. Any decode error yields a zero UIContext — the
// caller treats "no context" and "bad context" the same (best-effort awareness).
func ParseUIContext(headerValue string) UIContext {
	var ui UIContext
	if headerValue == "" {
		return ui
	}
	raw := []byte(headerValue)
	if decoded, err := base64.StdEncoding.DecodeString(headerValue); err == nil {
		raw = decoded
	}
	_ = json.Unmarshal(raw, &ui) // best-effort; zero value on failure
	return ui
}

// WithUIContext stores the decoded UI context on ctx. A zero-value context is
// still stored (harmless); callers read it with UIContextFromContext.
func WithUIContext(ctx context.Context, ui UIContext) context.Context {
	return context.WithValue(ctx, uiContextCtxKey, ui)
}

// UIContextFromContext returns the per-request UI context, or a zero UIContext.
func UIContextFromContext(ctx context.Context) UIContext {
	ui, _ := ctx.Value(uiContextCtxKey).(UIContext)
	return ui
}

// WithAuthorization stores the full Authorization header value on ctx so that
// Client.Do uses it instead of calling the Client's AuthHeader callback.
// Empty value is ignored.
func WithAuthorization(ctx context.Context, value string) context.Context {
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, authCtxKey, value)
}

// WithBaseURL stores a per-request API base URL override on ctx so that
// Client.Do routes against it instead of the Client's stored base URL.
// Empty value is ignored.
func WithBaseURL(ctx context.Context, value string) context.Context {
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, baseURLCtxKey, strings.TrimRight(value, "/"))
}

// WithWorkspaceID stores a per-request workspace id override on ctx so that
// Client.WorkspaceIDForContext returns it instead of the stored default.
// Empty value is ignored.
func WithWorkspaceID(ctx context.Context, value string) context.Context {
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, workspaceCtxKey, value)
}

// AuthorizationFromContext returns the per-request Authorization header value,
// or "" if none was attached.
func AuthorizationFromContext(ctx context.Context) string {
	v, _ := ctx.Value(authCtxKey).(string)
	return v
}

// BaseURLFromContext returns the per-request API base URL override, or "".
func BaseURLFromContext(ctx context.Context) string {
	v, _ := ctx.Value(baseURLCtxKey).(string)
	return v
}

// WorkspaceIDFromContext returns the per-request workspace id override, or "".
func WorkspaceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(workspaceCtxKey).(string)
	return v
}

// WithActorID stores a per-request actor id on ctx, switching the session into
// actor-scoped mode. Tools/list returns only the per-actor subset (with the
// actor identity hidden from every schema), and tool handlers inject the
// actor id into every request that needs it. Empty value is ignored.
func WithActorID(ctx context.Context, value string) context.Context {
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, actorCtxKey, value)
}

// ActorIDFromContext returns the per-request actor id, or "" if none was
// attached. A non-empty value means the request runs in actor-scoped mode.
func ActorIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(actorCtxKey).(string)
	return v
}

// WorkspaceIDForContext returns the per-request workspace id (from ctx) if set,
// otherwise the Client's stored default. Tool handlers that read the workspace
// should call this so stateless/SSE deployments can override per request.
func (c *Client) WorkspaceIDForContext(ctx context.Context) string {
	if id := WorkspaceIDFromContext(ctx); id != "" {
		return id
	}
	return c.WorkspaceID()
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
	req, err := c.buildRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s %s failed: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(respBody)}
	}
	return respBody, nil
}

// RawResponse is the result of DoRaw: the response body, its Content-Type, and
// whether the body was cut off at the requested byte limit.
type RawResponse struct {
	Body        []byte
	ContentType string
	Truncated   bool
}

// DoRaw issues a request and returns the raw body plus its Content-Type, without
// assuming a JSON response — used to fetch binary content such as file downloads.
// limit caps the number of body bytes read (limit <= 0 means no cap); Truncated
// reports the cap was hit (the body was longer). A non-2xx reply yields an
// *APIError carrying the (capped) body text.
func (c *Client) DoRaw(ctx context.Context, method, path string, query url.Values, body any, limit int64) (RawResponse, error) {
	req, err := c.buildRequest(ctx, method, path, query, body)
	if err != nil {
		return RawResponse{}, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return RawResponse{}, fmt.Errorf("request to %s %s failed: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var reader io.Reader = resp.Body
	if limit > 0 {
		reader = io.LimitReader(resp.Body, limit+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return RawResponse{}, fmt.Errorf("read response from %s %s: %w", method, path, err)
	}
	truncated := false
	if limit > 0 && int64(len(data)) > limit {
		data = data[:limit]
		truncated = true
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RawResponse{}, &APIError{Status: resp.StatusCode, Body: string(data)}
	}
	return RawResponse{
		Body:        data,
		ContentType: resp.Header.Get("Content-Type"),
		Truncated:   truncated,
	}, nil
}

// buildRequest assembles the *http.Request shared by Do and DoRaw — it resolves
// the base URL (ctx override else stored), appends the query, marshals a JSON
// body, and attaches the Authorization header (ctx override else AuthHeader).
func (c *Client) buildRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	base := BaseURLFromContext(ctx)
	if base == "" {
		base = c.BaseURL()
	}
	full := base + path
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
	if ctxAuth := AuthorizationFromContext(ctx); ctxAuth != "" {
		req.Header.Set("Authorization", ctxAuth)
	} else if c.AuthHeader != nil {
		auth, err := c.AuthHeader()
		if err != nil {
			return nil, err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
	}
	return req, nil
}
