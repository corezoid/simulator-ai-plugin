package main

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// maxDescriptionChars bounds a SKILL.md description. Claude Code, Codex and
// Kiro load the name and description of every skill into every session that
// has the plugin installed, whether or not the session touches Simulator, and
// once the skill listing overflows Claude Code shows skills by name only. A
// description says what the skill does plus a few trigger phrases; everything
// else belongs in the body, which loads only when the skill runs.
const maxDescriptionChars = 200

func TestSkillDescriptions(t *testing.T) {
	skillsDir := filepath.Join("..", "..", "..", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		checked++
		name, desc := parseFrontmatter(path)
		if name != e.Name() {
			t.Errorf("%s: frontmatter name %q does not match the directory name", path, name)
		}
		if desc == "" {
			t.Errorf("%s: missing description", path)
			continue
		}
		if n := utf8.RuneCountInString(desc); n > maxDescriptionChars {
			t.Errorf("%s: description is %d characters, over the %d limit: keep what the skill does and a few trigger phrases, move the rest into the body",
				path, n, maxDescriptionChars)
		}
	}
	if checked == 0 {
		t.Fatalf("no SKILL.md under %s", skillsDir)
	}
}

// The length check is only as good as the parser: a regexp parser read
// `description: >-` as the two characters ">-" and a multi-line plain scalar
// as its first line, so a long description in either form passed the check.
func TestParseFrontmatterScalarForms(t *testing.T) {
	const want = "First line. Second line."
	for name, fm := range map[string]string{
		"folded":          "description: >\n  First line.\n  Second line.\n",
		"folded strip":    "description: >-\n  First line.\n  Second line.\n",
		"literal strip":   "description: |-\n  First line.\n  Second line.\n",
		"plain multiline": "description: First line.\n  Second line.\n",
		"blank line":      "description: >\n  First line.\n\n  Second line.\n",
		"double quoted":   "description: \"First line. Second line.\"\n",
	} {
		_, got := parseFrontmatterBytes([]byte("---\nname: x\n" + fm + "license: MIT\n---\nbody\n"))
		if got != want {
			t.Errorf("%s: description = %q, want %q", name, got, want)
		}
	}
}
