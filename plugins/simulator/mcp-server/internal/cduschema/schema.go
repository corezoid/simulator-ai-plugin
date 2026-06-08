// Package cduschema loads the CDU (Smart Form) page protocol schema from the
// bundled swagger and exposes lightweight validation rules derived from it.
package cduschema

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed smart-forms-swagger.json
var swaggerJSON []byte

// Rules holds sets of valid values extracted from the swagger schema.
type Rules struct {
	// Classes is the set of valid item `class` values.
	// Source: allOf[1].properties.class.allOf[1].enum in each component schema,
	// plus `row` and `draggable` (renderer-side layout wrappers absent from the swagger).
	Classes map[string]bool

	// Visibility is the set of valid visibility values (visible|disabled|hidden).
	Visibility map[string]bool

	// SectionType is the set of valid section type values (body|block|modal|float).
	SectionType map[string]bool

	// GridType is the set of valid grid type values (one_column|two_column).
	GridType map[string]bool
}

var (
	rulesOnce sync.Once
	rules     *Rules
)

// GetRules returns the singleton Rules, parsing the embedded swagger on first call.
// If the swagger cannot be parsed the built-in fallback values are returned.
func GetRules() *Rules {
	rulesOnce.Do(func() {
		rules = parseRules(swaggerJSON)
	})
	return rules
}

func parseRules(data []byte) *Rules {
	r := &Rules{
		Classes: make(map[string]bool),
		// Extracted from Form.properties.visibility.allOf[0].enum
		Visibility: map[string]bool{"visible": true, "hidden": true, "disabled": true},
		// Extracted from Form.properties.sections.items.properties.type.enum
		SectionType: map[string]bool{"body": true, "block": true, "modal": true, "float": true},
		// Extracted from Page.properties.grid.properties.type.example / Page-grid-* discriminator mapping
		GridType: map[string]bool{"one_column": true, "two_column": true},
	}

	// Decode only the components.schemas map — avoid holding the full 3 MB in memory.
	var swagger struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &swagger); err != nil {
		return r
	}

	// Component schemas embed their class enum in one of two ways depending on depth:
	//
	//   Pattern A (2-entry allOf, e.g. Button-default, Label):
	//     allOf[N].properties.class.allOf[1].enum = ["<class>"]
	//
	//   Pattern B (Table-default — direct enum without inner allOf):
	//     allOf[N].properties.class.enum = ["<class>"]
	//
	// Schemas may have 2 or 3 allOf entries. We iterate all entries to be robust
	// against depth variations.
	// classEnumFromRaw extracts the single string from an enum array that may contain
	// booleans — e.g. `{"enum": ["button"]}` → "button"; `{"enum": [true, false]}` → "".
	classEnumFromRaw := func(raw json.RawMessage) string {
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) != nil || len(arr) != 1 {
			return ""
		}
		var s string
		if json.Unmarshal(arr[0], &s) != nil {
			return ""
		}
		return s
	}

	for _, schemaRaw := range swagger.Components.Schemas {
		// Decode only the top-level allOf list as raw messages to avoid type
		// conflicts (some properties carry bool enums, not string enums).
		var s struct {
			AllOf []json.RawMessage `json:"allOf"`
		}
		if json.Unmarshal(schemaRaw, &s) != nil {
			continue
		}
		for _, aoRaw := range s.AllOf {
			// Decode each allOf entry's properties map as raw values.
			var ao struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if json.Unmarshal(aoRaw, &ao) != nil {
				continue
			}
			classRaw, ok := ao.Properties["class"]
			if !ok {
				continue
			}
			// Pattern A: class.allOf[N] where allOf[N].enum = ["<class>"]
			var classDef struct {
				AllOf []json.RawMessage `json:"allOf"`
				Enum  json.RawMessage   `json:"enum"`
			}
			if json.Unmarshal(classRaw, &classDef) != nil {
				continue
			}
			for _, innerRaw := range classDef.AllOf {
				var inner struct {
					Enum json.RawMessage `json:"enum"`
				}
				if json.Unmarshal(innerRaw, &inner) == nil {
					if v := classEnumFromRaw(inner.Enum); v != "" {
						r.Classes[v] = true
					}
				}
			}
			// Pattern B: class.enum = ["<class>"] (direct, e.g. Table-default)
			if v := classEnumFromRaw(classDef.Enum); v != "" {
				r.Classes[v] = true
			}
		}
	}

	// `row` and `draggable` are client-side layout wrappers used by the renderer
	// (control-cdu) but defined outside the server-side swagger spec.
	r.Classes["row"] = true
	r.Classes["draggable"] = true

	return r
}
