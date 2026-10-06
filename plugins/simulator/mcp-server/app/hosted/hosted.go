// Package hosted serves a stateless simulator MCP server over streamable HTTP
// for remote (hosted) deployments such as mcp.simulator.company.
//
// The route set, OAuth discovery and Authorization handling match the hosted
// server that previously lived inside claude-code-api, so existing clients keep
// working when the public host moves to this package:
//
//	/mcp                                              no scope
//	/mcp/workspaces/{workspace_id}                    workspace scope
//	/mcp/workspaces/{workspace_id}/actors/{actor_id}  workspace + actor scope
//	/.well-known/oauth-protected-resource[/...]       RFC 9728 metadata
//	/healthz                                          load-balancer probe
//
// Every request carries the caller's own token. Nothing is stored server-side:
// no sessions, no credentials, no .env. The server passed in must be built with
// mcpserver.New(Options{Stateless: true}), which also leaves out the
// .env-mutating helper tools (login / set-workspace / set-environment).
package hosted

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/mark3labs/mcp-go/server"
)

// Config configures the hosted HTTP handler.
type Config struct {
	// EndpointPath is the MCP endpoint; defaults to "/mcp".
	EndpointPath string

	// ResourceURL is this server's canonical public URL (e.g.
	// "https://mcp.simulator.company"), advertised as the RFC 9728 protected
	// resource. Together with AuthServerURL it enables OAuth discovery:
	// anonymous requests get a 401 pointing at the metadata document. Leaving
	// either empty disables discovery, but requests without Authorization are
	// still refused with a plain 401 — the hosted server never serves anonymous
	// tool calls.
	ResourceURL string

	// AuthServerURL is the OAuth authorization server advertised in the
	// metadata (e.g. "https://account.corezoid.com").
	AuthServerURL string

	// AccountURL enables per-workspace API URL resolution on the
	// /mcp/workspaces/... routes (workspace → sim_client → redirect_uri).
	// Empty disables it: every request uses FallbackURL.
	AccountURL string

	// FallbackURL is the Simulator API base used when no per-workspace URL was
	// resolved (e.g. "https://mw.simulator.company"); "/papi/1.0" is appended
	// when missing. Empty leaves the choice to the mcpserver profile. Its host
	// and sibling subdomains form the default resolver allowlist.
	FallbackURL string

	// ResolverAllowedOrigins extends the resolver allowlist with on-prem
	// Simulators on their own domains ("https://sim.customer.com",
	// "https://*.customer.com").
	ResolverAllowedOrigins []string

	// Insecure skips TLS verification on Account lookups (self-signed on-prem
	// gateways only).
	Insecure bool

	// OpenAIAppsChallenge, when set, is served verbatim as text/plain at
	// /.well-known/openai-apps-challenge — the domain-verification token the
	// OpenAI plugin directory asks the MCP host to publish. It is public by
	// design; an unset value leaves the path a 404.
	OpenAIAppsChallenge string
}

const (
	headerControlEventsContext = "control-events-context"
	healthzPath                = "/healthz"
	openAIChallengePath        = "/.well-known/openai-apps-challenge"
	// heartbeatInterval keeps load-balancer idle timeouts from cutting
	// long-running tool calls that stream their response.
	heartbeatInterval = 30 * time.Second
)

// NewHandler wraps s (built with Stateless: true) into the full hosted HTTP
// handler described in the package comment.
func NewHandler(s *server.MCPServer, cfg Config) http.Handler {
	addToolTitles(s)
	endpoint := strings.TrimRight(cfg.EndpointPath, "/")
	if endpoint == "" {
		endpoint = "/mcp"
	}

	// Stateless: no Mcp-Session-Id is issued or required, so any replica can
	// serve any request.
	streamSrv := server.NewStreamableHTTPServer(s,
		server.WithEndpointPath(endpoint),
		server.WithHTTPContextFunc(httpContextFunc),
		server.WithStateLess(true),
		server.WithHeartbeatInterval(heartbeatInterval),
	)

	fallbackURL := normalizeSimulatorAPIURL(cfg.FallbackURL)
	var resolver *apiURLResolver
	if cfg.AccountURL != "" {
		allowed := resolverAllowlist(cfg.FallbackURL, cfg.ResolverAllowedOrigins)
		resolver = newAPIURLResolver(cfg.AccountURL, cfg.Insecure, allowed)
	}
	oauth := newOAuthConfig(cfg.ResourceURL, cfg.AuthServerURL)

	mux := http.NewServeMux()
	if oauth.enabled() {
		mux.Handle(oauth.metaPath, oauth.handler)
	}
	mux.Handle(endpoint, requireAuth(defaultBaseURLHandler(streamSrv, fallbackURL), oauth))
	scoped := scopedHandler(streamSrv, endpoint, resolver, fallbackURL)
	mux.Handle(endpoint+"/workspaces/{workspace_id}", requireAuth(scoped, oauth))
	mux.Handle(endpoint+"/workspaces/{workspace_id}/actors/{actor_id}", requireAuth(scoped, oauth))
	if tok := strings.TrimSpace(cfg.OpenAIAppsChallenge); tok != "" {
		mux.HandleFunc(openAIChallengePath, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(tok))
		})
	}
	mux.HandleFunc(healthzPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// httpContextFunc forwards the caller's own token and UI context onto ctx for
// every tool call. Workspace, actor and API base URL come from the URL path
// (scopedHandler), never from client headers: a header-chosen base URL would
// let a client point another caller's credentials at an arbitrary host.
func httpContextFunc(ctx context.Context, r *http.Request) context.Context {
	if auth := normalizeAuthorization(r.Header.Get("Authorization")); auth != "" {
		ctx = mcpserver.WithAuthorization(ctx, auth)
	}
	if uiCtx := r.Header.Get(headerControlEventsContext); uiCtx != "" {
		ctx = mcpserver.WithUIContext(ctx, uiCtx)
	}
	return ctx
}

// requireAuth refuses requests without a usable Authorization header before
// they reach the MCP stack: with the RFC 9728 challenge when discovery is
// configured, with a plain 401 otherwise.
func requireAuth(inner http.Handler, oauth oauthConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if normalizeAuthorization(r.Header.Get("Authorization")) == "" {
			if oauth.enabled() {
				oauth.challenge(w)
				return
			}
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// defaultBaseURLHandler pins the bare endpoint to the fallback Simulator URL.
func defaultBaseURLHandler(next http.Handler, fallbackURL string) http.Handler {
	if fallbackURL == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.Clone(mcpserver.WithBaseURL(r.Context(), fallbackURL)))
	})
}

// scopedHandler serves /mcp/workspaces/{workspace_id}[/actors/{actor_id}]:
// workspace and actor come from the path, the API base URL from the resolver
// (per caller, allowlisted) or the fallback. The path is rewritten to the
// canonical endpoint, which the streamable transport matches exactly.
func scopedHandler(next http.Handler, endpoint string, resolver *apiURLResolver, fallbackURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsID := strings.TrimSpace(r.PathValue("workspace_id"))
		if wsID == "" {
			http.NotFound(w, r)
			return
		}
		ctx := mcpserver.WithWorkspaceID(r.Context(), wsID)
		if actorID := strings.TrimSpace(r.PathValue("actor_id")); actorID != "" {
			ctx = mcpserver.WithActorID(ctx, actorID)
		}

		baseURL := ""
		if resolver != nil {
			// A failed or refused resolution is not an error for the caller: the
			// request proceeds against the fallback, as for "no mapping". The log
			// line is how operators find on-prem workspaces missing from
			// ResolverAllowedOrigins.
			var err error
			baseURL, err = resolver.resolve(r.Context(), wsID, normalizeAuthorization(r.Header.Get("Authorization")))
			if err != nil {
				log.Printf("hosted: workspace %s: API URL resolution failed, using fallback: %v", wsID, err)
			}
		}
		if baseURL == "" {
			baseURL = fallbackURL
		}
		if baseURL != "" {
			ctx = mcpserver.WithBaseURL(ctx, baseURL)
		}

		r2 := r.Clone(ctx)
		r2.URL.Path = endpoint
		r2.URL.RawPath = ""
		next.ServeHTTP(w, r2)
	})
}

// normalizeAuthorization converts the incoming Authorization header into the
// "Simulator <token>" form the Simulator API expects. MCP clients send
// "Bearer <token>"; some send the bare token or "Simulator <token>".
//
// A scheme with no token ("Bearer", "Bearer ") is no credential at all. HTTP
// stacks trim the trailing space, so without the scheme check a bare "Bearer"
// would be taken for a token and the request would pass requireAuth.
func normalizeAuthorization(raw string) string {
	fields := strings.Fields(raw)
	switch {
	case len(fields) == 0:
		return ""
	case len(fields) == 1:
		if isAuthScheme(fields[0]) {
			return ""
		}
		return "Simulator " + fields[0]
	case isAuthScheme(fields[0]):
		return "Simulator " + strings.Join(fields[1:], " ")
	default:
		return "Simulator " + strings.Join(fields, " ")
	}
}

func isAuthScheme(s string) bool {
	return strings.EqualFold(s, "Bearer") || strings.EqualFold(s, "Simulator")
}

// normalizeSimulatorAPIURL ensures the base URL carries a /papi/<version>
// segment, since the API client appends tool paths directly to it.
func normalizeSimulatorAPIURL(raw string) string {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if !strings.Contains(s, "/papi/") {
		s += "/papi/1.0"
	}
	return s
}

// oauthConfig is the RFC 9728 protected-resource side of OAuth discovery.
// Token issuance, DCR and PKCE live on the authorization server; this server
// never proxies the flow and never stores tokens.
type oauthConfig struct {
	metaPath string
	metaURL  string
	handler  http.Handler
}

func newOAuthConfig(resourceURL, authServerURL string) oauthConfig {
	if resourceURL == "" || authServerURL == "" {
		return oauthConfig{}
	}
	o := oauthConfig{metaPath: server.ProtectedResourceMetadataPath(resourceURL)}
	o.metaURL = originOf(resourceURL) + o.metaPath
	o.handler = server.NewProtectedResourceMetadataHandler(server.ProtectedResourceMetadataConfig{
		Resource:             resourceURL,
		AuthorizationServers: []string{authServerURL},
	})
	return o
}

func (o oauthConfig) enabled() bool { return o.handler != nil }

func (o oauthConfig) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+o.metaURL+`"`)
	http.Error(w, "authorization required", http.StatusUnauthorized)
}

func originOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.IsAbs() && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return rawURL
}

// addToolTitles gives every tool without one a human-readable title derived
// from its name ("getWorkspaces" -> "Get workspaces"). Connector directories
// require a title per tool; clients show it instead of the raw name.
func addToolTitles(s *server.MCPServer) {
	for name, st := range s.ListTools() {
		if st.Tool.Title != "" || st.Tool.Annotations.Title != "" {
			continue
		}
		t := st.Tool
		t.Title = humanizeToolName(name)
		t.Annotations.Title = t.Title
		s.AddTool(t, st.Handler)
	}
}

// humanizeToolName splits camelCase and kebab/snake case into words and
// capitalises the first: "uploadActorPictureBulk" -> "Upload actor picture bulk".
func humanizeToolName(name string) string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '-' || r == '_' || r == ' ':
			flush()
		case unicode.IsUpper(r) && len(cur) > 0 && (unicode.IsLower(cur[len(cur)-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	if len(words) == 0 {
		return name
	}
	first := []rune(words[0])
	first[0] = unicode.ToUpper(first[0])
	words[0] = string(first)
	return strings.Join(words, " ")
}
