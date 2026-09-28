package cduschema

import (
	"strings"
	"testing"
)

// has reports whether any finding mentions every one of substrs.
func has(findings []string, substrs ...string) bool {
	for _, f := range findings {
		all := true
		for _, s := range substrs {
			if !strings.Contains(f, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// A missing locale key is an ERROR: locale is resolved from the static files
// only, so nothing at runtime can rescue it — the browser shows "[[key]]".
func TestValidateTreeMissingLocaleKeyIsError(t *testing.T) {
	files := map[string]string{
		"locale":             `{"brand":{"en":"Acme","uk":"Acme"}}`,
		"viewModel":          `{}`,
		"pages/index/locale": `{"greeting":{"en":"Hi","uk":"Привіт"}}`,
		"pages/index/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"a","class":"label","value":"[[greeting]]"},{"id":"c","class":"label","value":"[[brand]]"},{"id":"d","class":"label","value":"[[nowhere]]"}]}]}]}`,
	}
	f := ValidateTree(files)
	if !has(f.Errors, "pages/index/config", "[[nowhere]]") {
		t.Errorf("expected an error for the undefined locale key, got errors=%v", f.Errors)
	}
	// The two defined keys must not be reported — one from the page locale, one
	// from the app locale (they are merged at serve time).
	for _, k := range []string{"[[greeting]]", "[[brand]]"} {
		if has(f.Errors, k) {
			t.Errorf("%s is defined but was reported: %v", k, f.Errors)
		}
	}
}

// A page locale key only covers ITS page — the same key referenced from another
// page must still be reported.
func TestValidateTreePageLocaleIsNotGlobal(t *testing.T) {
	page := `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"a","class":"label","value":"[[only_on_one]]"}]}]}]}`
	files := map[string]string{
		"locale":           `{}`,
		"viewModel":        `{}`,
		"pages/one/locale": `{"only_on_one":{"en":"x","uk":"x"}}`,
		"pages/one/config": page,
		"pages/two/config": page,
	}
	f := ValidateTree(files)
	if has(f.Errors, "pages/one/config") {
		t.Errorf("page one defines the key locally and must not be reported: %v", f.Errors)
	}
	if !has(f.Errors, "pages/two/config", "[[only_on_one]]") {
		t.Errorf("page two does not define the key and must be reported: %v", f.Errors)
	}
}

// A missing viewModel default is a WARNING, not an error: the bound Corezoid
// process returns a per-request viewModel merged over the defaults, so the key
// may well be supplied at runtime.
func TestValidateTreeMissingViewModelDefaultIsWarning(t *testing.T) {
	files := map[string]string{
		"locale":             `{}`,
		"viewModel":          `{"known":"x"}`,
		"pages/index/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"a","class":"label","value":"{{known}}"},{"id":"c","class":"label","value":"{{undefaulted}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	if len(f.Errors) != 0 {
		t.Errorf("a viewModel miss must not block the push, got errors=%v", f.Errors)
	}
	if !has(f.Warnings, "{{undefaulted}}") {
		t.Errorf("expected a warning for the undefaulted key, got %v", f.Warnings)
	}
	if has(f.Warnings, "{{known}}") {
		t.Errorf("a defaulted key must not warn: %v", f.Warnings)
	}
}

// contentLoop template placeholders are substituted from the loop ENTRIES, not
// from the viewModel — reporting them as missing defaults would be a false
// positive on every list page.
func TestValidateTreeContentLoopVarsAreNotViewModelKeys(t *testing.T) {
	files := map[string]string{
		"locale":    `{}`,
		"viewModel": `{"promos_loop":[]}`,
		"pages/promo/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},
			"forms":[{"id":"f","sections":[{"id":"loop","type":"body","contentLoop":"{{promos_loop}}",
			"content":[{"id":"i","class":"image","value":"{{img}}"},{"id":"t","class":"label","value":"{{text}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	for _, k := range []string{"{{img}}", "{{text}}"} {
		if has(f.Warnings, k, "no default in `viewModel`") {
			t.Errorf("loop-scoped %s must not be treated as a viewModel key: %v", k, f.Warnings)
		}
	}
	// The array that FEEDS the loop is a real viewModel key and is defaulted.
	if has(f.Warnings, "{{promos_loop}}") {
		t.Errorf("promos_loop is defaulted and must not warn: %v", f.Warnings)
	}
}

// When the contentLoop entries are written out in the file we can check them
// exactly: a template placeholder absent from an entry renders literally in that
// row. (A templated contentLoop is backend-filled, so it stays silent — see the
// test above.)
func TestValidateTreeLiteralContentLoopChecksEntries(t *testing.T) {
	files := map[string]string{
		"locale":    `{}`,
		"viewModel": `{}`,
		"pages/p/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},
			"forms":[{"id":"f","sections":[{"id":"tiles","type":"body",
			"contentLoop":[{"img":"a.jpg","text":"A"},{"img":"b.jpg"}],
			"content":[{"id":"i","class":"image","value":"{{img}}"},{"id":"t","class":"label","value":"{{text}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	if len(f.Errors) != 0 {
		t.Errorf("a loop-entry gap must not block the push, got errors=%v", f.Errors)
	}
	// entry 1 has no "text"
	if !has(f.Warnings, `section "tiles"`, "{{text}}", "1") {
		t.Errorf("expected a warning naming the section and the gap entry, got %v", f.Warnings)
	}
	// "img" is present in BOTH entries — must not be reported.
	if has(f.Warnings, "{{img}}") {
		t.Errorf("img is present in every entry and must not warn: %v", f.Warnings)
	}
	// Loop vars are never viewModel keys, literal loop or not.
	if has(f.Warnings, "no default in `viewModel`") {
		t.Errorf("loop vars must not be reported as missing viewModel defaults: %v", f.Warnings)
	}
}

// A label bound to a key whose default is "" renders as an empty value, which the
// renderer rejects outright ("value" is not allowed to be empty). ValidateFile
// catches a literal "" but cannot see a default that resolves to one.
func TestValidateTreeEmptyDefaultOnLabelWarns(t *testing.T) {
	files := map[string]string{
		"locale":             `{}`,
		"viewModel":          `{"msg":"","note":" "}`,
		"pages/index/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"m","class":"label","value":"{{msg}}"},{"id":"n","class":"label","value":"{{note}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	if !has(f.Warnings, "{{msg}}", "empty") {
		t.Errorf("expected an empty-default warning for msg, got %v", f.Warnings)
	}
	if has(f.Warnings, "{{note}}", "empty") {
		t.Errorf("note defaults to a non-breaking space and must not warn: %v", f.Warnings)
	}
}

// A viewModel default no page references is dead weight — usually a leftover from
// a component that was removed.
func TestValidateTreeDeadViewModelDefault(t *testing.T) {
	files := map[string]string{
		"locale":             `{}`,
		"viewModel":          `{"used":"a","orphan":"b"}`,
		"pages/index/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"u","class":"label","value":"{{used}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	if !has(f.Warnings, "orphan", "dead default") {
		t.Errorf("expected a dead-default warning for orphan, got %v", f.Warnings)
	}
	if has(f.Warnings, `"used"`, "dead default") {
		t.Errorf("used is referenced and must not be called dead: %v", f.Warnings)
	}
}

// A definition is inlined into pages we cannot attribute statically, so its
// locale keys are checked against every locale file — an error only when defined
// nowhere.
func TestValidateTreeDefinitionLocale(t *testing.T) {
	files := map[string]string{
		"locale":             `{}`,
		"viewModel":          `{}`,
		"pages/index/locale": `{"submit":{"en":"Send","uk":"Надіслати"}}`,
		"pages/index/config": `{"grid":{"type":"one_column","components":{"center":["f"]}},"forms":[{"id":"f","sections":[{"id":"b","type":"body","content":[{"id":"x","$ref":"#/button"}]}]}]}`,
		"definitions/button": `{"class":"button","title":"[[submit]]","type":"default"}`,
		"definitions/broken": `{"class":"button","title":"[[never_defined]]","type":"default"}`,
	}
	f := ValidateTree(files)
	if has(f.Errors, "definitions/button") {
		t.Errorf("submit is defined in a page locale; the definition must not error: %v", f.Errors)
	}
	if !has(f.Errors, "definitions/broken", "[[never_defined]]") {
		t.Errorf("expected an error for the definition key defined nowhere: %v", f.Errors)
	}
}

// Malformed JSON is ValidateFile's job; ValidateTree must not double-report or panic.
func TestValidateTreeToleratesMalformedJSON(t *testing.T) {
	files := map[string]string{
		"locale":             `{ not json`,
		"viewModel":          `{"a":1}`,
		"pages/index/config": `{ also not json`,
	}
	f := ValidateTree(files) // must not panic
	for _, e := range f.Errors {
		if strings.Contains(e, "invalid JSON") {
			t.Errorf("ValidateTree should leave JSON syntax to ValidateFile, got %q", e)
		}
	}
}

// An env with no pages at all (e.g. a freshly created form) must stay silent
// rather than declaring every default dead.
func TestValidateTreeNoPagesIsSilent(t *testing.T) {
	f := ValidateTree(map[string]string{
		"locale":    `{"a":{"en":"A"}}`,
		"viewModel": `{"x":"1"}`,
	})
	if len(f.Errors) != 0 || len(f.Warnings) != 0 {
		t.Errorf("expected no findings for a page-less env, got errors=%v warnings=%v", f.Errors, f.Warnings)
	}
}

// A `[[…]]` run inside a pattern field is regex syntax, not a locale token. This
// one aborted the push over a locale key nobody ever wrote.
func TestValidateTreeIgnoresBracketsInPatternFields(t *testing.T) {
	files := map[string]string{
		"locale":    `{}`,
		"viewModel": `{}`,
		"pages/p/config": `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		  "content":[{"id":"e","class":"edit","type":"text","value":"","regexp":"^[[:alpha:]]+$","mask":"[[0-9]]"}]}]}]}`,
	}
	f := ValidateTree(files)
	if len(f.Errors) != 0 {
		t.Errorf("a character class in `regexp`/`mask` must not be read as a locale token: %v", f.Errors)
	}
}

// Only a section's `content` is expanded per loop entry. Its own fields are
// substituted once, from the viewModel — scoping them to the loop dropped the key
// and then reported its default as dead.
func TestValidateTreeSectionFieldsAreNotLoopScoped(t *testing.T) {
	files := map[string]string{
		"locale":    `{}`,
		"viewModel": `{"rows":[],"section_title":"T"}`,
		"pages/p/config": `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		  "title":"{{section_title}}","contentLoop":"{{rows}}",
		  "content":[{"id":"l","class":"label","value":"{{cell}}"}]}]}]}`,
	}
	f := ValidateTree(files)
	if has(f.Warnings, "section_title", "dead default") {
		t.Errorf("the section title references the key — it is not dead: %v", f.Warnings)
	}
	if has(f.Warnings, "{{cell}}", "no default in `viewModel`") {
		t.Errorf("the loop template var must stay loop-scoped: %v", f.Warnings)
	}
}

// A string sitting directly in an array is a leaf too — walk() skipped those, so
// every token inside one was invisible.
func TestValidateTreeHarvestsTokensInStringArrays(t *testing.T) {
	files := map[string]string{
		"locale":    `{}`,
		"viewModel": `{}`,
		"pages/p/config": `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		  "content":[{"id":"m","class":"mainMenu","value":"a","structure":["[[missing_loc]]","{{missing_vm}}"]}]}]}]}`,
	}
	f := ValidateTree(files)
	if !has(f.Errors, "[[missing_loc]]") {
		t.Errorf("expected the locale key inside the array to be reported: %v", f.Errors)
	}
	if !has(f.Warnings, "{{missing_vm}}") {
		t.Errorf("expected the viewModel key inside the array to be reported: %v", f.Warnings)
	}
}

// Scoping: a locale miss in a page this push does not write is pre-existing debt.
// Blocking on it would strand the user — pushSmartForm has no force flag.
func TestValidateTreeScopedUntouchedPageIsWarning(t *testing.T) {
	page := `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
	  "content":[{"id":"l","class":"label","value":"[[nowhere]]"}]}]}]}`
	files := map[string]string{
		"locale":            `{}`,
		"viewModel":         `{}`,
		"pages/old/config":  page,
		"pages/new/config":  page,
		"definitions/stale": `{"class":"button","title":"[[nowhere]]","type":"default"}`,
	}

	f := ValidateTreeScoped(files, map[string]bool{"pages/new/config": true})
	if !has(f.Errors, "pages/new/config", "[[nowhere]]") {
		t.Errorf("the page being written must still block: %v", f.Errors)
	}
	if has(f.Errors, "pages/old/config") || has(f.Errors, "definitions/stale") {
		t.Errorf("untouched files must not block an unrelated push: %v", f.Errors)
	}
	if !has(f.Warnings, "pages/old/config", "pre-existing") {
		t.Errorf("the untouched page should still be reported as a warning: %v", f.Warnings)
	}

	// Writing a locale file can be exactly what removed the key, so every page
	// resolving against it goes back in scope.
	f2 := ValidateTreeScoped(files, map[string]bool{"locale": true})
	for _, p := range []string{"pages/old/config", "pages/new/config", "definitions/stale"} {
		if !has(f2.Errors, p, "[[nowhere]]") {
			t.Errorf("%s must block when the app locale is being written: %v", p, f2.Errors)
		}
	}

	// A page locale file scopes its own page back in, and nothing else.
	f3 := ValidateTreeScoped(files, map[string]bool{"pages/old/locale": true})
	if !has(f3.Errors, "pages/old/config", "[[nowhere]]") {
		t.Errorf("the page whose locale is being written must block: %v", f3.Errors)
	}
	if has(f3.Errors, "pages/new/config") {
		t.Errorf("another page must stay a warning: %v", f3.Errors)
	}
}

// ValidateTree keeps the unscoped contract: every finding at full severity.
func TestValidateTreeUnscopedBlocksEverywhere(t *testing.T) {
	page := `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
	  "content":[{"id":"l","class":"label","value":"[[nowhere]]"}]}]}]}`
	f := ValidateTree(map[string]string{"locale": `{}`, "viewModel": `{}`, "pages/a/config": page})
	if !has(f.Errors, "pages/a/config", "[[nowhere]]") {
		t.Errorf("unscoped ValidateTree must report an error: %v", f.Errors)
	}
}
