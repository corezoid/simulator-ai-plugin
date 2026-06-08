package cduschema

import (
	"sort"
	"testing"
)

func TestGetRules(t *testing.T) {
	r := GetRules()

	// Spot-check a sample of expected classes derived from the swagger.
	for _, want := range []string{"button", "edit", "label", "select", "table", "row", "draggable"} {
		if !r.Classes[want] {
			t.Errorf("expected class %q to be present", want)
		}
	}

	classes := make([]string, 0, len(r.Classes))
	for c := range r.Classes {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	t.Logf("extracted classes (%d): %v", len(classes), classes)

	if !r.GridType["one_column"] || !r.GridType["two_column"] {
		t.Error("expected one_column and two_column grid types")
	}
	if !r.Visibility["visible"] || !r.Visibility["hidden"] || !r.Visibility["disabled"] {
		t.Error("expected visibility values visible|hidden|disabled")
	}
	if !r.SectionType["body"] || !r.SectionType["modal"] {
		t.Error("expected section types body and modal")
	}
}

func TestValidatePageConfig_Valid(t *testing.T) {
	config := `{
		"grid": {"type": "one_column", "components": {"center": ["main"]}},
		"forms": [
			{
				"id": "main",
				"sections": [
					{
						"id": "body",
						"type": "body",
						"content": [
							{"id": "lbl", "class": "label", "value": "Hello"},
							{"id": "btn", "class": "button", "title": "Submit", "type": "default"}
						]
					}
				]
			}
		]
	}`
	errs := ValidateFile("pages/index/config", config)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidatePageConfig_Errors(t *testing.T) {
	config := `{
		"grid": {"type": "bad_type"},
		"forms": [
			{
				"sections": [
					{
						"type": "invalid_section",
						"content": [
							{"id": "x", "class": "unknownClass"},
							{"class": "label"}
						]
					}
				]
			}
		]
	}`
	errs := ValidateFile("pages/index/config", config)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	t.Logf("errors (%d):", len(errs))
	for _, e := range errs {
		t.Logf("  %s", e)
	}
	// Should catch: bad grid type, missing form id, bad section type, unknown class, missing item id
	if len(errs) < 4 {
		t.Errorf("expected at least 4 errors, got %d", len(errs))
	}
}

func TestValidateLocale(t *testing.T) {
	valid := `{"hello": {"en": "Hello", "uk": "Привіт"}}`
	if errs := ValidateFile("locale", valid); len(errs) != 0 {
		t.Errorf("expected no errors: %v", errs)
	}

	invalid := `{"hello": "bad value"}`
	if errs := ValidateFile("locale", invalid); len(errs) == 0 {
		t.Error("expected error for non-object locale value")
	}
}
