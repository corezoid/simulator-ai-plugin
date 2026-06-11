// Command server is the simulator MCP server: it exposes a curated set of
// Simulator.Company (pong-server) public-API operations as MCP tools.
//
// Environment is selected by --profile (or SIMULATOR_PROFILE), default "prod".
// Credentials live in .env in the working directory (written by the login tool).
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
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

	s, info, err := mcpserver.New(mcpserver.Options{
		Profile:  *profileFlag,
		Insecure: *insecure,
		Version:  version,
	})
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("simulator MCP server %s — profile=%s api=%s account=%s", version, info.Profile, info.APIBaseURL, info.AccountURL)
	if mcpserver.IsInsecureCredentialTransport(info.APIBaseURL) {
		log.Printf("WARNING: API base URL %q uses plaintext HTTP to a non-local host — the auth token will be sent unencrypted. Use HTTPS.", info.APIBaseURL)
	}
	log.Printf("registered %d curated API tools + auth helpers + engine tools", mcpserver.ToolCount())

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
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
