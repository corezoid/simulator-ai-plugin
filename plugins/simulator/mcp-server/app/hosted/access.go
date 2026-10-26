package hosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
		// The body is never read here: this runs before requireAuth and the
		// per-token limiter, so an anonymous or slow body must not hold the
		// handler. The JSON-RPC method and tool name are filled in by the MCP
		// layer (accessMethodHook, accessToolMiddleware) once the request has
		// been authorized and parsed — whatever the body's size.
		e := &accessEntry{Route: route}
		if r.Method != http.MethodPost {
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

// accessMethodHook records the JSON-RPC method of a parsed request.
func accessMethodHook(ctx context.Context, _ any, method mcp.MCPMethod, _ any) {
	if e, ok := ctx.Value(accessKey{}).(*accessEntry); ok {
		e.mu.Lock()
		e.Method = clip(string(method), 64)
		e.mu.Unlock()
	}
}

// accessToolMiddleware records the called tool and whether the call failed;
// the HTTP status of a failed tool call is still 200.
func accessToolMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := next(ctx, req)
		if e, ok := ctx.Value(accessKey{}).(*accessEntry); ok {
			e.mu.Lock()
			e.Method = string(mcp.MethodToolsCall)
			e.Tool = clip(req.Params.Name, 64)
			e.IsError = err != nil || (result != nil && result.IsError)
			e.mu.Unlock()
		}
		return result, err
	}
}
