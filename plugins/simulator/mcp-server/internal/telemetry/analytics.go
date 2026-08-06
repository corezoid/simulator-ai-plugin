package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const batchSize = 20
const flushInterval = 5 * time.Second

var enabled atomic.Bool
var transport string
var serverVersion string
var installationID string
var telemetryEmail atomic.Pointer[string]
var eventCh chan Event

// setTelemetryEmail and telemetryEmailValue give telemetryEmail the same
// atomic discipline as enabled: AskForEmailOnce (triggered by login) and
// Middleware (triggered by every tool call) can otherwise run concurrently
// under a future non-stdio transport.
func setTelemetryEmail(email string) {
	telemetryEmail.Store(&email)
}

func telemetryEmailValue() string {
	if v := telemetryEmail.Load(); v != nil {
		return *v
	}
	return ""
}

// flushCh lets Stop ask the sender goroutine to drain and flush
// synchronously before process exit, avoiding the loss of events that would
// otherwise still be sitting in eventCh or the sender's local batch.
var flushCh chan chan struct{}

// endpoint and convID are set from config during Init.
var endpoint string
var convID int

// Event holds telemetry data for a single tool call.
type Event struct {
	Ts             string `json:"ts"`
	Product        string `json:"product"` // "simulator" — distinguishes this plugin's events from corezoid-ai-plugin's in the shared ingest process
	Tool           string `json:"tool"`
	DurationMs     int64  `json:"duration_ms"`
	IsError        bool   `json:"is_error"`
	ErrorType      string `json:"error_type,omitempty"`
	APIURL         string `json:"api_url,omitempty"`
	Transport      string `json:"transport"`
	ServerVersion  string `json:"server_version"`
	InstallationID string `json:"installation_id"`
	UserEmail      string `json:"user_email,omitempty"`
	ClientName     string `json:"client_name,omitempty"`
	ClientVersion  string `json:"client_version,omitempty"`
}

// classifyError maps an error result string to one of the fixed error_type
// enum values. It never returns free-form text — only the predefined enum
// values below.
func classifyError(result string) string {
	lower := strings.ToLower(result)
	switch {
	case strings.Contains(lower, "auth") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden"):
		return "auth_error"
	case strings.Contains(lower, "validation") || strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "lint"):
		return "validation_error"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "404"):
		return "not_found"
	case strings.Contains(lower, "api") || strings.Contains(lower, "http") ||
		strings.Contains(lower, "request") || strings.Contains(lower, "fetch"):
		return "api_error"
	default:
		return "unknown"
	}
}

// hostnameOnly strips scheme, path, and query from a URL, returning just the host.
func hostnameOnly(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// resultText extracts the first text content from a tool result, for
// classifyError to inspect. Returns err's message instead when the call
// failed at the protocol level (result is nil in that case).
func resultText(result *mcp.CallToolResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// loadOrCreateInstallationID reads ~/.simulator/installation_id or generates
// and persists a fresh UUID v4. Falls back to an in-memory UUID if the file
// cannot be written.
func loadOrCreateInstallationID() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return generateUUIDv4()
	}
	dir := filepath.Join(home, ".simulator")
	path := filepath.Join(dir, "installation_id")

	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); len(id) == 36 {
			return id
		}
	}

	id := generateUUIDv4()
	if err := os.MkdirAll(dir, 0700); err == nil {
		_ = os.WriteFile(path, []byte(id+"\n"), 0600)
	}
	return id
}

// generateUUIDv4 produces a random UUID v4 using crypto/rand.
func generateUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// Init loads telemetry configuration and starts the background sender,
// unless SIMULATOR_ANALYTICS_DISABLED is set. transportName identifies the
// serving transport (e.g. "stdio"); version is the running server version,
// both recorded on every event. Call once at startup, before serving.
func Init(transportName, version string) {
	cfg := loadConfig()
	endpoint = cfg.Endpoint
	convID = cfg.ConvID
	transport = transportName
	serverVersion = version

	if os.Getenv("SIMULATOR_ANALYTICS_DISABLED") != "" {
		return
	}
	installationID = loadOrCreateInstallationID()
	prefs := LoadPreferences()
	setTelemetryEmail(prefs.TelemetryEmail)
	eventCh = make(chan Event, 100)
	flushCh = make(chan chan struct{})
	enabled.Store(true)
	go runSender()
}

// Middleware returns a server.ToolHandlerMiddleware that records an Event for
// every tool call: duration, error classification, the API host currently in
// use (via apiURL, called fresh on every call so it reflects a live
// set-environment switch), and the calling client's identity. Wrap the
// MCPServer with it once, in mcpserver.New, so it applies uniformly across
// curated ops, auth helpers, and engine tools without changes to any of them.
func Middleware(apiURL func() string) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !enabled.Load() {
				return next(ctx, req)
			}
			start := time.Now()
			result, err := next(ctx, req)

			isError := err != nil || (result != nil && result.IsError)
			e := Event{
				Ts:             start.UTC().Format(time.RFC3339),
				Product:        "simulator",
				Tool:           req.Params.Name,
				DurationMs:     time.Since(start).Milliseconds(),
				IsError:        isError,
				APIURL:         hostnameOnly(apiURL()),
				Transport:      transport,
				ServerVersion:  serverVersion,
				InstallationID: installationID,
				UserEmail:      telemetryEmailValue(),
			}
			if session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo); ok {
				info := session.GetClientInfo()
				e.ClientName, e.ClientVersion = info.Name, info.Version
			}
			if isError {
				e.ErrorType = classifyError(resultText(result, err))
			}
			emit(e)
			return result, err
		}
	}
}

// emit enqueues an event for async delivery. Non-blocking: events are
// dropped silently when the channel is full.
func emit(e Event) {
	if !enabled.Load() {
		return
	}
	select {
	case eventCh <- e:
	default:
	}
}

// runSender drains eventCh and flushes batches every 5s or 20 events, until
// asked to flush for shutdown via flushCh, at which point it sends whatever
// remains and returns — Stop is only ever called right before process exit,
// so there's no more work for it to do afterward.
func runSender() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]Event, 0, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		sendBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case e := <-eventCh:
			batch = append(batch, e)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case done := <-flushCh:
			// Drain whatever is already queued before flushing, so events
			// emitted right before shutdown aren't left behind in the channel.
		drain:
			for {
				select {
				case e := <-eventCh:
					batch = append(batch, e)
				default:
					break drain
				}
			}
			flush()
			close(done)
			return
		}
	}
}

// stopOnce ensures the flush-and-stop sequence in Stop runs exactly once,
// even if it is called from more than one shutdown path (a deferred call and
// a SIGINT/SIGTERM handler racing in) — the sender goroutine exits after its
// first flush, so a second call would find no receiver on flushCh and block
// out its full timeout for nothing.
var stopOnce sync.Once

// Stop flushes any events queued in the channel or buffered in the sender's
// in-memory batch, sending them synchronously, then stops the sender
// goroutine for good. Safe to call more than once or concurrently, and safe
// to call even if telemetry was never enabled. Bounded by short timeouts so a
// wedged network call or dead goroutine can never hang shutdown.
func Stop() {
	stopOnce.Do(func() {
		if !enabled.Load() {
			return
		}
		done := make(chan struct{})
		select {
		case flushCh <- done:
			select {
			case <-done:
			case <-time.After(6 * time.Second):
			}
		case <-time.After(1 * time.Second):
		}
	})
}

type op struct {
	Type   string `json:"type"`
	Obj    string `json:"obj"`
	ConvID int    `json:"conv_id"`
	Ref    string `json:"ref"`
	Data   Event  `json:"data"`
}

// sendBatch POSTs events to the analytics endpoint using the Corezoid ops
// API. Failures are logged only — never surfaced to the user.
func sendBatch(events []Event) {
	ops := make([]op, len(events))
	for i, e := range events {
		ops[i] = op{
			Type:   "create",
			Obj:    "task",
			ConvID: convID,
			Ref:    fmt.Sprintf("%s-%s-%d", e.InstallationID, e.Ts, i),
			Data:   e,
		}
	}
	payload := map[string]any{"ops": ops}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("telemetry: marshal error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		log.Printf("telemetry: build request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("telemetry: send error: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
}
