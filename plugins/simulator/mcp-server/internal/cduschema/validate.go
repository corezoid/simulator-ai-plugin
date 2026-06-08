package cduschema

import (
	"encoding/json"
	"fmt"
	"strings"
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

		if vis, _ := form["visibility"].(string); vis != "" && !r.Visibility[vis] {
			errs = append(errs, fmt.Sprintf(
				"%s: %s: invalid visibility %q; must be visible|disabled|hidden", relPath, fLabel, vis,
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
	if vis, _ := section["visibility"].(string); vis != "" && !r.Visibility[vis] {
		errs = append(errs, fmt.Sprintf(
			"%s: %s: invalid visibility %q; must be visible|disabled|hidden", relPath, secLabel, vis,
		))
	}

	for _, slot := range []string{"header", "content", "footer", "modalHeader"} {
		items, _ := section[slot].([]any)
		slotLabel := fmt.Sprintf("%s.%s", secLabel, slot)
		errs = append(errs, validateItems(relPath, slotLabel, items, r)...)
	}
	return errs
}

func validateItems(relPath, path string, items []any, r *Rules) []string {
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

		if vis, _ := item["visibility"].(string); vis != "" && !r.Visibility[vis] {
			errs = append(errs, fmt.Sprintf(
				"%s: %s: invalid visibility %q; must be visible|disabled|hidden", relPath, itemLabel, vis,
			))
		}

		// Recurse into row/draggable layout wrapper children.
		if class == "row" || class == "draggable" {
			nested, _ := item["items"].([]any)
			errs = append(errs, validateItems(relPath, itemLabel+".items", nested, r)...)
		}
	}
	return errs
}
