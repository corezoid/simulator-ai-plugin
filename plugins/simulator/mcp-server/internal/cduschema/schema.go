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

	// ItemProps maps an item `class` to the union of property names every schema
	// variant of that class declares, plus the shared base-item properties.
	// The union is deliberate: see propResolver's doc comment — checking against a
	// single variant reports legal keys as unknown.
	ItemProps map[string]map[string]bool

	// NestedProps maps "<class>.<field>" to the property names allowed inside that
	// nested structure — "image.extra", "table.head" and "table.body" items,
	// "<class>.options" items. These are the spots where the swagger IS precise and
	// where the client renderer rejects extra keys even though the server accepts
	// them (no schema sets additionalProperties:false).
	NestedProps map[string]map[string]bool
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
		GridType:    map[string]bool{"one_column": true, "two_column": true},
		ItemProps:   make(map[string]map[string]bool),
		NestedProps: make(map[string]map[string]bool),
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

	collectProps(data, r)

	return r
}

// collectProps fills r.ItemProps and r.NestedProps by walking every component
// schema in the swagger. Failure is silent and leaves the maps empty, which makes
// the dependent checks skip rather than reject — a missing rule must never block a
// push.
func collectProps(data []byte, r *Rules) {
	root := decodeSwagger(data)
	if root == nil {
		return
	}
	comps, _ := root["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	if schemas == nil {
		return
	}
	p := propResolver{root: root}

	// Every item inherits this base (id, visibility, row, w, styleClass).
	base := map[string]bool{}
	if f, ok := schemas["File"]; ok {
		if allOf, ok := f.(map[string]any)["allOf"].([]any); ok && len(allOf) > 0 {
			base = p.props(allOf[0], 0)
		}
	}

	add := func(m map[string]map[string]bool, key string, names map[string]bool) {
		if len(names) == 0 {
			return
		}
		if m[key] == nil {
			m[key] = map[string]bool{}
		}
		for n := range names {
			m[key][n] = true
		}
	}

	for _, schema := range schemas {
		class := p.classOf(schema, 0)
		if class == "" {
			continue
		}
		add(r.ItemProps, class, p.props(schema, 0))
		add(r.ItemProps, class, base)

		if extra := p.child(schema, "extra", 0); extra != nil {
			add(r.NestedProps, class+".extra", p.props(extra, 0))
		}
		if options := p.child(schema, "options", 0); options != nil {
			if items := p.itemsOf(options, 0); items != nil {
				add(r.NestedProps, class+".options", p.props(items, 0))
			}
		}
		for _, field := range []string{"head", "body"} {
			if f := p.child(schema, field, 0); f != nil {
				if items := p.itemsOf(f, 0); items != nil {
					add(r.NestedProps, class+"."+field, p.props(items, 0))
				}
			}
		}
	}

	applySupplements(r, add)
}

// applySupplements widens the swagger-derived allowlists with the fields the
// renderer accepts but the bundled swagger omits. Without it the derived union is
// NARROWER than the protocol this repo documents, and a push carrying a
// documented shape (`mainMenu.options`, `carousel.items`, `file.extra.downloadUrl`,
// …) is rejected outright. Provenance for every entry is
// `docs/user-flows/cdu-page-protocol.md` §4 (base fields) and §5 (per-class table);
// `TestProbeDocumentedKeysPresentInUnion` walks that same table and fails if any
// documented key falls out of the union again.
//
// Two deliberate limits:
//
//   - a class the swagger never described keeps NO rule (the check stays off for
//     it) — supplements only widen a union that already exists, they never switch
//     a check on;
//   - the same holds for a nested spot: `carousel.extra` has no derived rule, so
//     it is left unchecked rather than pinned to the two keys the docs name.
func applySupplements(r *Rules, add func(map[string]map[string]bool, string, map[string]bool)) {
	// §4 "Item — the component envelope": the renderer's baseSchema. The swagger's
	// File.allOf[0] carries only half of it (id, visibility, row, w, styleClass).
	baseFields := setOf("id", "class", "value", "visibility", "required", "error",
		"errorMsg", "styleClass", "row", "w", "submitOnChange", "extra")

	// §5, per class — key fields the swagger's schema for that class does not declare.
	itemSupplement := map[string]map[string]bool{
		"carousel": setOf("items"),   // §5: `items[]`, `value` (index)
		"mainMenu": setOf("options"), // §5: `options[]`, `value`
		"comments": setOf("title"),   // §5: `value`, `title`
	}

	// §5, per nested spot — only widens a rule the swagger already produced.
	nestedSupplement := map[string]map[string]bool{
		"timer.extra":      setOf("duration"),                         // §5: extra.duration
		"file.extra":       setOf("downloadUrl", "uploadUrl", "auth"), // §5: extra.{downloadUrl,uploadUrl,auth}
		"upload.extra":     setOf("compression"),                      // §5: extra.{…,compression}
		"attachment.extra": setOf("downloadUrl"),                      // §5: extra.downloadUrl
		"edit.extra":       setOf("lineNumbers"),                      // §5: extra.{length,lineNumbers}
	}

	for class := range r.ItemProps {
		add(r.ItemProps, class, baseFields)
		if extra, ok := itemSupplement[class]; ok {
			add(r.ItemProps, class, extra)
		}
	}
	for key, extra := range nestedSupplement {
		if len(r.NestedProps[key]) == 0 {
			continue // no derived rule — the check is off, keep it off
		}
		add(r.NestedProps, key, extra)
	}
}

func setOf(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}
