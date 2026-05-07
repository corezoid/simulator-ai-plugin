package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const (
	AuthorizeURL = "https://account.corezoid.com/oauth2/authorize"
	TokenURL     = "https://account.corezoid.com/oauth2/token"

	// DefaultClientID is the built-in OAuth2 client ID for the Simulator Claude Code plugin.
	DefaultClientID = "5ec679f5a2710f0da6000005"
)

// PKCEFlow runs the full OAuth2 PKCE authorization code flow.
// It starts a local HTTP server to receive the callback, opens the user's browser,
// waits for the authorization code, exchanges it for tokens, and returns Credentials.
// If clientID is empty, DefaultClientID is used.
func PKCEFlow(clientID string, scopes []string) (*Credentials, error) {
	if clientID == "" {
		clientID = DefaultClientID
	}
	// Generate PKCE code verifier (random 32 bytes → base64url, no padding)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate PKCE verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Compute code challenge = base64url(SHA256(verifier))
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	// Pick a random available port for the redirect server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start callback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	// Build authorization URL
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	if len(scopes) > 0 {
		params.Set("scope", strings.Join(scopes, " "))
	}
	authURL := AuthorizeURL + "?" + params.Encode()

	// Channel to receive the code (or error) from the callback handler
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		errParam := r.URL.Query().Get("error")

		if errParam != "" {
			desc := r.URL.Query().Get("error_description")
			_, _ = fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2><p>%s</p><p>You may close this tab.</p></body></html>", errParam, desc)
			errCh <- fmt.Errorf("OAuth error: %s – %s", errParam, desc)
			return
		}
		if code == "" {
			_, _ = fmt.Fprint(w, "<html><body><h2>No authorization code received.</h2><p>You may close this tab.</p></body></html>")
			errCh <- fmt.Errorf("no authorization code in callback")
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You may close this tab and return to your terminal.</p></body></html>")
		codeCh <- code
	})

	// Start callback server (non-blocking)
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("OAuth callback server error: %v", err)
		}
	}()

	// Open the browser
	log.Printf("Opening browser for Simulator authorization...\nIf it did not open automatically, visit:\n  %s\n", authURL)
	_ = openBrowser(authURL)

	// Wait for authorization code (timeout: 5 minutes)
	var code string
	select {
	case code = <-codeCh:
	case oauthErr := <-errCh:
		_ = srv.Shutdown(context.Background())
		return nil, oauthErr
	case <-time.After(5 * time.Minute):
		_ = srv.Shutdown(context.Background())
		return nil, fmt.Errorf("timed out waiting for OAuth callback (5 minutes)")
	}
	_ = srv.Shutdown(context.Background())

	// Exchange code for tokens
	return exchangeCode(clientID, code, codeVerifier, redirectURI)
}

// exchangeCode exchanges an authorization code for access and refresh tokens.
func exchangeCode(clientID, code, codeVerifier, redirectURI string) (*Credentials, error) {
	if clientID == "" {
		clientID = DefaultClientID
	}
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("code_verifier", codeVerifier)
	data.Set("redirect_uri", redirectURI)

	return postTokenRequest(data)
}

// tokenResponse is the raw JSON response from the Simulator token endpoint.
type tokenResponse struct {
	SimulatorToken string `json:"simulator_token"` // JWT — the actual MCP auth token
	Error          string `json:"error"`
	ErrorDesc      string `json:"error_description"`
}

func postTokenRequest(data url.Values) (*Credentials, error) {
	resp, err := http.PostForm(TokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tr.Error != "" {
		return nil, fmt.Errorf("token error: %s – %s", tr.Error, tr.ErrorDesc)
	}
	if tr.SimulatorToken == "" {
		return nil, fmt.Errorf("no simulator_token in response: %s", string(body))
	}

	creds := &Credentials{
		AccessToken: tr.SimulatorToken,
		TokenType:   "Simulator",
		ExpiresAt:   jwtExpiry(tr.SimulatorToken),
	}
	return creds, nil
}

// jwtExpiry extracts the exp claim from a JWT without verifying the signature.
// Returns zero time if parsing fails (treated as "no expiry").
func jwtExpiry(token string) time.Time {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

// openBrowser opens the given URL in the default system browser (macOS / Linux).
func openBrowser(u string) error {
	// macOS
	if err := exec.Command("open", u).Start(); err == nil {
		return nil
	}
	// Linux
	if err := exec.Command("xdg-open", u).Start(); err == nil {
		return nil
	}
	return fmt.Errorf("could not open browser automatically")
}
