package auth

import "testing"

// envLineAssigns returns the verbatim text before the key so a rewrite can
// preserve it. Getting that offset from the first substring match in the raw
// line renames the variable when the key also occurs inside the prefix.
func TestEnvLineAssignsPrefix(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF})
	cases := []struct {
		name, line, key, wantPrefix string
		wantOK                      bool
	}{
		{"bare", "ACCESS_TOKEN=old", "ACCESS_TOKEN", "", true},
		{"export", "export ACCESS_TOKEN=old", "ACCESS_TOKEN", "export ", true},
		{"export tab", "export\tACCESS_TOKEN=old", "ACCESS_TOKEN", "export\t", true},
		{"indented export", "  export ACCESS_TOKEN=old", "ACCESS_TOKEN", "  export ", true},
		{"spaced equals", "ACCESS_TOKEN = old", "ACCESS_TOKEN", "", true},
		{"bom keeps the mark", bom + "ACCESS_TOKEN=old", "ACCESS_TOKEN", bom, true},
		// The key occurs inside the "export" keyword itself. Taking the first
		// substring hit yielded prefix "ex", rewriting the line as "export=2".
		{"key is a substring of the prefix", "export port=1", "port", "export ", true},
		{"value looks like the key", "export FOO=FOO", "FOO", "export ", true},
		{"different key", "WORKSPACE_ID=w1", "ACCESS_TOKEN", "", false},
		{"comment", "# ACCESS_TOKEN=old", "ACCESS_TOKEN", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, ok := envLineAssigns(tc.line, tc.key)
			if ok != tc.wantOK {
				t.Fatalf("envLineAssigns(%q, %q) ok = %v, want %v", tc.line, tc.key, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if prefix != tc.wantPrefix {
				t.Errorf("prefix = %q, want %q", prefix, tc.wantPrefix)
			}
			// The whole point of the prefix: the rewritten line must still assign
			// the same key, in the same shape.
			rewritten := prefix + tc.key + "=NEW"
			gotKey, gotVal, parsed := ParseEnvLine(rewritten)
			if !parsed || gotKey != tc.key || gotVal != "NEW" {
				t.Errorf("rewrite %q parses as (%q, %q, %v), want (%q, %q, true)",
					rewritten, gotKey, gotVal, parsed, tc.key, "NEW")
			}
		})
	}
}
