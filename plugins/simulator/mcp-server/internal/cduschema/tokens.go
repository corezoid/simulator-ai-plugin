package cduschema

// Cross-file token audit.
//
// ValidateFile (validate.go) is deliberately per-file: it takes one relPath and
// one source, so it can never see whether a `[[key]]` in a page resolves against
// the locale files, or whether a `{{key}}` has a viewModel default. Those defects
// therefore survive every server-side check — the app_content endpoint stores the
// source opaquely, and pong-server substitutes whatever it finds and serves the
// rest verbatim. The user meets them as a literal `[[key]]` on the rendered page.
//
// ValidateTree closes that gap by auditing the whole env tree at once. It runs
// alongside the per-file pass in pushSmartForm.
//
// The error/warning split is deliberate and follows what the runtime can still
// rescue:
//
//   - a missing LOCALE key is an ERROR. Locale is resolved purely from the static
//     files (app locale merged with the page locale); nothing at runtime can
//     supply it, so an unresolved `[[key]]` always reaches the browser as text.
//   - a missing viewModel default is a WARNING. The Corezoid process returns a
//     per-request viewModel that is merged over the defaults, so an undefaulted
//     key may well be filled at runtime — it just renders as a literal `{{key}}`
//     whenever the backend call fails, which is why it is still worth flagging.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	localeTokenRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	vmTokenRe     = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
)

// TreeFindings is the result of a cross-file audit. Errors should block a push;
// Warnings are reported alongside a successful one.
type TreeFindings struct {
	Errors   []string
	Warnings []string
}

// ValidateTree audits an entire Smart Form env tree for cross-file token defects
// that no single-file check can see. files maps env-relative paths (the same keys
// pushSmartForm collects: "locale", "viewModel", "pages/<id>/config",
// "pages/<id>/locale", "definitions/<name>", …) to their source.
//
// Pass the WHOLE tree, not just the files being written: deleting a viewModel
// default breaks an untouched page, and that is exactly the case a changed-files
// audit would miss.
func ValidateTree(files map[string]string) TreeFindings {
	var f TreeFindings

	appLocale := localeKeys(files["locale"])
	viewModel := objectKeys(files["viewModel"])
	emptyDefaults := emptyStringKeys(files["viewModel"])

	// Every locale key defined anywhere — used to check definitions/, which are
	// inlined into pages we cannot attribute statically.
	anyLocale := map[string]bool{}
	for k := range appLocale {
		anyLocale[k] = true
	}
	pageLocales := map[string]map[string]bool{}
	for path, src := range files {
		if page, ok := pagePart(path, "locale"); ok {
			keys := localeKeys(src)
			pageLocales[page] = keys
			for k := range keys {
				anyLocale[k] = true
			}
		}
	}

	referenced := map[string]bool{} // every {{key}} any page/definition uses

	// ── page configs ────────────────────────────────────────────────────────
	pages := make([]string, 0, len(files))
	for path := range files {
		if _, ok := pagePart(path, "config"); ok {
			pages = append(pages, path)
		}
	}
	sort.Strings(pages)

	for _, path := range pages {
		page, _ := pagePart(path, "config")
		scan := scanConfig(files[path])

		for _, k := range sortedKeys(scan.locale) {
			if appLocale[k] || pageLocales[page][k] {
				continue
			}
			f.Errors = append(f.Errors, fmt.Sprintf(
				"%s: locale key [[%s]] is defined in neither `locale` nor `pages/%s/locale` — "+
					"it will render as the literal text \"[[%s]]\" (locale is resolved from files only; "+
					"the backend cannot supply it)", path, k, page, k))
		}

		for _, k := range sortedKeys(scan.viewModel) {
			referenced[k] = true
			if viewModel[k] {
				if emptyDefaults[k] && scan.bareValueOf[k] != "" {
					f.Warnings = append(f.Warnings, fmt.Sprintf(
						"%s: %s value is exactly \"{{%s}}\" and its `viewModel` default is the empty "+
							"string — if the backend sends nothing the renderer rejects the item "+
							"(\"value\" is not allowed to be empty). Default it to a non-breaking space "+
							"(label), or a fetchable placeholder URL (image) — a data: URI is "+
							"rejected by the image proxy", path, scan.bareValueOf[k], k))
				}
				continue
			}
			f.Warnings = append(f.Warnings, fmt.Sprintf(
				"%s: {{%s}} has no default in `viewModel` — it renders as the literal \"{{%s}}\" "+
					"whenever the backend does not supply it (e.g. a failed /get)", path, k, k))
		}

		// A section whose contentLoop is a LITERAL array can be checked exactly:
		// every {{k}} in the template must be present in every entry. (A templated
		// contentLoop is filled by the backend, so its template vars are silent.)
		for _, issue := range scan.loopIssues {
			f.Warnings = append(f.Warnings, path+": "+issue)
		}
	}

	// ── definitions ─────────────────────────────────────────────────────────
	defs := make([]string, 0, len(files))
	for path := range files {
		if strings.HasPrefix(path, "definitions/") {
			defs = append(defs, path)
		}
	}
	sort.Strings(defs)
	for _, path := range defs {
		scan := scanConfig(files[path])
		for _, k := range sortedKeys(scan.locale) {
			if anyLocale[k] {
				continue
			}
			f.Errors = append(f.Errors, fmt.Sprintf(
				"%s: locale key [[%s]] is defined in no locale file — a $ref'd definition renders it "+
					"as literal text on every page that inlines it", path, k))
		}
		for _, k := range sortedKeys(scan.viewModel) {
			referenced[k] = true
		}
	}

	// ── dead viewModel defaults ─────────────────────────────────────────────
	// A viewModel default exists only to back a page placeholder, so one that no
	// page or definition references is dead weight (and usually a leftover from a
	// removed component).
	if len(pages) > 0 {
		for _, k := range sortedKeys(viewModel) {
			if !referenced[k] {
				f.Warnings = append(f.Warnings, fmt.Sprintf(
					"viewModel: key %q is referenced by no page or definition — dead default", k))
			}
		}
	}

	return f
}

// configScan is what one page config / definition contributed.
type configScan struct {
	locale    map[string]bool
	viewModel map[string]bool
	// bareValueOf[k] names the component class when an item's `value` is exactly
	// "{{k}}" and the class is one the renderer rejects an empty value on.
	bareValueOf map[string]string
	// loopIssues holds findings about sections whose contentLoop is a literal
	// array — there the entries are in the file, so the template's placeholders
	// can be checked against them exactly.
	loopIssues []string
}

// scanConfig walks a page config (or a definition fragment) and collects the
// locale / viewModel tokens it uses. Placeholders inside a section whose
// `contentLoop` is itself templated are loop-scoped: they are substituted from
// the loop entries the backend returns, so they are NOT viewModel keys.
func scanConfig(source string) configScan {
	s := configScan{
		locale:      map[string]bool{},
		viewModel:   map[string]bool{},
		bareValueOf: map[string]string{},
	}
	var root any
	if err := json.Unmarshal([]byte(source), &root); err != nil {
		return s // malformed JSON is already reported by ValidateFile
	}
	s.walk(root, false)
	return s
}

func (s *configScan) walk(node any, inLoop bool) {
	switch v := node.(type) {
	case map[string]any:
		// A section with a contentLoop makes its `content` template loop-scoped:
		// those {{vars}} are substituted per entry, never from the viewModel.
		loopHere := inLoop
		if cl, ok := v["contentLoop"]; ok {
			switch loop := cl.(type) {
			case string:
				// Bound from the viewModel — the ARRAY is the viewModel key; the
				// template's own vars come from whatever the backend returns.
				for _, m := range vmTokenRe.FindAllStringSubmatch(loop, -1) {
					s.viewModel[strings.TrimSpace(m[1])] = true
				}
			case []any:
				// Literal entries: they are right here, so check them exactly.
				s.checkLiteralLoop(v, loop)
			}
			loopHere = true
		}

		// Remember components whose `value` is a single bare placeholder and whose
		// class rejects an empty value.
		if cls, ok := v["class"].(string); ok && (cls == "label" || cls == "image") {
			if val, ok := v["value"].(string); ok {
				if m := vmTokenRe.FindStringSubmatch(val); m != nil && strings.TrimSpace(val) == m[0] {
					s.bareValueOf[strings.TrimSpace(m[1])] = cls
				}
			}
		}

		for k, child := range v {
			if k == "contentLoop" {
				continue // already handled
			}
			s.walkString(k, child, loopHere)
			s.walk(child, loopHere)
		}
	case []any:
		for _, child := range v {
			s.walk(child, inLoop)
		}
	}
}

// walkString harvests tokens out of a string leaf. Loop-scoped placeholders are
// dropped: they are filled per contentLoop entry, so they are not viewModel keys
// and treating them as such would false-positive on every list page.
func (s *configScan) walkString(_ string, node any, inLoop bool) {
	str, ok := node.(string)
	if !ok {
		return
	}
	for _, m := range localeTokenRe.FindAllStringSubmatch(str, -1) {
		s.locale[strings.TrimSpace(m[1])] = true
	}
	if inLoop {
		return
	}
	for _, m := range vmTokenRe.FindAllStringSubmatch(str, -1) {
		s.viewModel[strings.TrimSpace(m[1])] = true
	}
}

// checkLiteralLoop verifies a section whose contentLoop entries are written out
// in the file: every {{var}} the content template uses must exist in every entry,
// or that row renders a literal "{{var}}".
func (s *configScan) checkLiteralLoop(section map[string]any, entries []any) {
	content, ok := section["content"]
	if !ok {
		return
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return
	}
	used := map[string]bool{}
	for _, m := range vmTokenRe.FindAllStringSubmatch(string(raw), -1) {
		used[strings.TrimSpace(m[1])] = true
	}
	if len(used) == 0 {
		return
	}
	label := "a section"
	if id, ok := section["id"].(string); ok && id != "" {
		label = "section " + quote(id)
	}
	for _, key := range sortedKeys(used) {
		var missing []string
		for i, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok || entry[key] == nil {
				missing = append(missing, itoa(i))
			}
		}
		if len(missing) == 0 {
			continue
		}
		s.loopIssues = append(s.loopIssues, fmt.Sprintf(
			"%s has literal `contentLoop` entries and its template uses {{%s}}, but entr%s %s "+
				"do not carry that key — those rows render the literal \"{{%s}}\"",
			label, key, plural(len(missing), "y", "ies"), strings.Join(missing, ", "), key))
	}
}

func quote(s string) string { return "\"" + s + "\"" }

func itoa(i int) string { return fmt.Sprintf("%d", i) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// pagePart reports the page id when path is "pages/<id>/<base>".
func pagePart(path, base string) (string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 3 && parts[0] == "pages" && parts[2] == base {
		return parts[1], true
	}
	return "", false
}

// localeKeys returns the top-level keys of a locale file.
func localeKeys(source string) map[string]bool { return objectKeys(source) }

// objectKeys returns the top-level keys of a JSON object, or an empty set.
func objectKeys(source string) map[string]bool {
	out := map[string]bool{}
	if source == "" {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(source), &m); err != nil {
		return out
	}
	for k := range m {
		out[k] = true
	}
	return out
}

// emptyStringKeys returns the keys whose value is exactly "".
func emptyStringKeys(source string) map[string]bool {
	out := map[string]bool{}
	if source == "" {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(source), &m); err != nil {
		return out
	}
	for k, v := range m {
		if s, ok := v.(string); ok && s == "" {
			out[k] = true
		}
	}
	return out
}
