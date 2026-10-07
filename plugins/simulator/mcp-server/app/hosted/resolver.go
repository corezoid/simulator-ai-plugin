package hosted

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// apiURLResolver maps a workspace id to the simulator API base URL by calling
// the Account service. Two steps:
//
//  1. GET {AccountURL}/face/api/1/workspaces/{workspaceID}
//     → settings.sim_client
//  2. GET {AccountURL}/face/api/1/clients
//     → entry with client_id == sim_client
//     → strip redirect_uri to "scheme://host" and append "/papi/1.0"
//
// Both responses are cached with a short TTL so the hot path is one map lookup.
// On any failure or missing data the resolver returns "" — the caller falls
// back to Config.FallbackURL.
//
// Two properties are security boundaries, not optimisations:
//
// Caches are per caller. Every entry is keyed by a hash of the caller's
// Authorization value. A shared cache let one caller's lookup answer another's:
// whoever resolved a workspace first decided, for five minutes, where every
// other caller's token was sent for that workspace — without the later caller
// ever proving to the Account service that it may read that workspace. Keying
// by caller means a cache hit only ever replays an answer the Account service
// already gave to the same credential. Only the hash is kept; the token itself
// never sits in the cache.
//
// The resolved origin must be allowed. redirect_uri belongs to an OAuth client,
// and clients are created through public dynamic registration — so its host is
// whatever the client's registrant typed. The caller's token is forwarded to
// that host on every tool call, so a host outside allowedOrigins is refused and
// the request falls back to Config.FallbackURL instead.
type apiURLResolver struct {
	accountURL     string
	httpClient     *http.Client
	allowedOrigins []string

	mu             sync.Mutex
	workspaceCache map[string]workspaceCacheEntry
	clientsCache   map[string]clientsCacheEntry
}

type workspaceCacheEntry struct {
	baseURL   string
	expiresAt time.Time
}

type clientsCacheEntry struct {
	clients   []accountClient
	expiresAt time.Time
}

type accountClient struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
}

type accountWorkspace struct {
	Settings struct {
		SimClient string `json:"sim_client"`
	} `json:"settings"`
}

const (
	workspaceCacheTTL = 5 * time.Minute
	clientsCacheTTL   = 15 * time.Minute
	simAPIPathSuffix  = "/papi/1.0"

	// maxCacheEntries bounds each cache. Keys now carry a caller hash, so the
	// key space grows with the number of distinct tokens seen, not just with
	// the number of workspaces.
	maxCacheEntries = 10000
)

// newAPIURLResolver returns a resolver targeting the given Account service URL.
// accountURL must not be empty; the caller checks before constructing.
// allowedOrigins are origin patterns ("https://host" or "https://*.parent");
// an empty list refuses every resolved origin, so all scoped requests use the
// fallback.
func newAPIURLResolver(accountURL string, insecure bool, allowedOrigins []string) *apiURLResolver {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &apiURLResolver{
		accountURL: strings.TrimRight(accountURL, "/"),
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		},
		allowedOrigins: allowedOrigins,
		workspaceCache: map[string]workspaceCacheEntry{},
		clientsCache:   map[string]clientsCacheEntry{},
	}
}

// resolve returns the simulator API base URL for workspaceID, or "" if the
// workspace has no sim_client or the client_id has no matching redirect_uri.
// auth is the already-normalised Simulator authorization header.
func (r *apiURLResolver) resolve(ctx context.Context, workspaceID, auth string) (string, error) {
	if workspaceID == "" || auth == "" {
		return "", nil
	}
	caller := callerKey(auth)
	wsKey := caller + "|" + workspaceID

	if cached, ok := r.cachedWorkspace(wsKey); ok {
		return cached, nil
	}

	simClient, err := r.fetchSimClient(ctx, workspaceID, auth)
	if err != nil {
		return "", err
	}
	if simClient == "" {
		r.storeWorkspace(wsKey, "")
		return "", nil
	}

	redirectURI, err := r.fetchRedirectURI(ctx, caller, auth, simClient)
	if err != nil {
		return "", err
	}
	if redirectURI == "" {
		r.storeWorkspace(wsKey, "")
		return "", nil
	}

	// Refusals are cached as "no mapping" so a refused workspace does not cost
	// two Account round trips on every request.
	base, err := baseFromRedirect(redirectURI)
	if err != nil {
		r.storeWorkspace(wsKey, "")
		return "", err
	}
	if !originAllowed(base, r.allowedOrigins) {
		r.storeWorkspace(wsKey, "")
		return "", fmt.Errorf("resolved origin %s is not in the allowed simulator origins", base)
	}
	final := base + simAPIPathSuffix
	r.storeWorkspace(wsKey, final)
	return final, nil
}

// callerKey identifies the caller for cache keying without retaining the
// credential itself.
func callerKey(auth string) string {
	sum := sha256.Sum256([]byte(auth))
	return hex.EncodeToString(sum[:])
}

func (r *apiURLResolver) cachedWorkspace(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.workspaceCache[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.baseURL, true
}

func (r *apiURLResolver) storeWorkspace(key, baseURL string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.workspaceCache) >= maxCacheEntries {
		now := time.Now()
		for k, e := range r.workspaceCache {
			if now.After(e.expiresAt) {
				delete(r.workspaceCache, k)
			}
		}
		if len(r.workspaceCache) >= maxCacheEntries {
			r.workspaceCache = map[string]workspaceCacheEntry{}
		}
	}
	r.workspaceCache[key] = workspaceCacheEntry{
		baseURL:   baseURL,
		expiresAt: time.Now().Add(workspaceCacheTTL),
	}
}

func (r *apiURLResolver) fetchSimClient(ctx context.Context, workspaceID, auth string) (string, error) {
	endpoint := fmt.Sprintf("%s/face/api/1/workspaces/%s", r.accountURL, url.PathEscape(workspaceID))
	var ws accountWorkspace
	if err := r.getJSON(ctx, endpoint, auth, &ws); err != nil {
		return "", fmt.Errorf("account: workspace %s: %w", workspaceID, err)
	}
	return strings.TrimSpace(ws.Settings.SimClient), nil
}

func (r *apiURLResolver) fetchRedirectURI(ctx context.Context, caller, auth, simClient string) (string, error) {
	clients, err := r.getClients(ctx, caller, auth)
	if err != nil {
		return "", err
	}
	for _, c := range clients {
		if c.ClientID == simClient {
			return strings.TrimSpace(c.RedirectURI), nil
		}
	}
	return "", nil
}

func (r *apiURLResolver) getClients(ctx context.Context, caller, auth string) ([]accountClient, error) {
	r.mu.Lock()
	if e, ok := r.clientsCache[caller]; ok && time.Now().Before(e.expiresAt) {
		r.mu.Unlock()
		return e.clients, nil
	}
	r.mu.Unlock()

	endpoint := r.accountURL + "/face/api/1/clients"
	var clients []accountClient
	if err := r.getJSON(ctx, endpoint, auth, &clients); err != nil {
		return nil, fmt.Errorf("account: clients: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.clientsCache) >= maxCacheEntries {
		now := time.Now()
		for k, e := range r.clientsCache {
			if now.After(e.expiresAt) {
				delete(r.clientsCache, k)
			}
		}
		if len(r.clientsCache) >= maxCacheEntries {
			r.clientsCache = map[string]clientsCacheEntry{}
		}
	}
	r.clientsCache[caller] = clientsCacheEntry{
		clients:   clients,
		expiresAt: time.Now().Add(clientsCacheTTL),
	}
	return clients, nil
}

func (r *apiURLResolver) getJSON(ctx context.Context, endpoint, auth string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", auth)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// baseFromRedirect extracts "scheme://host" from a redirect_uri. A redirect_uri
// carrying userinfo is refused: "https://mw.simulator.company@evil.com" reads
// as a trusted host to a person but its host is evil.com.
func baseFromRedirect(redirectURI string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("parse redirect_uri %q: %w", redirectURI, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("redirect_uri missing scheme or host: " + redirectURI)
	}
	if u.User != nil {
		return "", errors.New("redirect_uri carries userinfo: " + redirectURI)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

// resolverAllowlist assembles the origins a resolved simulator API URL may
// point at: the host of the fallback Simulator URL, its sibling subdomains,
// and any operator-supplied extras (Config.ResolverAllowedOrigins) for on-prem
// Simulators living on their own domains:
//
//	FallbackURL = https://mw.simulator.company
//	  -> https://mw.simulator.company   (exact)
//	  -> https://*.simulator.company    (siblings)
func resolverAllowlist(simulatorURL string, extra []string) []string {
	var allowed []string
	if u, err := url.Parse(strings.TrimSpace(simulatorURL)); err == nil && u.Scheme == "https" && u.Host != "" {
		origin := "https://" + strings.ToLower(u.Host)
		allowed = append(allowed, origin)
		if wildcard := siblingWildcard(u); wildcard != "" {
			allowed = append(allowed, wildcard)
		}
	}
	for _, e := range extra {
		if e = strings.TrimSpace(e); e != "" {
			allowed = append(allowed, strings.TrimRight(e, "/"))
		}
	}
	return allowed
}

// siblingWildcard turns https://mw.simulator.company into
// https://*.simulator.company, or "" when the host has no parent worth
// wildcarding (a bare domain, an IP address, a host carrying a port). The
// parent is only wildcarded when it still has two labels, so a bare
// simulator.company never yields *.company.
func siblingWildcard(u *url.URL) string {
	if u.Host != u.Hostname() {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) != nil {
		return ""
	}
	_, parent, found := strings.Cut(host, ".")
	if !found || strings.Count(parent, ".") < 1 {
		return ""
	}
	return "https://*." + parent
}

// originAllowed reports whether origin ("scheme://host[:port]") matches one of
// the patterns. Matching is structural, never by string prefix: a prefix test
// would accept https://simulator.company.evil.com for https://simulator.company.
// Only https is accepted — the caller's token travels to this origin. A
// "*." pattern matches subdomains at a label boundary, never the bare parent.
func originAllowed(origin string, patterns []string) bool {
	target, err := url.Parse(origin)
	if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil {
		return false
	}
	host := strings.ToLower(target.Host)
	for _, raw := range patterns {
		pattern, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || pattern.Scheme != "https" || pattern.Host == "" {
			continue
		}
		pHost := strings.ToLower(pattern.Host)
		if suffix, ok := strings.CutPrefix(pHost, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == pHost {
			return true
		}
	}
	return false
}
