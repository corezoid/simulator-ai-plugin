package cduschema

import "encoding/json"

// propResolver walks the bundled swagger, following $ref and flattening allOf, to
// collect the property names a component schema declares. It exists because the
// swagger composes component schemas through several layers of allOf/$ref, so the
// declared surface of a class is not readable from any single schema node.
//
// Two swagger quirks shape this code:
//
//   - $ref paths address array elements by index (".../allOf/0/properties/class"),
//     so the walker has to index lists as well as maps.
//   - The same class is split across type variants (Edit-default, Edit-text,
//     Edit-date, …) and each variant redeclares only part of the surface. A key
//     legal on one variant is therefore absent from another, so the only safe
//     allowlist for a class is the UNION over every schema that claims it.
//     Checking against a single variant produces false positives — e.g. `value`
//     is declared on Edit-int but not on Edit-default.
type propResolver struct {
	root map[string]any
}

// follow resolves a local JSON pointer such as "#/components/schemas/Label" or
// "#/components/schemas/File/allOf/0", indexing arrays by their numeric segment.
func (p propResolver) follow(ref string) any {
	if len(ref) < 2 || ref[0] != '#' {
		return nil
	}
	var cur any = p.root
	start := 1
	if ref[1] == '/' {
		start = 2
	}
	for _, seg := range splitPath(ref[start:]) {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[seg]
		case []any:
			i, ok := atoi(seg)
			if !ok || i < 0 || i >= len(node) {
				return nil
			}
			cur = node[i]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

// props returns the property names declared by a schema node, flattening allOf and
// following $ref. depth guards against a cyclic $ref chain.
func (p propResolver) props(node any, depth int) map[string]bool {
	out := map[string]bool{}
	if depth > 16 {
		return out
	}
	m, ok := node.(map[string]any)
	if !ok {
		return out
	}
	if ref, ok := m["$ref"].(string); ok {
		return p.props(p.follow(ref), depth+1)
	}
	if allOf, ok := m["allOf"].([]any); ok {
		for _, sub := range allOf {
			for k := range p.props(sub, depth+1) {
				out[k] = true
			}
		}
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for k := range props {
			out[k] = true
		}
	}
	return out
}

// child returns a named sub-schema of node (e.g. the "extra" property's schema),
// resolving node through $ref/allOf first.
func (p propResolver) child(node any, name string, depth int) any {
	if depth > 16 {
		return nil
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := m["$ref"].(string); ok {
		return p.child(p.follow(ref), name, depth+1)
	}
	if props, ok := m["properties"].(map[string]any); ok {
		if c, ok := props[name]; ok {
			return c
		}
	}
	if allOf, ok := m["allOf"].([]any); ok {
		for _, sub := range allOf {
			if c := p.child(sub, name, depth+1); c != nil {
				return c
			}
		}
	}
	return nil
}

// itemsOf returns the "items" sub-schema of an array schema, following $ref/allOf.
func (p propResolver) itemsOf(node any, depth int) any {
	if depth > 16 {
		return nil
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := m["$ref"].(string); ok {
		return p.itemsOf(p.follow(ref), depth+1)
	}
	if it, ok := m["items"]; ok {
		return it
	}
	if allOf, ok := m["allOf"].([]any); ok {
		for _, sub := range allOf {
			if it := p.itemsOf(sub, depth+1); it != nil {
				return it
			}
		}
	}
	return nil
}

// classOf returns the single-value class enum a component schema pins, or "".
func (p propResolver) classOf(schema any, depth int) string {
	c := p.child(schema, "class", depth)
	return p.singleEnum(c, 0)
}

func (p propResolver) singleEnum(node any, depth int) string {
	if depth > 12 {
		return ""
	}
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	if ref, ok := m["$ref"].(string); ok {
		return p.singleEnum(p.follow(ref), depth+1)
	}
	if enum, ok := m["enum"].([]any); ok && len(enum) == 1 {
		if s, ok := enum[0].(string); ok {
			return s
		}
	}
	if allOf, ok := m["allOf"].([]any); ok {
		for _, sub := range allOf {
			if v := p.singleEnum(sub, depth+1); v != "" {
				return v
			}
		}
	}
	return ""
}

func splitPath(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// decodeSwagger parses the embedded swagger into a generic map once.
func decodeSwagger(data []byte) map[string]any {
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	return root
}
