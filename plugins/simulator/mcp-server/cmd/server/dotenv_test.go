package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/auth"
)

// loadDotEnv is the first thing a hand-pasted secret passes through, and every
// failure mode below is silent: either the quotes end up inside the
// Authorization header, or the variable is never set and the user is dropped
// back to OAuth with no explanation.
func TestLoadDotEnvParsesHandWrittenValues(t *testing.T) {
	cases := []struct {
		name string
		line string
		key  string
		want string
	}{
		{"plain", `SIMULATOR_API_SECRET=wsk_abc`, "SIMULATOR_API_SECRET", "wsk_abc"},
		{"double quoted", `SIMULATOR_API_SECRET="wsk_abc"`, "SIMULATOR_API_SECRET", "wsk_abc"},
		{"single quoted", `SIMULATOR_API_SECRET='wsk_abc'`, "SIMULATOR_API_SECRET", "wsk_abc"},
		{"crlf", "SIMULATOR_API_SECRET=wsk_abc\r", "SIMULATOR_API_SECRET", "wsk_abc"},
		{"equals inside value", `SIMULATOR_API_SECRET=eyJhbGc=`, "SIMULATOR_API_SECRET", "eyJhbGc="},
		{"hash is part of the secret", `SIMULATOR_API_SECRET=wsk#abc`, "SIMULATOR_API_SECRET", "wsk#abc"},
		{"unbalanced quote is kept", `SIMULATOR_API_SECRET="wsk_abc`, "SIMULATOR_API_SECRET", `"wsk_abc`},
		{"surrounding spaces", `SIMULATOR_API_SECRET =  wsk_abc  `, "SIMULATOR_API_SECRET", "wsk_abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, "")
			os.Unsetenv(tc.key)
			writeAndLoad(t, tc.line+"\n")
			if got := os.Getenv(tc.key); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// A .env written by Notepad or PowerShell redirection starts with a UTF-8 BOM,
// which would otherwise become part of the first key's name.
func TestLoadDotEnvStripsBOM(t *testing.T) {
	t.Setenv("SIMULATOR_API_SECRET", "")
	os.Unsetenv("SIMULATOR_API_SECRET")

	writeAndLoad(t, "\uFEFFSIMULATOR_API_SECRET=wsk_abc\n")

	if got := os.Getenv("SIMULATOR_API_SECRET"); got != "wsk_abc" {
		t.Errorf("SIMULATOR_API_SECRET = %q, want %q", got, "wsk_abc")
	}
}

func TestLoadDotEnvDoesNotOverrideExistingValue(t *testing.T) {
	t.Setenv("SIMULATOR_API_SECRET", "from_environment")

	writeAndLoad(t, "SIMULATOR_API_SECRET=from_file\n")

	if got := os.Getenv("SIMULATOR_API_SECRET"); got != "from_environment" {
		t.Errorf("SIMULATOR_API_SECRET = %q, want the pre-existing environment value", got)
	}
}

func TestLoadDotEnvSkipsCommentsAndJunk(t *testing.T) {
	t.Setenv("DOTENV_TEST_KEY", "")
	os.Unsetenv("DOTENV_TEST_KEY")

	writeAndLoad(t, "# a comment\n\nno_equals_sign\nDOTENV_TEST_KEY=value\n")

	if got := os.Getenv("DOTENV_TEST_KEY"); got != "value" {
		t.Errorf("DOTENV_TEST_KEY = %q, want %q", got, "value")
	}
}

func TestLoadDotEnvMissingFileIsFine(t *testing.T) {
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist"))
}

func writeAndLoad(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	loadDotEnv(path)
}

// The loader and the .env writers must read a line the same way. They did not:
// the loader trimmed a line before splitting it, the writers matched a bare `KEY=` prefix, so
// a rewrite appended a duplicate — and the loader takes the FIRST occurrence, so
// the value the user just changed lost to the stale one on the next start.
func TestLoadDotEnvAgreesWithTheEnvWriters(t *testing.T) {
	for _, shape := range []string{
		"  ACCESS_TOKEN=old",
		`ACCESS_TOKEN="old"`,
		// A Notepad / PowerShell .env: the BOM sits on the first line, which is
		// exactly the line the writers have to match.
		"\uFEFFACCESS_TOKEN=old",
	} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SIMULATOR_WORK_DIR", dir)
			path := filepath.Join(dir, ".env")
			if err := os.WriteFile(path, []byte(shape+"\n"), 0o600); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			// Read it first. Asserting only the post-Save value is not enough: if
			// the loader and the writers are BOTH blind to a shape they stay
			// consistent with each other while silently never loading the key at
			// all, and the round-trip below still passes.
			os.Unsetenv("ACCESS_TOKEN")
			loadDotEnv(path)
			if got := os.Getenv("ACCESS_TOKEN"); got != "old" {
				t.Fatalf("loader did not read this shape: ACCESS_TOKEN = %q, want %q", got, "old")
			}

			if err := auth.Save(&auth.Credentials{AccessToken: "new"}); err != nil {
				t.Fatalf("save: %v", err)
			}
			os.Unsetenv("ACCESS_TOKEN")
			loadDotEnv(path)
			if got := os.Getenv("ACCESS_TOKEN"); got != "new" {
				body, _ := os.ReadFile(path)
				t.Errorf("after Save, ACCESS_TOKEN = %q, want %q; .env is:\n%s", got, "new", body)
			}
			if n := countAssignments(t, path, "ACCESS_TOKEN"); n != 1 {
				body, _ := os.ReadFile(path)
				t.Errorf("after Save, .env holds %d ACCESS_TOKEN assignments, want 1 (a rewrite must not append); .env is:\n%s", n, body)
			}

			if err := auth.Delete(); err != nil {
				t.Fatalf("delete: %v", err)
			}
			os.Unsetenv("ACCESS_TOKEN")
			loadDotEnv(path)
			if got := os.Getenv("ACCESS_TOKEN"); got != "" {
				body, _ := os.ReadFile(path)
				t.Errorf("after Delete, ACCESS_TOKEN = %q, want it gone; .env is:\n%s", got, body)
			}
		})
	}
}

// loadDotEnv reports which keys it supplied, so the startup log can tell a key
// exported in the shell apart from one read out of the project's .env.
func TestLoadDotEnvReportsItsOwnKeys(t *testing.T) {
	t.Setenv("SIMULATOR_API_SECRET", "from_environment")
	t.Setenv("DOTENV_TEST_KEY", "")
	os.Unsetenv("DOTENV_TEST_KEY")

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("SIMULATOR_API_SECRET=from_file\nDOTENV_TEST_KEY=value\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	fromFile := loadDotEnv(path)

	if fromFile["SIMULATOR_API_SECRET"] {
		t.Error("the environment already had SIMULATOR_API_SECRET — the file did not supply it")
	}
	if !fromFile["DOTENV_TEST_KEY"] {
		t.Error("DOTENV_TEST_KEY came from the file and should be reported as such")
	}
	if got := envSource("SIMULATOR_API_SECRET", fromFile); got != "the process environment" {
		t.Errorf("envSource = %q, want the process environment", got)
	}
	if got := envSource("DOTENV_TEST_KEY", fromFile); got != ".env" {
		t.Errorf("envSource = %q, want .env", got)
	}
}

// countAssignments reports how many lines assign key, as the loader sees them.
// A rewrite that appends instead of replacing leaves two, and the loader then
// takes the stale first one.
func countAssignments(t *testing.T, path, key string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if k, _, ok := auth.ParseEnvLine(line); ok && k == key {
			n++
		}
	}
	return n
}
