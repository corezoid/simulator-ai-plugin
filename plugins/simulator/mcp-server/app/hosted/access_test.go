package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
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
	if e := got[2]; e.Status != 401 || e.Caller != "" || e.Method != "initialize" {
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
