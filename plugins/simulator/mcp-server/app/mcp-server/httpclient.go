package mcpserver

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"
)

// apiHTTPClient returns a process-wide shared HTTP client for all outbound API
// calls. Centralising it gives us (a) a real request Timeout so a stalled
// upstream can't pin a goroutine forever, (b) connection reuse instead of a
// fresh Transport per call, and (c) a single place where TLS verification is
// controlled. Verification is ON by default; pass --insecure (globalApiConfig
// .Insecure) only for self-signed gateways.
//
// globalApiConfig.Insecure is set in LoadSwaggerServer / RunCLI before any
// operation runs, so the once-initialised client observes the right value.
var (
	httpClientOnce sync.Once
	sharedClient   *http.Client
)

func apiHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}
		if globalApiConfig.Insecure {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in via --insecure for self-signed gateways
		}
		sharedClient = &http.Client{Timeout: 60 * time.Second, Transport: tr}
	})
	return sharedClient
}
