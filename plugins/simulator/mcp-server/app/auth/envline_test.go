package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bom is the UTF-8 byte order mark a Notepad / PowerShell .env starts with.
var bom = string([]byte{0xEF, 0xBB, 0xBF})

func TestParseEnvLine(t *testing.T) {
	cases := []struct {
		line     string
		key, val string
		ok       bool
	}{
		{`ACCESS_TOKEN=abc`, "ACCESS_TOKEN", "abc", true},
		// .env is not a shell script — `export` makes the key "export ACCESS_TOKEN".
		{`export ACCESS_TOKEN=abc`, "", "", false},
		{"export\tACCESS_TOKEN=abc", "", "", false},
		{`   ACCESS_TOKEN = abc `, "ACCESS_TOKEN", "abc", true},
		{`ACCESS_TOKEN="abc"`, "ACCESS_TOKEN", "abc", true},
		{`ACCESS_TOKEN='abc'`, "ACCESS_TOKEN", "abc", true},
		{`ACCESS_TOKEN="abc`, "ACCESS_TOKEN", `"abc`, true}, // one-sided quote is part of the value
		{`ACCESS_TOKEN=abc"`, "ACCESS_TOKEN", `abc"`, true},
		{`ACCESS_TOKEN=""`, "ACCESS_TOKEN", "", true},
		{`ACCESS_TOKEN=ey=J0`, "ACCESS_TOKEN", "ey=J0", true},     // only the first = splits
		{`ACCESS_TOKEN=wsk#abc`, "ACCESS_TOKEN", "wsk#abc", true}, // # is not a comment mid-value
		{"ACCESS_TOKEN=abc\r", "ACCESS_TOKEN", "abc", true},
		{`exportACCESS_TOKEN=abc`, "exportACCESS_TOKEN", "abc", true}, // no space: a real key name
		{`export=abc`, "export", "abc", true},
		{`# ACCESS_TOKEN=abc`, "", "", false},
		{`no_equals_sign`, "", "", false},
		{`foo bar=baz`, "", "", false},
		{`=abc`, "", "", false},
		{``, "", "", false},
		{`   `, "", "", false},
	}
	for _, c := range cases {
		key, val, ok := ParseEnvLine(c.line)
		if ok != c.ok || key != c.key || val != c.val {
			t.Errorf("ParseEnvLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.line, key, val, ok, c.key, c.val, c.ok)
		}
	}
}

// A rewrite must land on the line that is already there, whatever shape it was
// written in — appending a second assignment leaves the loader reading the stale
// first one. The file's BOM and the author's indentation survive the rewrite.
func TestEnvWritersMatchEveryReadableShape(t *testing.T) {
	for _, shape := range []string{
		"  ACCESS_TOKEN=old",
		"\tACCESS_TOKEN=old",
		"ACCESS_TOKEN = old",
		`ACCESS_TOKEN="old"`,
		bom + "ACCESS_TOKEN=old",
	} {
		t.Run(shape, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(shape+"\nOTHER=keep\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := updateEnvFile(path, "ACCESS_TOKEN", "new"); err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(path)
			if n := countAssignments(string(body), "ACCESS_TOKEN"); n != 1 {
				t.Fatalf("want exactly one ACCESS_TOKEN line, got %d:\n%s", n, body)
			}
			if _, val, _ := ParseEnvLine(firstAssignment(string(body), "ACCESS_TOKEN")); val != "new" {
				t.Errorf("value not updated in place:\n%s", body)
			}
			if strings.HasPrefix(shape, bom) {
				if line := firstAssignment(string(body), "ACCESS_TOKEN"); !strings.HasPrefix(line, bom) {
					t.Errorf("the file's BOM must survive a rewrite, got %q", line)
				}
			}

			if err := removeEnvKey(path, "ACCESS_TOKEN"); err != nil {
				t.Fatal(err)
			}
			body, _ = os.ReadFile(path)
			if n := countAssignments(string(body), "ACCESS_TOKEN"); n != 0 {
				t.Errorf("removeEnvKey left the key behind:\n%s", body)
			}
			if n := countAssignments(string(body), "OTHER"); n != 1 {
				t.Errorf("an unrelated key was dropped:\n%s", body)
			}
		})
	}
}

func countAssignments(body, key string) int {
	n := 0
	for _, line := range splitLines(body) {
		if k, _, ok := ParseEnvLine(line); ok && k == key {
			n++
		}
	}
	return n
}

func firstAssignment(body, key string) string {
	for _, line := range splitLines(body) {
		if k, _, ok := ParseEnvLine(line); ok && k == key {
			return line
		}
	}
	return ""
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
