package sim

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OMap is an insertion-ordered string-keyed map: YAML mappings keep their order
// (action fields are evaluated in the order they are written).
type OMap struct {
	keys []string
	m    map[string]any
}

func NewOMap() *OMap { return &OMap{m: map[string]any{}} }

func (o *OMap) Get(k string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.m[k]
	return v, ok
}

func (o *OMap) Has(k string) bool { _, ok := o.Get(k); return ok }

func (o *OMap) Set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *OMap) Delete(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *OMap) Keys() []string {
	if o == nil {
		return nil
	}
	return append([]string(nil), o.keys...)
}

func (o *OMap) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// deepCopy copies maps and lists; numbers and strings are immutable.
func deepCopy(v any) any {
	switch x := v.(type) {
	case *OMap:
		if x == nil {
			return (*OMap)(nil)
		}
		c := NewOMap()
		for _, k := range x.keys {
			c.Set(k, deepCopy(x.m[k]))
		}
		return c
	case []any:
		c := make([]any, len(x))
		for i, e := range x {
			c[i] = deepCopy(e)
		}
		return c
	}
	return v
}

// ---- YAML ----------------------------------------------------------

// fromYAML converts a YAML node into engine values: mappings -> *OMap, sequences -> []any,
// numbers -> *big.Rat (exact, from the literal text), booleans, null, strings.
//
// Aliases are expanded here, not by yaml.v3, so its alias-bomb guard does not apply: the
// expansion is capped at the document's own size plus maxAliasNodes.
func fromYAML(n *yaml.Node) (any, error) {
	c := &yamlConv{budget: yamlSize(n) + maxAliasNodes}
	return c.conv(n)
}

const maxAliasNodes = 10_000

type yamlConv struct{ budget int }

// yamlSize counts the nodes of a document without following aliases.
func yamlSize(n *yaml.Node) int {
	size := 1
	for _, c := range n.Content {
		size += yamlSize(c)
	}
	return size
}

func (c *yamlConv) conv(n *yaml.Node) (any, error) {
	if c.budget--; c.budget < 0 {
		return nil, fmt.Errorf("line %d: YAML aliases expand to too many nodes", n.Line)
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return c.conv(n.Content[0])
	case yaml.AliasNode:
		return c.conv(n.Alias)
	case yaml.MappingNode:
		o := NewOMap()
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Tag == "!!merge" {
				return nil, fmt.Errorf("line %d: YAML merge keys are not supported", k.Line)
			}
			val, err := c.conv(v)
			if err != nil {
				return nil, err
			}
			o.Set(k.Value, val)
		}
		return o, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			val, err := c.conv(item)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!null":
			return nil, nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err != nil {
				return nil, err
			}
			return b, nil
		case "!!int", "!!float":
			text := strings.ReplaceAll(n.Value, "_", "")
			if r, ok := parseNum(text); ok {
				return r, nil
			}
			return nil, fmt.Errorf("line %d: unsupported number %q", n.Line, n.Value)
		default:
			return n.Value, nil
		}
	}
	return nil, fmt.Errorf("line %d: unsupported YAML node", n.Line)
}

func parseYAML(src []byte) (any, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		return nil, err
	}
	if n.Kind == 0 {
		return nil, nil
	}
	return fromYAML(&n)
}

// ---- JSON (tool output, actor state) --------------------------------

// toJSON converts engine values into plain JSON-able values; numbers become strings.
func toJSON(v any) any {
	switch x := v.(type) {
	case *big.Rat:
		return numString(x)
	case *OMap:
		if x == nil {
			return nil
		}
		m := make(map[string]any, len(x.keys))
		for _, k := range x.keys {
			m[k] = toJSON(x.m[k])
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = toJSON(e)
		}
		return out
	case *ActorView:
		return map[string]any{"id": x.id, "title": x.title()}
	}
	return v
}

// yamlNum is a number in a graph file: a plain YAML int or float, so LoadGraph reads it
// back as a number. toJSON would write a string and change what the value means.
type yamlNum struct{ r *big.Rat }

func (n yamlNum) MarshalYAML() (any, error) {
	tag := "!!float"
	if n.r.IsInt() {
		tag = "!!int"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: numString(n.r)}, nil
}

// toYAML is toJSON for graph files: numbers stay numbers.
func toYAML(v any) any {
	switch x := v.(type) {
	case *big.Rat:
		return yamlNum{x}
	case *OMap:
		if x == nil {
			return nil
		}
		m := make(map[string]any, len(x.keys))
		for _, k := range x.keys {
			m[k] = toYAML(x.m[k])
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = toYAML(e)
		}
		return out
	}
	return toJSON(v)
}

// fromJSON converts decoded JSON (UseNumber) into engine values.
func fromJSON(v any) any {
	switch x := v.(type) {
	case json.Number:
		if r, ok := parseNum(x.String()); ok {
			return r
		}
		return x.String()
	case float64:
		r, _ := toNum(x)
		return r
	case map[string]any:
		o := NewOMap()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			o.Set(k, fromJSON(x[k]))
		}
		return o
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = fromJSON(e)
		}
		return out
	}
	return v
}

// ---- generic helpers ---------------------------------------------------

// truthy follows Python truthiness.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case *big.Rat:
		return x.Sign() != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case *OMap:
		return x.Len() > 0
	}
	return true
}

// show renders a value for messages and str().
func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case *big.Rat:
		return numString(x)
	case string:
		return x
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = repr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *OMap:
		parts := make([]string, 0, x.Len())
		for _, k := range x.keys {
			parts = append(parts, fmt.Sprintf("'%s': %s", k, repr(x.m[k])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *ActorView:
		return fmt.Sprintf("<actor %s>", x.label())
	case *Namespace:
		return fmt.Sprintf("<%s>", x.name)
	}
	return fmt.Sprint(v)
}

func repr(v any) string {
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	}
	return show(v)
}

// equal follows Python ==: numbers by value, actors by id, containers element-wise.
func equal(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case *big.Rat:
		y, ok := b.(*big.Rat)
		return ok && x.Cmp(y) == 0
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case *OMap:
		y, ok := b.(*OMap)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.keys {
			yv, has := y.Get(k)
			if !has || !equal(x.m[k], yv) {
				return false
			}
		}
		return true
	case *ActorView:
		y, ok := b.(*ActorView)
		return ok && x.id == y.id
	}
	return a == b
}

func asString(v any) string { return show(v) }
