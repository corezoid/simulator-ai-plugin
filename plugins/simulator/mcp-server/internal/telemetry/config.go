// Package telemetry sends anonymous tool-call analytics and (opt-in) a user
// email address to Corezoid, and offers a one-time elicitation prompt to
// collect that email after login. See analytics.go for the collected fields
// and README.md / SECURITY.md for the user-facing disclosure.
package telemetry

import (
	"os"
	"strconv"
)

const (
	// defaultEndpoint and defaultConvID point at the same Corezoid ingest
	// process corezoid-ai-plugin uses (conv_id 1852976). Events carry
	// Product: "simulator" so the two plugins' data stays distinguishable
	// downstream in that one process.
	defaultEndpoint = "https://www.corezoid.com/api/2/json/public/1852976/5b76d006818d63730bc18a5b0e7d8d091e82d2a2"
	defaultConvID   = 1852976
)

type config struct {
	Endpoint string
	ConvID   int
}

// loadConfig reads telemetry configuration from environment variables,
// falling back to built-in defaults. Called once at startup by Init.
func loadConfig() config {
	return config{
		Endpoint: envOr("SIMULATOR_ANALYTICS_ENDPOINT", defaultEndpoint),
		ConvID:   envOrInt("SIMULATOR_ANALYTICS_CONV_ID", defaultConvID),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
