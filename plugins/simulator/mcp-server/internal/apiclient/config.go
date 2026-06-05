package apiclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchPublicConfig calls the platform's public, unauthenticated config endpoint
// (getConfigReq, served at {apiBaseURL}/config) and returns its saUrl — the OAuth2
// account (SA) base URL to authenticate against for that environment.
//
// pong-server authenticates through the account system, and one account may back
// several sim environments, so the correct auth URL is not fixed per gateway: it is
// whatever the chosen gateway reports as saUrl. This call also doubles as the
// reachability / validity check for a freshly chosen environment.
//
// insecure skips TLS verification — only for self-signed on-prem gateways.
func FetchPublicConfig(ctx context.Context, apiBaseURL string, insecure bool) (string, error) {
	configURL := strings.TrimRight(apiBaseURL, "/") + "/config"

	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — opt-in via --insecure
	}
	httpClient := &http.Client{Timeout: 15 * time.Second, Transport: tr}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, configURL, nil)
	if err != nil {
		return "", fmt.Errorf("build config request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to %s failed: %w", configURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read config response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("config endpoint %s returned %d: %s", configURL, resp.StatusCode, string(body))
	}

	var parsed struct {
		Data struct {
			SaURL string `json:"saUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse config response from %s: %w", configURL, err)
	}
	if parsed.Data.SaURL == "" {
		return "", fmt.Errorf("config endpoint %s returned no saUrl — is this a Simulator API base URL?", configURL)
	}
	return strings.TrimRight(parsed.Data.SaURL, "/"), nil
}
