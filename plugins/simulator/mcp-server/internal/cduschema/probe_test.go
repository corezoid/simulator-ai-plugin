package cduschema

import "testing"

// Keys the docs present as real and that a form is likely to use. If any is absent
// from the swagger-derived union, a blanket top-level key check would reject legal
// config, so it must not be enabled.
func TestProbeDocumentedKeysPresentInUnion(t *testing.T) {
	r := GetRules()
	want := map[string][]string{
		"edit":   {"value", "type", "title", "required", "error", "errorMsg", "helpMsg", "placeholder", "regexp", "mask", "submitOnEnter", "resettable", "extra", "submitOnChange"},
		"table":  {"head", "body", "value", "type", "submitOnChange", "submitOnScroll", "extra"},
		"select": {"value", "options", "type", "submitOnChange", "submitOnScroll"},
		"button": {"title", "type", "tooltip", "extra"},
		"label":  {"value", "align", "tooltip"},
		"radio":  {"value", "options", "extra", "submitOnChange"},
		"check":  {"value", "required", "title"},
		"toggle": {"value", "title"},
		"image":  {"value", "extra", "align"},
		"copy":   {"value", "title"},
	}
	for class, keys := range want {
		allowed := r.ItemProps[class]
		if len(allowed) == 0 {
			t.Errorf("%s: no ItemProps derived", class)
			continue
		}
		for _, k := range keys {
			if !allowed[k] {
				t.Errorf("%s: documented key %q missing from swagger union", class, k)
			}
		}
	}
}
