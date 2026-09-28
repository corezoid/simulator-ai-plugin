package cduschema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	visibilityValues            = "visible|disabled|hidden"
	visibilityPlaceholderValues = visibilityValues + " or a {{viewModelKey}} placeholder"
)

// ValidateFile validates the source of a single Smart Form env file before push.
// relPath is the env-relative path (e.g. "pages/index/config", "locale", "definitions/button").
// Returns a slice of human-readable error messages; empty means valid.
func ValidateFile(relPath, source string) []string {
	parts := strings.Split(relPath, "/")
	base := parts[len(parts)-1]

	switch {
	case len(parts) >= 2 && parts[0] == "pages" && base == "config":
		return validatePageConfig(relPath, source)
	case base == "locale":
		return validateLocale(relPath, source)
	case base == "viewModel":
		return validateJSON(relPath, source)
	case len(parts) >= 2 && parts[0] == "definitions":
		return validateJSON(relPath, source)
	case base == "widgets":
		return validateJSON(relPath, source)
	}
	// styles/*.css, any unknown file — no structural check
	return nil
}

// validateJSON ensures the source is valid JSON.
func validateJSON(relPath, source string) []string {
	var v any
	if err := json.Unmarshal([]byte(source), &v); err != nil {
		return []string{fmt.Sprintf("%s: invalid JSON: %v", relPath, err)}
	}
	return nil
}

// validateLocale checks that the locale file is a JSON object whose every value is
// itself an object (the language map), e.g. {"hello": {"en": "Hello", "uk": "Привіт"}}.
func validateLocale(relPath, source string) []string {
	var v map[string]any
	if err := json.Unmarshal([]byte(source), &v); err != nil {
		return []string{fmt.Sprintf("%s: invalid JSON: %v", relPath, err)}
	}
	var errs []string
	for k, val := range v {
		if _, ok := val.(map[string]any); !ok {
			errs = append(errs, fmt.Sprintf(
				"%s: key %q must be a language map {\"en\": \"...\", \"uk\": \"...\"}, got %T",
				relPath, k, val,
			))
		}
	}
	return errs
}

// validatePageConfig validates a pages/<id>/config file against the CDU page protocol:
// Page → Grid → Form → Section → Item.
func validatePageConfig(relPath, source string) []string {
	var page map[string]any
	if err := json.Unmarshal([]byte(source), &page); err != nil {
		return []string{fmt.Sprintf("%s: invalid JSON: %v", relPath, err)}
	}

	r := GetRules()
	var errs []string

	// ── grid ──────────────────────────────────────────────────────────────────
	grid, _ := page["grid"].(map[string]any)
	if grid == nil {
		errs = append(errs, relPath+`: missing required field "grid"`)
	} else {
		gridType, _ := grid["type"].(string)
		if !r.GridType[gridType] {
			errs = append(errs, fmt.Sprintf(
				`%s: grid.type %q is not valid; expected one_column or two_column`, relPath, gridType,
			))
		}
	}

	// ── forms ─────────────────────────────────────────────────────────────────
	formsRaw, _ := page["forms"].([]any)
	if formsRaw == nil {
		errs = append(errs, relPath+`: missing required field "forms"`)
	}
	for i, f := range formsRaw {
		form, _ := f.(map[string]any)
		if form == nil {
			errs = append(errs, fmt.Sprintf("%s: forms[%d] must be an object", relPath, i))
			continue
		}
		formID, _ := form["id"].(string)
		fLabel := fmt.Sprintf("forms[%d]", i)
		if formID != "" {
			fLabel = fmt.Sprintf("forms[%q]", formID)
		} else {
			errs = append(errs, fmt.Sprintf(`%s: %s: missing required field "id"`, relPath, fLabel))
		}

		if vis, _ := form["visibility"].(string); vis != "" && !validVisibility(vis, r, true) {
			errs = append(errs, fmt.Sprintf(
				"%s: %s: invalid visibility %q; must be %s", relPath, fLabel, vis, visibilityPlaceholderValues,
			))
		}

		sections, _ := form["sections"].([]any)
		for j, sec := range sections {
			errs = append(errs, validateSection(relPath, fLabel, j, sec, r)...)
		}
	}
	return errs
}

func validateSection(relPath, formLabel string, idx int, raw any, r *Rules) []string {
	section, _ := raw.(map[string]any)
	if section == nil {
		return []string{fmt.Sprintf("%s: %s.sections[%d] must be an object", relPath, formLabel, idx)}
	}

	secID, _ := section["id"].(string)
	secLabel := fmt.Sprintf("%s.sections[%d]", formLabel, idx)
	if secID != "" {
		secLabel = fmt.Sprintf("%s.sections[%q]", formLabel, secID)
	}

	var errs []string

	if secType, _ := section["type"].(string); secType != "" && !r.SectionType[secType] {
		errs = append(errs, fmt.Sprintf(
			"%s: %s: invalid type %q; must be body|block|modal|float", relPath, secLabel, secType,
		))
	}
	if vis, _ := section["visibility"].(string); vis != "" && !validVisibility(vis, r, true) {
		errs = append(errs, fmt.Sprintf(
			"%s: %s: invalid visibility %q; must be %s", relPath, secLabel, vis, visibilityPlaceholderValues,
		))
	}

	// renderPage resolves item placeholders in these section slots.
	for _, slot := range []string{"header", "content", "modalHeader"} {
		items, _ := section[slot].([]any)
		slotLabel := fmt.Sprintf("%s.%s", secLabel, slot)
		errs = append(errs, validateItems(relPath, slotLabel, items, r, true)...)
	}

	// Keep validating the legacy slot, but do not accept placeholders that the
	// server does not resolve.
	footer, _ := section["footer"].([]any)
	errs = append(errs, validateItems(relPath, secLabel+".footer", footer, r, false)...)
	return errs
}

func validateItems(relPath, path string, items []any, r *Rules, allowVisibilityPlaceholder bool) []string {
	var errs []string
	for i, raw := range items {
		item, _ := raw.(map[string]any)
		if item == nil {
			errs = append(errs, fmt.Sprintf("%s: %s[%d] must be an object", relPath, path, i))
			continue
		}

		// $ref items are resolved server-side — nothing to validate structurally.
		if _, hasRef := item["$ref"]; hasRef {
			continue
		}

		itemID, _ := item["id"].(string)
		itemLabel := fmt.Sprintf("%s[%d]", path, i)
		if itemID != "" {
			itemLabel = fmt.Sprintf("%s[%q]", path, itemID)
		}

		class, _ := item["class"].(string)
		if class == "" {
			errs = append(errs, fmt.Sprintf(`%s: %s: missing required field "class"`, relPath, itemLabel))
		} else if !r.Classes[class] {
			errs = append(errs, fmt.Sprintf(
				"%s: %s: unknown class %q", relPath, itemLabel, class,
			))
		}

		// Dividers have no meaningful id; everything else should.
		if itemID == "" && class != "divider" {
			errs = append(errs, fmt.Sprintf(`%s: %s: missing required field "id"`, relPath, itemLabel))
		}

		if vis, _ := item["visibility"].(string); vis != "" && !validVisibility(vis, r, allowVisibilityPlaceholder) {
			expected := visibilityValues
			if allowVisibilityPlaceholder {
				expected = visibilityPlaceholderValues
			}
			errs = append(errs, fmt.Sprintf(
				"%s: %s: invalid visibility %q; must be %s", relPath, itemLabel, vis, expected,
			))
		}

		errs = append(errs, validateClientOnlyRules(relPath, itemLabel, class, item)...)
		errs = append(errs, validateItemKeys(relPath, itemLabel, class, item, r)...)
		errs = append(errs, validateNested(relPath, itemLabel, class, item, r)...)

		// Recurse into row/draggable layout wrapper children.
		if class == "row" || class == "draggable" {
			nested, _ := item["items"].([]any)
			errs = append(errs, validateItems(relPath, itemLabel+".items", nested, r, allowVisibilityPlaceholder)...)
		}
	}
	return errs
}

// validateClientOnlyRules enforces constraints the renderer (control-cdu) applies
// but the swagger does not express. The swagger carries no minLength anywhere, so
// these cannot be derived — they are transcribed from observed client validation
// errors and must be kept in sync by hand.
func validateClientOnlyRules(relPath, itemLabel, class string, item map[string]any) []string {
	var errs []string

	// `label` and `image` reject an empty string outright ("value is not allowed to
	// be empty"). This bites hardest when a viewModel default is "" to mean "nothing
	// to show" — use a non-breaking space, or omit the item.
	switch class {
	case "label", "image":
		raw, present := item["value"]
		str, isStr := raw.(string)
		if !present {
			errs = append(errs, fmt.Sprintf(
				`%s: %s: %s requires a non-empty "value"`, relPath, itemLabel, class,
			))
		} else if isStr && str == "" {
			hint := "use a non-breaking space (\u00a0) for a visually empty label"
			if class == "image" {
				// A data: URI does NOT work here: the renderer loads every image
				// through /api/1.0/image?src=, which rejects the scheme outright
				// (400 {"statusCode":400,"message":"URL is not allowed"}).
				hint = "point it at a placeholder the server can FETCH (an http(s) URL, or an actor-attached asset) — a data: URI is rejected by the image proxy"
			}
			errs = append(errs, fmt.Sprintf(
				`%s: %s: %s "value" must not be an empty string — the renderer rejects it; %s`,
				relPath, itemLabel, class, hint,
			))
		}
	}

	// An `image` value is never loaded directly: the renderer proxies it through
	// /api/1.0/image?src=<urlencoded>, and that proxy rejects the data: scheme with
	// 400 {"statusCode":400,"message":"URL is not allowed"} (cdu-page-protocol.md §4).
	// The image therefore renders as a broken box, and only the network tab says why.
	// A templated value ("{{qr_src}}") resolves per request and cannot be judged here.
	if class == "image" {
		if str, ok := item["value"].(string); ok && strings.HasPrefix(strings.TrimSpace(str), "data:") {
			errs = append(errs, fmt.Sprintf(
				`%s: %s: image "value" must be a URL the SERVER can fetch — a data: URI is `+
					"rejected by the image proxy (/api/1.0/image?src=) with "+
					`400 "URL is not allowed". Use an http(s) URL or an actor-attached asset. `+
					"(A data: URI in Less `url()` is fine — that path is not proxied.)",
				relPath, itemLabel,
			))
		}
	}
	return errs
}

// validateNested checks the sub-structures where the swagger is precise and the
// client renderer is strict: `extra`, `options[]`, and a table's `head[]` / `body[]`.
// The server accepts unknown keys here (no schema sets additionalProperties:false),
// so without this check they surface only as console errors in the browser.
//
// A value that is a template string (`"{{history_tx_body}}"`) is resolved per
// request and cannot be checked here, so it is skipped.
func validateNested(relPath, itemLabel, class string, item map[string]any, r *Rules) []string {
	var errs []string
	if class == "" {
		return nil
	}

	checkObject := func(label string, raw any, allowed map[string]bool) {
		if len(allowed) == 0 {
			return // no rule derived — never guess
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			return
		}
		for _, key := range sortedKeys(obj) {
			if !allowed[key] {
				errs = append(errs, fmt.Sprintf(
					"%s: %s.%s is not allowed; allowed keys: %s",
					relPath, label, key, strings.Join(sortedKeys(allowed), ", "),
				))
			}
		}
	}

	checkArray := func(field string, allowed map[string]bool) {
		if len(allowed) == 0 {
			return
		}
		arr, ok := item[field].([]any)
		if !ok {
			return // absent, or a "{{template}}" resolved at request time
		}
		for i, raw := range arr {
			checkObject(fmt.Sprintf("%s.%s[%d]", itemLabel, field, i), raw, allowed)
		}
	}

	checkObject(itemLabel+".extra", item["extra"], r.NestedProps[class+".extra"])
	checkArray("options", r.NestedProps[class+".options"])
	checkArray("head", r.NestedProps[class+".head"])
	checkArray("body", r.NestedProps[class+".body"])
	return errs
}

// sortedKeys returns a map's keys in a stable order, so a defect list reads the
// same on every run. Generic over the value type: the callers hold both
// map[string]any (an item's raw config) and map[string]bool (a derived allowlist).
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validateItemKeys reports keys an item carries that no schema variant of its class
// declares. A typo'd base field (`visibilty`, `styleclass`) is otherwise silent: the
// renderer ignores it and the item simply never hides or never picks up its style.
//
// Safe only because the allowlist is the UNION over every variant of the class —
// TestProbeDocumentedKeysPresentInUnion guards that assumption, so a swagger update
// that drops a real key fails the tests instead of rejecting valid config.
func validateItemKeys(relPath, itemLabel, class string, item map[string]any, r *Rules) []string {
	allowed := r.ItemProps[class]
	if len(allowed) == 0 {
		return nil // unknown class, or a renderer-only wrapper (row/draggable)
	}
	var errs []string
	for _, key := range sortedKeys(item) {
		if !allowed[key] {
			errs = append(errs, fmt.Sprintf(
				"%s: %s: %q is not a known %s field; allowed: %s",
				relPath, itemLabel, key, class, strings.Join(sortedKeys(allowed), ", "),
			))
		}
	}
	return errs
}

func validVisibility(value string, r *Rules, allowPlaceholder bool) bool {
	return r.Visibility[value] || allowPlaceholder && isViewModelPlaceholder(value)
}

func isViewModelPlaceholder(value string) bool {
	if !strings.HasPrefix(value, "{{") || !strings.HasSuffix(value, "}}") {
		return false
	}

	key := value[2 : len(value)-2]
	return key != "" && key == strings.TrimSpace(key) && !strings.ContainsAny(key, "{}")
}
