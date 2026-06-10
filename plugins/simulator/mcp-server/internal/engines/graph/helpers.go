package graph

import (
	"strconv"
)

// SysFormItem is a system form entry (used by the graph-sync form name cache).
type SysFormItem struct {
	ID          int                      `json:"id"          yaml:"id"`
	Title       string                   `json:"title"                yaml:"title"`
	Description string                   `json:"description,omitempty" yaml:"description,omitempty"`
	Fields      []map[string]any `json:"fields,omitempty" yaml:"fields,omitempty"`
	Childs      []SysFormItem            `json:"childs,omitempty" yaml:"childs,omitempty"`
}

// loadSysForms is a no-op placeholder for the pull-side cosmetic form-name
// lookup. The push-side GraphSyncer has its own API-backed loadSysForms; on the
// pull side, form names in the exported YAML are best-effort and optional.
func loadSysForms() ([]SysFormItem, error) { return nil, nil }

// resolveFormIDToName returns "" — form names in exported YAML are optional
// (the formId is always present and is what push relies on).
func resolveFormIDToName(id int) string { return "" }

// resolveFormNameToID returns 0 (cache miss); callers fall back to a live API
// lookup. The legacy sys-forms cache is not ported.
func resolveFormNameToID(name string) int { return 0 }

// findFormInTree locates a form by id within a SysFormItem tree, reporting its
// parent id and whether it is a child (non-root) form.
func findFormInTree(forms []SysFormItem, formID, parentID int) (parent int, isChild, found bool) {
	for i := range forms {
		if forms[i].ID == formID {
			return parentID, parentID != 0, true
		}
		if len(forms[i].Childs) > 0 {
			if p, ch, ok := findFormInTree(forms[i].Childs, formID, forms[i].ID); ok {
				return p, ch, true
			}
		}
	}
	return 0, false, false
}

// omitEmptyFields recursively drops keys whose values are empty (nil, "",
// empty slice/map) — used to keep exported actor data tidy.
func omitEmptyFields(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		cleaned := cleanFieldValue(v)
		if !isEmptyFieldValue(cleaned) {
			result[k] = cleaned
		}
	}
	return result
}

func cleanFieldValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		c := omitEmptyFields(val)
		if len(c) == 0 {
			return nil
		}
		return c
	case []any:
		if len(val) == 0 {
			return nil
		}
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = cleanFieldValue(item)
		}
		return out
	default:
		return v
	}
}

func isEmptyFieldValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	}
	return false
}

// toInt coerces a JSON tool-argument value (float64 / string / int) to int.
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	}
	return 0
}
