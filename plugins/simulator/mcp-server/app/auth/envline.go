package auth

import "strings"

// ParseEnvLine normalises one .env line into the key/value pair a reader sees.
//
// It is the single source of truth for what a line MEANS, because the loader
// (cmd/server.loadDotEnv) and the writers in this file have to agree on it. When
// they did not, the damage was silent and one-directional: the loader accepted
// `export ACCESS_TOKEN=…` and a leading-indented `  WORKSPACE_ID=…`, while the
// writers matched a bare `KEY=` prefix, so a rewrite appended a SECOND line for a
// key that was already there — and the loader takes the FIRST occurrence, so the
// stale value won again on the next start. `Delete` had the same blind spot:
// an `export ACCESS_TOKEN=` line survived a logout.
//
// Accepted shapes (all of them arrive from a human editing .env by hand):
//
//	KEY=value            export KEY=value          KEY = value
//	KEY="value"          KEY='value'               "  KEY=value" (indented)
//
// ok is false for a blank line, a `#` comment, a line with no `=`, and a key
// containing whitespace (`foo bar=baz` is not a shell assignment either).
//
// Inline comments are NOT stripped: `#` is legal inside a secret, and treating it
// as a delimiter would corrupt the value.
func ParseEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line) // also drops a CRLF \r
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	rawKey, rawVal, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(trimExportPrefix(strings.TrimSpace(rawKey)))
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, trimMatchingQuotes(strings.TrimSpace(rawVal)), true
}

// trimExportPrefix drops a leading `export` keyword. Only a real keyword counts —
// `exportKEY=1` is a variable named exportKEY.
func trimExportPrefix(key string) string {
	const kw = "export"
	if !strings.HasPrefix(key, kw) || len(key) == len(kw) {
		return key
	}
	if c := key[len(kw)]; c != ' ' && c != '\t' {
		return key
	}
	return strings.TrimSpace(key[len(kw):])
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

// envLineAssigns reports whether line is an assignment to key, whatever shape it
// was written in, and returns the verbatim text before the key so a rewrite can
// preserve it — a user who wrote `export FOO=…` may be sourcing the file, and
// silently dropping the keyword would stop it exporting.
func envLineAssigns(line, key string) (prefix string, ok bool) {
	lineKey, _, parsed := ParseEnvLine(line)
	if !parsed || lineKey != key {
		return "", false
	}
	if i := strings.Index(line, key); i >= 0 {
		return line[:i], true
	}
	return "", true
}
