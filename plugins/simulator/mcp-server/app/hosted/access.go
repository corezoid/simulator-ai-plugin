package hosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// accessPeekBytes bounds how much of a request body is read to find the
// JSON-RPC method and tool name; the full body still reaches the server.
const accessPeekBytes = 64 << 10

// accessEntry is one line of the access log: which JSON-RPC method and tool
// were called, how it ended and how long it took. It never carries the token,
// the arguments or the result — only a short hash grouping one caller's calls,
// and the client's User-Agent.
type accessEntry struct {
	Method  string `json:"method,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Route   string `json:"route,omitempty"`
	Status  int    `json:"status"`
	IsError bool   `json:"is_error,omitempty"`
	Ms      int64  `json:"ms"`
	Caller  string `json:"caller,omitempty"`
	Client  string `json:"client,omitempty"`

	mu sync.Mutex
}

type accessKey struct{}

// callerHash is a stable, non-reversible id for an Authorization value.
func callerHash(auth string) string {
	if auth == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(auth))
	return hex.EncodeToString(sum[:6])
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// statusRecorder keeps the status code and still streams (Flush passes through).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// accessLog wraps an MCP route so every request leaves one log line.
func accessLog(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		e := &accessEntry{Route: route}
		if r.Method == http.MethodPost && r.Body != nil {
			peek, _ := io.ReadAll(io.LimitReader(r.Body, accessPeekBytes))
			r.Body = struct {
				io.Reader
				io.Closer
			}{io.MultiReader(bytes.NewReader(peek), r.Body), r.Body}
			var msg struct {
				Method string `json:"method"`
				Params struct {
					Name string `json:"name"`
				} `json:"params"`
			}
			if json.Unmarshal(peek, &msg) == nil {
				e.Method = clip(msg.Method, 64)
				if msg.Method == "tools/call" {
					e.Tool = clip(msg.Params.Name, 64)
				}
			}
		} else {
			e.Method = r.Method
		}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), accessKey{}, e)))

		e.mu.Lock()
		defer e.mu.Unlock()
		e.Status = rec.status
		if e.Status == 0 {
			e.Status = http.StatusOK
		}
		e.Ms = time.Since(start).Milliseconds()
		e.Caller = callerHash(normalizeAuthorization(r.Header.Get("Authorization")))
		e.Client = clip(r.UserAgent(), 80)
		line, _ := json.Marshal(e)
		log.Printf("access %s", line)
	})
}

// accessToolMiddleware records whether the tool call failed; the HTTP status
// of a failed tool call is still 200.
func accessToolMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := next(ctx, req)
		if e, ok := ctx.Value(accessKey{}).(*accessEntry); ok {
			e.mu.Lock()
			e.IsError = err != nil || (result != nil && result.IsError)
			e.mu.Unlock()
		}
		return result, err
	}
}
