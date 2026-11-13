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

// A blank line inside a folded description used to end the match, so the
// length check above saw only the first paragraph.
func TestParseDescriptionSpansBlankLines(t *testing.T) {
	fm := "name: x\ndescription: >\n  First paragraph.\n\n  Second paragraph.\nlicense: MIT"
	if got, want := parseDescription(fm), "First paragraph. Second paragraph."; got != want {
		t.Errorf("parseDescription = %q, want %q", got, want)
	}
}
