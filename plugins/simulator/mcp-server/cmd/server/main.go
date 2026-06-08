// Command server is the simulator MCP server: it exposes a curated set of
// Simulator.Company (pong-server) public-API operations as MCP tools.
//
// Environment is selected by --profile (or SIMULATOR_PROFILE), default "prod".
// Credentials live in .env in the working directory (written by the login tool).
package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

const version = "2.1.0"

func main() {
	profileFlag := flag.String("profile", "", "Environment profile: local | prod (default: SIMULATOR_PROFILE or prod)")
	insecure := flag.Bool("insecure", false, "Skip TLS verification (self-signed on-prem gateways only)")
	flag.Parse()

	if workDir := os.Getenv("SIMULATOR_WORK_DIR"); workDir != "" {
		loadDotEnv(filepath.Join(workDir, ".env"))
	} else {
		loadDotEnv(".env")
	}

	prof, err := config.Resolve(*profileFlag)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("simulator MCP server %s — profile=%s api=%s account=%s", version, prof.Name, prof.APIBaseURL, prof.AccountURL)
	warnIfInsecureCredentialTransport(prof.APIBaseURL)

	// AuthHeader provider reads the current credentials each call so a fresh
	// login takes effect without a restart.
	authHeader := func() (string, error) {
		creds, err := auth.Load()
		if err != nil {
			return "", err
		}
		if creds == nil {
			return "", errors.New("not authenticated — run the `login` tool first")
		}
		return creds.AuthorizationHeader(), nil
	}

	client := apiclient.New(prof.APIBaseURL, os.Getenv("WORKSPACE_ID"), authHeader, *insecure)

	s := server.NewMCPServer("simulator", version)
	tools.BuildAll(s, client, prof, *insecure)
	engines.Configure(prof.APIBaseURL, *insecure)
	engines.RegisterTools(s)
	log.Printf("registered %d curated API tools + auth helpers + engine tools", tools.Count())

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// warnIfInsecureCredentialTransport logs a warning when the resolved API base
// URL would send the bearer token over plaintext HTTP to a non-loopback host.
// The token is attached to every request, so a remote http:// endpoint (e.g.
// set via SIMULATOR_API_BASE_URL or profiles.json) would expose it on the wire.
func warnIfInsecureCredentialTransport(baseURL string) {
	if apiclient.IsInsecureCredentialTransport(baseURL) {
		log.Printf("WARNING: API base URL %q uses plaintext HTTP to a non-local host — the auth token will be sent unencrypted. Use HTTPS.", baseURL)
	}
}

// loadDotEnv loads KEY=VALUE lines from path into the process environment,
// without overriding values already set. Best-effort: a missing file is fine.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
