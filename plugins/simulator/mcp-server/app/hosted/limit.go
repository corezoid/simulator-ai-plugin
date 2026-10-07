package hosted

import (
	"crypto/sha256"
	"net/http"
	"strconv"
	"sync"
)

// defaultMaxConcurrentPerToken caps the requests one caller may have in
// flight. A tool call holds its slot until it returns (a simulation can
// stream for minutes), so one account cannot occupy every worker of a
// replica. AI platforms call on behalf of many users from shared IPs, which
// is why the key is the caller's token, not the source address.
const defaultMaxConcurrentPerToken = 4

// tokenLimiter counts in-flight requests per Authorization header.
type tokenLimiter struct {
	max      int
	mu       sync.Mutex
	inFlight map[[sha256.Size]byte]int
}

// newTokenLimiter returns nil (no limit) for max < 0; 0 means the default.
func newTokenLimiter(max int) *tokenLimiter {
	if max < 0 {
		return nil
	}
	if max == 0 {
		max = defaultMaxConcurrentPerToken
	}
	return &tokenLimiter{max: max, inFlight: map[[sha256.Size]byte]int{}}
}

func (l *tokenLimiter) acquire(key [sha256.Size]byte) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight[key] >= l.max {
		return false
	}
	l.inFlight[key]++
	return true
}

func (l *tokenLimiter) release(key [sha256.Size]byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight[key] <= 1 {
		delete(l.inFlight, key)
		return
	}
	l.inFlight[key]--
}

// limit wraps next so a caller over its in-flight budget gets 429 at once.
// Only the hash of the token is kept, never the token itself.
func (l *tokenLimiter) limit(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		key := sha256.Sum256([]byte(auth))
		if !l.acquire(key) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32000,"message":"too many concurrent requests for this account (limit ` +
				strconv.Itoa(l.max) + `); wait for a running tool call to finish and retry"}}`))
			return
		}
		defer l.release(key)
		next.ServeHTTP(w, r)
	})
}
