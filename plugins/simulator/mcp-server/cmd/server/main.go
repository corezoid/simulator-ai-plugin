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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/telemetry"
	"github.com/mark3labs/mcp-go/server"
)

// version is the single source of truth in mcpserver; kept in lockstep with
// the plugin manifests by scripts/release.sh.
const version = mcpserver.DefaultVersion

// allowInsecureAPISecretEnv opts out of the refusal to send a long-lived API key
// over plaintext HTTP to a non-local host. Loopback is already exempt, so this
// exists only for trusted-network on-prem gateways with no TLS.
const allowInsecureAPISecretEnv = "SIMULATOR_ALLOW_INSECURE_API_SECRET"

// installShutdownFlush flushes buffered telemetry events before the process
// exits on SIGINT/SIGTERM (e.g. the MCP client terminating the server). Go's
// default signal handling terminates immediately without running deferred
// calls, so this is the only chance to send events queued right before
// shutdown.
func installShutdownFlush() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		telemetry.Stop()
		os.Exit(0)
	}()
}

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
	log.Printf("simulator MCP server %s — profile=%s api=%s account=%s auth=%s",
		version, info.Profile, info.APIBaseURL, info.AccountURL, info.AuthMode)
	apiKeyMode := info.AuthMode == mcpserver.AuthModeAPIKey
	if apiKeyMode {
		// Mode only — never the key, its length, its prefix or a fingerprint.
		// This line lands in the host's MCP log pane, which the user may share.
		log.Printf("auth: %s is set — OAuth login is disabled and ACCESS_TOKEN is ignored; requests use Authorization: Bearer <key>",
			mcpserver.APISecretEnv)
	}
	if mcpserver.IsInsecureCredentialTransport(info.APIBaseURL) {
		// A 12h JWT leaked on the wire is a bounded incident; an API key does not
		// expire and has no in-product revocation, so refuse rather than warn.
		if apiKeyMode && os.Getenv(allowInsecureAPISecretEnv) == "" {
			log.Fatalf("refusing to start: %s is a long-lived credential and %q would send it in cleartext to a non-local host. "+
				"Use HTTPS, or set %s=1 to override on a trusted network.",
				mcpserver.APISecretEnv, info.APIBaseURL, allowInsecureAPISecretEnv)
		}
		log.Printf("WARNING: API base URL %q uses plaintext HTTP to a non-local host — the auth token will be sent unencrypted. Use HTTPS.", info.APIBaseURL)
	}
	log.Printf("registered %d curated API tools + auth helpers + engine tools", mcpserver.ToolCount())

	// Flush any buffered telemetry events before exit. Covers both a normal
	// return (stdio EOF) via defer, and the client killing the process with
	// SIGINT/SIGTERM, which by default terminates immediately without running
	// deferred calls.
	telemetry.Init("stdio", version)
	defer telemetry.Stop()
	installShutdownFlush()

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// loadDotEnv loads KEY=VALUE lines from path into the process environment,
// without overriding values already set. Best-effort: a missing file is fine.
//
// The parser stays deliberately small, but it handles the shapes a *hand-written*
// secret arrives in — quoted values, an `export ` prefix, and a UTF-8 BOM from
// Notepad / PowerShell redirection. Machine-written keys (ACCESS_TOKEN and
// friends) never hit those, but SIMULATOR_API_SECRET is pasted by a human and
// each of them otherwise fails silently: a quoted key becomes part of the
// Authorization header, while an `export ` prefix or a BOM makes the variable
// simply not exist and drops the user back to OAuth with no explanation.
//
// Inline comments are NOT stripped: `#` is legal inside a secret, and treating
// it as a delimiter would corrupt the value.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := strings.TrimPrefix(string(data), "\uFEFF")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		val = trimMatchingQuotes(val)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

// trimMatchingQuotes removes one matching pair of surrounding single or double
// quotes. A value that is quoted on one side only is left alone — that is more
// likely to be part of the secret than a typo we should silently repair.
func trimMatchingQuotes(val string) string {
	if len(val) < 2 {
		return val
	}
	first, last := val[0], val[len(val)-1]
	if first == last && (first == '"' || first == '\'') {
		return val[1 : len(val)-1]
	}
	return val
}
