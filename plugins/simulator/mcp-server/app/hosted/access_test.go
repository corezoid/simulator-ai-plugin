package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Every MCP request leaves one access line with method, tool and outcome, and
// never the token, the arguments or the result.
func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	s := echoServer()
	s.AddTool(mcp.NewTool("fail"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, errors.New("boom secret-result")
	})
	cfg := prodLike
	cfg.AccessLog = true
	ts := newTestServerWith(t, s, cfg)

	auth := map[string]string{"Authorization": "Bearer secret-token-123", "User-Agent": "openai-mcp/1.0.0"}
	post(t, ts.URL+"/mcp", auth, callBody)
	post(t, ts.URL+"/mcp/workspaces/ws-1", auth, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"fail","arguments":{"secret-arg":"x"}}}`)
	post(t, ts.URL+"/mcp", nil, initBody)

	var got []*accessEntry
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		rest, ok := strings.CutPrefix(l, "access ")
		if !ok {
			continue
		}
		e := &accessEntry{}
		if err := json.Unmarshal([]byte(rest), e); err != nil {
			t.Fatalf("bad access line %q: %v", l, err)
		}
		got = append(got, e)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 access lines, got %d:\n%s", len(got), buf.String())
	}
	if e := got[0]; e.Method != "tools/call" || e.Tool != "echo-ctx" || e.Route != "bare" || e.IsError || e.Status != 200 ||
		e.Caller != callerHash(normalizeAuthorization("Bearer secret-token-123")) || len(e.Caller) != 12 || e.Client != "openai-mcp/1.0.0" {
		t.Errorf("successful call: %+v", e)
	}
	if e := got[1]; !e.IsError || e.Tool != "fail" || e.Route != "workspace" {
		t.Errorf("failed call: %+v", e)
	}
	// refused before its body is read, so its JSON-RPC method stays unknown
	if e := got[2]; e.Status != 401 || e.Caller != "" || e.Method != "" {
		t.Errorf("anonymous request: %+v", e)
	}
	for _, leak := range []string{"secret-token-123", "secret-arg", "secret-result"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("log contains %q", leak)
		}
	}
}

func newTestServerWith(t *testing.T, s *server.MCPServer, cfg Config) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(NewHandler(s, cfg))
	t.Cleanup(ts.Close)
	return ts
}

// accessLines captures the log for the test and returns a reader of the
// access entries written so far.
func accessLines(t *testing.T) func() []*accessEntry {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return func() []*accessEntry {
		var got []*accessEntry
		for l := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
			if rest, ok := strings.CutPrefix(l, "access "); ok {
				e := &accessEntry{}
				if err := json.Unmarshal([]byte(rest), e); err != nil {
					t.Fatalf("bad access line %q: %v", l, err)
				}
				got = append(got, e)
			}
		}
		return got
	}
}

// A request body larger than any peek buffer (an inline base64 image, a long
// description) still logs its method and tool, and a non-tool method is named.
func TestAccessLogAttributesLargeRequests(t *testing.T) {
	lines := accessLines(t)
	cfg := prodLike
	cfg.AccessLog = true
	ts := newTestServerWith(t, echoServer(), cfg)

	auth := map[string]string{"Authorization": "Bearer t"}
	big := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo-ctx","arguments":{"blob":"` +
		strings.Repeat("A", 100<<10) + `"}}}`
	if resp, _ := post(t, ts.URL+"/mcp", auth, big); resp.StatusCode != 200 {
		t.Fatalf("large call: %d", resp.StatusCode)
	}
	post(t, ts.URL+"/mcp", auth, initBody)

	got := lines()
	if len(got) != 2 {
		t.Fatalf("want 2 access lines, got %d", len(got))
	}
	if e := got[0]; e.Method != "tools/call" || e.Tool != "echo-ctx" {
		t.Errorf("large call lost its attribution: %+v", e)
	}
	if e := got[1]; e.Method != "initialize" || e.Tool != "" {
		t.Errorf("initialize: %+v", e)
	}
}

// readCounter counts body bytes read.
type readCounter struct {
	r io.Reader
	n int
}

func (c *readCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// The access log runs before requireAuth: it must not read the body, so an
// anonymous request is refused without its body being consumed.
func TestAccessLogDoesNotReadBodyBeforeAuth(t *testing.T) {
	accessLines(t)
	cfg := prodLike
	cfg.AccessLog = true
	h := NewHandler(echoServer(), cfg)

	body := &readCounter{r: strings.NewReader(initBody)}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request: %d", rec.Code)
	}
	if body.n != 0 {
		t.Errorf("read %d body bytes before authorization", body.n)
	}
}
