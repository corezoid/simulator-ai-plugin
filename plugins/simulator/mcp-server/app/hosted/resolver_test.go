package hosted

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeAccount is an Account service stand-in. workspaces maps
// "<auth>|<workspaceID>" to the sim_client that caller is allowed to see (a
// missing key answers 403, as the real service does for a non-member);
// clients maps auth to the clients list that caller sees.
type fakeAccount struct {
	workspaces map[string]string
	clients    map[string][]accountClient
	calls      atomic.Int64
}

func (f *fakeAccount) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		auth := r.Header.Get("Authorization")
		switch {
		case strings.HasPrefix(r.URL.Path, "/face/api/1/workspaces/"):
			wsID := strings.TrimPrefix(r.URL.Path, "/face/api/1/workspaces/")
			simClient, ok := f.workspaces[auth+"|"+wsID]
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"settings": map[string]string{"sim_client": simClient}})
		case r.URL.Path == "/face/api/1/clients":
			_ = json.NewEncoder(w).Encode(f.clients[auth])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var testAllowlist = []string{"https://mw.simulator.company", "https://*.simulator.company"}

// A cache hit must never replay an answer the Account service gave to a
// different caller. Before per-caller keys, attacker resolved their workspace
// first and the victim — who cannot read that workspace — was served the
// cached origin anyway, so the victim's token went wherever the attacker's
// workspace pointed.
func TestResolveCacheIsPerCaller(t *testing.T) {
	const attacker, victim = "Simulator attacker", "Simulator victim"
	acct := &fakeAccount{
		workspaces: map[string]string{attacker + "|ws-1": "client-a"},
		clients: map[string][]accountClient{
			attacker: {{ClientID: "client-a", RedirectURI: "https://sim-a.simulator.company/cb"}},
		},
	}
	r := newAPIURLResolver(acct.server(t).URL, false, testAllowlist)

	got, err := r.resolve(context.Background(), "ws-1", attacker)
	if err != nil || got != "https://sim-a.simulator.company/papi/1.0" {
		t.Fatalf("attacker resolve = %q, %v", got, err)
	}

	got, err = r.resolve(context.Background(), "ws-1", victim)
	if got != "" {
		t.Fatalf("victim was served the attacker's cached origin %q", got)
	}
	if err == nil {
		t.Fatal("victim resolve: want the Account 403 surfaced, got nil error")
	}
}

// The clients list is per caller too: one caller's list must not be used to
// look up another caller's sim_client.
func TestClientsCacheIsPerCaller(t *testing.T) {
	const a, b = "Simulator a", "Simulator b"
	acct := &fakeAccount{
		workspaces: map[string]string{a + "|ws-a": "shared-id", b + "|ws-b": "shared-id"},
		clients: map[string][]accountClient{
			a: {{ClientID: "shared-id", RedirectURI: "https://one.simulator.company/cb"}},
			b: {{ClientID: "shared-id", RedirectURI: "https://two.simulator.company/cb"}},
		},
	}
	r := newAPIURLResolver(acct.server(t).URL, false, testAllowlist)

	if got, _ := r.resolve(context.Background(), "ws-a", a); got != "https://one.simulator.company/papi/1.0" {
		t.Fatalf("caller a = %q", got)
	}
	if got, _ := r.resolve(context.Background(), "ws-b", b); got != "https://two.simulator.company/papi/1.0" {
		t.Fatalf("caller b = %q (served caller a's clients list?)", got)
	}
}

// redirect_uri is registrant-controlled (public dynamic client registration),
// so an origin outside the allowlist is refused rather than handed the
// caller's token. The refusal is cached so it does not cost Account round
// trips on every request.
func TestResolveRefusesOriginOutsideAllowlist(t *testing.T) {
	const caller = "Simulator me"
	for name, redirect := range map[string]string{
		"foreign host":       "https://evil.example/cb",
		"suffix trick":       "https://mw.simulator.company.evil.example/cb",
		"plaintext":          "http://mw.simulator.company/cb",
		"userinfo disguise":  "https://mw.simulator.company@evil.example/cb",
		"bare parent domain": "https://simulator.company/cb",
	} {
		t.Run(name, func(t *testing.T) {
			acct := &fakeAccount{
				workspaces: map[string]string{caller + "|ws": "c"},
				clients:    map[string][]accountClient{caller: {{ClientID: "c", RedirectURI: redirect}}},
			}
			r := newAPIURLResolver(acct.server(t).URL, false, testAllowlist)

			got, err := r.resolve(context.Background(), "ws", caller)
			if got != "" || err == nil {
				t.Fatalf("resolve = %q, %v; want refusal", got, err)
			}
			before := acct.calls.Load()
			if got, _ := r.resolve(context.Background(), "ws", caller); got != "" {
				t.Fatalf("second resolve = %q", got)
			}
			if after := acct.calls.Load(); after != before {
				t.Errorf("refusal not cached: %d extra Account calls", after-before)
			}
		})
	}
}

func TestResolveAllowedOriginIsCached(t *testing.T) {
	const caller = "Simulator me"
	acct := &fakeAccount{
		workspaces: map[string]string{caller + "|ws": "c"},
		clients:    map[string][]accountClient{caller: {{ClientID: "c", RedirectURI: "https://MW.Simulator.Company/cb"}}},
	}
	r := newAPIURLResolver(acct.server(t).URL, false, testAllowlist)

	for i := 0; i < 3; i++ {
		got, err := r.resolve(context.Background(), "ws", caller)
		if err != nil || got != "https://mw.simulator.company/papi/1.0" {
			t.Fatalf("resolve #%d = %q, %v", i, got, err)
		}
	}
	if n := acct.calls.Load(); n != 2 {
		t.Errorf("Account calls = %d, want 2 (workspace + clients, then cache)", n)
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://mw.simulator.company", true},
		{"https://MW.SIMULATOR.COMPANY", true},
		{"https://sim-dev.simulator.company", true},
		{"https://a.b.simulator.company", true},
		{"https://simulator.company", false},
		{"https://simulator.company.evil.com", false},
		{"https://evilsimulator.company", false},
		{"http://mw.simulator.company", false},
		{"https://mw.simulator.company:8443", false},
		{"https://user@mw.simulator.company", false},
		{"", false},
		{"mw.simulator.company", false},
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, testAllowlist); got != c.want {
			t.Errorf("originAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
	if originAllowed("https://mw.simulator.company", nil) {
		t.Error("empty allowlist must refuse everything")
	}
}

func TestResolverAllowlist(t *testing.T) {
	cases := []struct {
		name, simURL string
		extra        []string
		want         []string
	}{
		{"prod", "https://mw.simulator.company", nil,
			[]string{"https://mw.simulator.company", "https://*.simulator.company"}},
		{"with papi path", "https://mw.simulator.company/papi/1.0", nil,
			[]string{"https://mw.simulator.company", "https://*.simulator.company"}},
		{"bare domain gets no wildcard", "https://simulator.company", nil,
			[]string{"https://simulator.company"}},
		{"port gets no wildcard", "https://sim.simulator.company:8443", nil,
			[]string{"https://sim.simulator.company:8443"}},
		{"plaintext contributes nothing", "http://mw.simulator.company", nil, nil},
		{"extras appended", "https://mw.simulator.company", []string{" https://sim.customer.com/ ", ""},
			[]string{"https://mw.simulator.company", "https://*.simulator.company", "https://sim.customer.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolverAllowlist(c.simURL, c.extra)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestCallerKeyDoesNotRetainToken(t *testing.T) {
	const token = "Simulator super-secret-jwt"
	k := callerKey(token)
	if strings.Contains(k, "super-secret") || len(k) != 64 {
		t.Errorf("callerKey = %q", k)
	}
	if callerKey(token) != k || callerKey(token+"x") == k {
		t.Error("callerKey must be deterministic and distinguish tokens")
	}
}
