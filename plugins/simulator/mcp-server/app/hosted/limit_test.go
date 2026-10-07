package hosted

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// A caller over its in-flight budget gets 429 at once; other callers and the
// same caller after a call finishes are not affected.
func TestTokenLimiter(t *testing.T) {
	block := make(chan struct{})
	var started sync.WaitGroup
	h := newTokenLimiter(2).limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Done()
		<-block
	}))
	do := func(token string) int {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	started.Add(2)
	var done sync.WaitGroup
	for i := 0; i < 2; i++ {
		done.Add(1)
		go func() { defer done.Done(); do("Bearer a") }()
	}
	started.Wait()

	if code := do("Bearer a"); code != http.StatusTooManyRequests {
		t.Fatalf("third concurrent call for one token: %d, want 429", code)
	}
	started.Add(1)
	go func() { do("Bearer b") }()
	started.Wait() // another token is served while a is at its limit

	close(block)
	done.Wait()
	started.Add(1)
	if code := do("Bearer a"); code != http.StatusOK {
		t.Fatalf("after calls finished: %d, want 200", code)
	}
}

func TestTokenLimiterDisabled(t *testing.T) {
	if newTokenLimiter(-1) != nil {
		t.Fatal("negative limit must disable the limiter")
	}
	if l := newTokenLimiter(0); l == nil || l.max != defaultMaxConcurrentPerToken {
		t.Fatal("zero must mean the default limit")
	}
}
