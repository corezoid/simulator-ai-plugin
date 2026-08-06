package telemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- classifyError ---------------------------------------------------------

func TestClassifyError(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"authentication failed", "auth_error"},
		{"invalid token supplied", "auth_error"},
		{"401 Unauthorized", "auth_error"},
		{"403 Forbidden", "auth_error"},
		{"validation failed: bad shape", "validation_error"},
		{"node is invalid", "validation_error"},
		{"lint reported issues", "validation_error"},
		{"resource not found", "not_found"},
		{"got 404 from server", "not_found"},
		{"api returned error", "api_error"},
		{"http request failed", "api_error"},
		{"fetch error", "api_error"},
		{"something else entirely", "unknown"},
		{"", "unknown"},
	}
	for _, c := range cases {
		got := classifyError(c.in)
		if got != c.want {
			t.Errorf("classifyError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClassifyError_CaseInsensitive(t *testing.T) {
	if got := classifyError("AUTHENTICATION FAILED"); got != "auth_error" {
		t.Errorf("expected auth_error for upper-case input, got %q", got)
	}
}

// ---- hostnameOnly ----------------------------------------------------------

func TestHostnameOnly(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://api.simulator.company/papi/1.0", "api.simulator.company"},
		{"http://example.com:8080/path?x=1", "example.com"},
		{"api.simulator.company", "api.simulator.company"},
		{"api.simulator.company/path", "api.simulator.company"},
		{"", ""},
	}
	for _, c := range cases {
		got := hostnameOnly(c.in)
		if got != c.want {
			t.Errorf("hostnameOnly(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHostnameOnly_InvalidURL(t *testing.T) {
	// Pass a URL that fails url.Parse — strings with control characters.
	got := hostnameOnly("https://exa mple.com")
	if got != "" {
		t.Errorf("expected empty string for malformed URL, got %q", got)
	}
}

// ---- generateUUIDv4 --------------------------------------------------------

var reUUIDv4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestGenerateUUIDv4_Format(t *testing.T) {
	id := generateUUIDv4()
	if len(id) != 36 {
		t.Fatalf("expected 36-char UUID, got %d: %q", len(id), id)
	}
	if !reUUIDv4.MatchString(id) {
		t.Errorf("UUID %q does not match v4 pattern", id)
	}
}

func TestGenerateUUIDv4_Unique(t *testing.T) {
	ids := make(map[string]struct{}, 50)
	for i := 0; i < 50; i++ {
		id := generateUUIDv4()
		if _, dup := ids[id]; dup {
			t.Fatalf("duplicate UUID generated: %s", id)
		}
		ids[id] = struct{}{}
	}
}

// ---- loadOrCreateInstallationID --------------------------------------------

// Replace HOME with a temp dir so the test does not touch the real
// ~/.simulator/installation_id file.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	t.Cleanup(func() { _ = os.Setenv("HOME", orig) })
	return dir
}

func TestLoadOrCreateInstallationID_CreatesFile(t *testing.T) {
	home := withTempHome(t)

	id := loadOrCreateInstallationID()
	if !reUUIDv4.MatchString(id) {
		t.Fatalf("generated ID %q is not a valid UUID v4", id)
	}

	// File should have been persisted.
	path := filepath.Join(home, ".simulator", "installation_id")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected installation_id file to be created: %v", err)
	}
	if strings.TrimSpace(string(data)) != id {
		t.Errorf("file content %q does not match returned id %q", strings.TrimSpace(string(data)), id)
	}
}

func TestLoadOrCreateInstallationID_ReadsExisting(t *testing.T) {
	home := withTempHome(t)

	dir := filepath.Join(home, ".simulator")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	existing := "11111111-2222-4333-8444-555555555555"
	if err := os.WriteFile(filepath.Join(dir, "installation_id"), []byte(existing+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	id := loadOrCreateInstallationID()
	if id != existing {
		t.Errorf("expected to read existing id %q, got %q", existing, id)
	}
}

func TestLoadOrCreateInstallationID_RegeneratesOnInvalidLength(t *testing.T) {
	home := withTempHome(t)

	dir := filepath.Join(home, ".simulator")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Length != 36 — should be discarded and a new ID generated.
	if err := os.WriteFile(filepath.Join(dir, "installation_id"), []byte("too-short\n"), 0600); err != nil {
		t.Fatal(err)
	}

	id := loadOrCreateInstallationID()
	if id == "too-short" {
		t.Error("expected invalid id to be discarded")
	}
	if !reUUIDv4.MatchString(id) {
		t.Errorf("expected fresh UUID v4, got %q", id)
	}
}

// ---- emit -------------------------------------------------------------------

func TestEmit_DisabledIsNoOp(t *testing.T) {
	// When telemetry is disabled, emit must not panic even if eventCh is nil.
	prev := enabled.Load()
	enabled.Store(false)
	t.Cleanup(func() { enabled.Store(prev) })

	emit(Event{Tool: "test"})
}

func TestEmit_FullChannelDoesNotBlock(t *testing.T) {
	prevEnabled := enabled.Load()
	prevCh := eventCh
	t.Cleanup(func() {
		enabled.Store(prevEnabled)
		eventCh = prevCh
	})

	eventCh = make(chan Event, 1)
	enabled.Store(true)
	eventCh <- Event{Tool: "filler"} // fill the buffer

	// Second emit should drop silently rather than block. If it blocks, the
	// goroutine never closes `done` and the timeout below fires.
	done := make(chan struct{})
	go func() {
		emit(Event{Tool: "dropped"})
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(time.Second):
		t.Fatal("emit blocked on a full channel")
	}
}

// ---- Stop --------------------------------------------------------------------

func TestStop_DisabledIsNoOp(t *testing.T) {
	prev := enabled.Load()
	enabled.Store(false)
	// stopOnce is package-global; reset it so this test's call isn't silently
	// absorbed by (or doesn't silently absorb) another test's.
	stopOnce = sync.Once{}
	t.Cleanup(func() { enabled.Store(prev) })

	done := make(chan struct{})
	go func() {
		Stop()
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(time.Second):
		t.Fatal("Stop blocked when telemetry is disabled")
	}
}

// TestStop_FlushesPendingEvent starts a real sender goroutine against a local
// test server to verify Stop drains a queued event and sends it
// synchronously, instead of leaving it stranded when the process exits.
func TestStop_FlushesPendingEvent(t *testing.T) {
	prevEnabled := enabled.Load()
	prevCh := eventCh
	prevFlushCh := flushCh
	prevEndpoint := endpoint
	t.Cleanup(func() {
		enabled.Store(prevEnabled)
		eventCh = prevCh
		flushCh = prevFlushCh
		endpoint = prevEndpoint
	})

	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	endpoint = srv.URL
	eventCh = make(chan Event, 10)
	flushCh = make(chan chan struct{})
	enabled.Store(true)
	// stopOnce is package-global; reset it so an earlier test's call doesn't
	// cause this one to silently skip its work.
	stopOnce = sync.Once{}
	// runSender returns once it services the flush request below, so no
	// goroutine leaks past this test.
	go runSender()

	eventCh <- Event{Tool: "test-tool", InstallationID: "abc", Ts: "2024-01-01T00:00:00Z"}

	Stop()

	select {
	case body := <-received:
		if !strings.Contains(string(body), "test-tool") {
			t.Errorf("expected flushed batch to contain the queued event, got: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not flush the pending event before returning")
	}
}

// TestStop_SecondCallReturnsImmediately guards against the regression
// flagged in corezoid-ai-plugin's review: main.go calls Stop from up to three
// places (a deferred call, the SIGINT/SIGTERM handler, and an error path),
// and the sender goroutine exits after its first flush. Without the
// sync.Once guard, a second call finds no receiver on flushCh and blocks out
// its full 1s timeout for nothing — stacked with a signal handler racing in,
// that's up to 2s of pure, avoidable hang.
func TestStop_SecondCallReturnsImmediately(t *testing.T) {
	prevEnabled := enabled.Load()
	prevCh := eventCh
	prevFlushCh := flushCh
	prevEndpoint := endpoint
	t.Cleanup(func() {
		enabled.Store(prevEnabled)
		eventCh = prevCh
		flushCh = prevFlushCh
		endpoint = prevEndpoint
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	endpoint = srv.URL
	eventCh = make(chan Event, 10)
	flushCh = make(chan chan struct{})
	enabled.Store(true)
	stopOnce = sync.Once{}
	go runSender()

	Stop() // first call: real flush, sender goroutine returns afterward

	done := make(chan struct{})
	go func() {
		Stop() // second call: must be a no-op, not a 1s blocked send
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(200 * time.Millisecond):
		t.Fatal("second Stop call blocked instead of returning immediately")
	}
}
