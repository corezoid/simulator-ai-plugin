package main

import (
	"os"
	"path/filepath"
	"testing"
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
		{"export prefix", `export SIMULATOR_API_SECRET=wsk_abc`, "SIMULATOR_API_SECRET", "wsk_abc"},
		{"export and quotes", `export SIMULATOR_API_SECRET="wsk_abc"`, "SIMULATOR_API_SECRET", "wsk_abc"},
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

func TestTrimMatchingQuotes(t *testing.T) {
	cases := map[string]string{
		`"a"`:  "a",
		`'a'`:  "a",
		`"a`:   `"a`,
		`a"`:   `a"`,
		`"a'`:  `"a'`,
		`""`:   "",
		`a`:    "a",
		``:     "",
		`"a"b`: `"a"b`,
	}
	for in, want := range cases {
		if got := trimMatchingQuotes(in); got != want {
			t.Errorf("trimMatchingQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeAndLoad(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	loadDotEnv(path)
}
