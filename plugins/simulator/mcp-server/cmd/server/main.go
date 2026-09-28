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

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/telemetry"
	"github.com/mark3labs/mcp-go/server"
)

// version is the single source of truth in mcpserver; kept in lockstep with
// the plugin manifests by scripts/release.sh.
const version = mcpserver.DefaultVersion

const apiBaseURLEnv = "SIMULATOR_API_BASE_URL"

// insecureAPISecretAllowed reports whether the operator really asked to send a
// long-lived API key over plaintext HTTP.
//
// This is parsed as a boolean rather than the repo's usual "set to anything"
// convention (SIMULATOR_ANALYTICS_DISABLED): the natural way to turn a switch off
// is `=0` or `=false`, and under a non-empty test that DISABLES the guard — the
// exact opposite of the intent, for the one credential that never expires and has
// no in-product revocation. Anything unparseable is treated as "not allowed", so
// the failure mode is a refusal to start with an explanatory message.
// envSource says where a variable's value came from, for the startup log.
func envSource(key string, fromDotEnv map[string]bool) string {
	switch {
	case fromDotEnv[key]:
		return ".env"
	case os.Getenv(key) != "":
		return "the process environment"
	default:
		return "the profile default"
	}
}

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

	dotEnvPath := ".env"
	if workDir := os.Getenv("SIMULATOR_WORK_DIR"); workDir != "" {
		dotEnvPath = filepath.Join(workDir, ".env")
	}
	fromDotEnv := loadDotEnv(dotEnvPath)

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
		// Naming the SOURCE of each half matters: loadDotEnv never overrides a
		// value already in the environment, so a key exported in the developer's
		// shell silently outranks the project's .env while the base URL still comes
		// from that .env — the key then travels to a gateway it was not issued for,
		// and nothing in the log used to say so. Sources only; never the value.
		log.Printf("auth: %s is set — OAuth login is disabled and ACCESS_TOKEN is ignored; requests use Authorization: Bearer <key> (key from %s, %s from %s)",
			mcpserver.APISecretEnv, envSource(mcpserver.APISecretEnv, fromDotEnv),
			apiBaseURLEnv, envSource(apiBaseURLEnv, fromDotEnv))
	}
	// mcpserver.New already REFUSED this combination unless it was explicitly
	// waived, so reaching here in API-key mode means the override is on.
	if mcpserver.IsInsecureCredentialTransport(info.APIBaseURL) {
		if apiKeyMode {
			log.Printf("WARNING: %s is set — the long-lived API key will be sent in cleartext to %q. Remove the override once the gateway has TLS.",
				mcpserver.AllowInsecureAPISecretEnv, info.APIBaseURL)
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
// Returns the set of keys it actually set, so the startup log can say whether a
// credential came from the file or from the process environment.
//
// The line shapes a *hand-written* secret arrives in — quoted values, an indent,
// spaces around the `=`, and the UTF-8 BOM that Notepad and PowerShell
// redirection prepend — are all handled by auth.ParseEnvLine, which the .env
// WRITERS in that package match against too. Keeping one parser is the point:
// when the reader accepted a shape the writer did not, a rewrite appended a
// duplicate line and the stale first occurrence kept winning.
func loadDotEnv(path string) map[string]bool {
	fromFile := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return fromFile
	}
	// No BOM strip here: auth.ParseEnvLine does it, so the loader and the .env
	// writers cannot drift apart on the first line of the file.
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := auth.ParseEnvLine(line)
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
			fromFile[key] = true
		}
	}
	return fromFile
}
