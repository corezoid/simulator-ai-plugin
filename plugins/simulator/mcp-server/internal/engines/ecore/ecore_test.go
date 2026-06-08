package ecore

import (
	"strings"
	"testing"
)

// TestRequireUUIDRejectsTraversal locks in the guard that stops path-traversal
// and URL-injection via tool arguments that are interpolated into file paths
// (<layerId>.yaml) and API URLs.
func TestRequireUUIDRejectsTraversal(t *testing.T) {
	good := "8e86c99d-ffda-40a4-acd9-eef94418d4a9"
	if r := RequireUUID("layerId", good); r != nil {
		t.Fatalf("valid UUID %q was rejected", good)
	}
	bad := []string{
		"../../../../etc/passwd",
		"../../secret",
		"id?admin=1",
		"id/../../x",
		"foo#frag",
		"",
		"not-a-uuid",
	}
	for _, v := range bad {
		if r := RequireUUID("layerId", v); r == nil {
			t.Errorf("malicious/invalid value %q was accepted", v)
		}
	}
}

// TestSegEscapesPathSeparators verifies that IDs from a graph file are escaped
// before being placed in a URL path segment, so a "/" or "?" cannot alter the
// request path or inject query parameters.
func TestSegEscapesPathSeparators(t *testing.T) {
	cases := map[string]bool{
		"8e86c99d-ffda-40a4-acd9-eef94418d4a9": false, // a real UUID must be unchanged
		"a/b":     true,
		"a?x=1":   true,
		"a#frag":  true,
		"../../x": true,
	}
	for in, mustChange := range cases {
		out := Seg(in)
		if mustChange && out == in {
			t.Errorf("Seg(%q) = %q: separators not escaped", in, out)
		}
		if !mustChange && out != in {
			t.Errorf("Seg(%q) = %q: a plain UUID should be unchanged", in, out)
		}
		if strings.ContainsAny(out, "/?#") {
			t.Errorf("Seg(%q) = %q still contains a path/query metacharacter", in, out)
		}
	}
}

// TestIsUUID checks IsUUID accepts well-formed UUIDs (case-insensitive) and
// rejects malformed ones.
func TestIsUUID(t *testing.T) {
	valid := []string{
		"ab636ecd-b68b-4a33-97cb-309521b330e6",
		"AB636ECD-B68B-4A33-97CB-309521B330E6", // case-insensitive
	}
	invalid := []string{"", "not-a-uuid", "12345", "ab636ecd-b68b-4a33-97cb"}
	for _, s := range valid {
		if !IsUUID(s) {
			t.Errorf("IsUUID(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsUUID(s) {
			t.Errorf("IsUUID(%q) = true, want false", s)
		}
	}
}

// TestBuildBaseURLPrecedence covers the deterministic precedence in BuildBaseURL:
// Cfg.Url overrides Cfg.BaseUrl, and an empty config falls back to the
// hard-coded default; trailing slashes are trimmed.
// Mutates package-global Cfg, so it cannot run in parallel.
func TestBuildBaseURLPrecedence(t *testing.T) {
	saved := Cfg
	t.Cleanup(func() { Cfg = saved })

	Cfg = Config{}
	if got := BuildBaseURL(); got != "https://api.simulator.company/v/1.0" {
		t.Errorf("BuildBaseURL (empty) = %q, want default", got)
	}

	Cfg = Config{BaseUrl: "https://base.example/papi/1.0/"}
	if got := BuildBaseURL(); got != "https://base.example/papi/1.0" {
		t.Errorf("BuildBaseURL (BaseUrl) = %q, want trimmed base", got)
	}

	Cfg = Config{Url: "https://url.example/v/1.0/", BaseUrl: "https://base.example/papi/1.0"}
	if got := BuildBaseURL(); got != "https://url.example/v/1.0" {
		t.Errorf("BuildBaseURL (Url wins) = %q, want url override", got)
	}
}
