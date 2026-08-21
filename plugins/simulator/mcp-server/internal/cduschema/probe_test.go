package cduschema

import "testing"

// Every key `docs/user-flows/cdu-page-protocol.md` presents as real must survive
// into the derived union, because ValidateFile rejects — and so aborts a push on —
// anything outside it. The first cut of this test listed ten classes and missed
// exactly the ones the swagger under-describes (mainMenu, carousel, comments,
// timer, file, upload, attachment), so the table below now mirrors §5 row for row.
// A class the swagger never described has no rule at all and is skipped by the
// check; those are asserted separately at the bottom.

// §4 "Item — the component envelope" — the renderer's baseSchema, shared by every class.
var documentedBaseFields = []string{
	"id", "class", "value", "visibility", "required", "error", "errorMsg",
	"styleClass", "row", "w", "submitOnChange", "extra",
}

// §5 "Components" — the "Key fields (beyond base)" column, one entry per row.
var documentedItemKeys = map[string][]string{
	"button":      {"title", "type", "tooltip", "extra"},
	"edit":        {"value", "type", "placeholder", "regexp", "mask", "errorMsg", "helpMsg", "submitOnEnter", "resettable", "extra"},
	"select":      {"value", "options", "type", "submitOnChange", "submitOnScroll"},
	"multiselect": {"value", "options", "extra"},
	"radio":       {"value", "options", "extra"},
	"check":       {"value", "required"},
	"toggle":      {"value", "title"},
	"slider":      {"value", "extra"},
	"phone":       {"value", "options", "regexp", "required"},
	"otp":         {"value", "type", "extra"},
	"label":       {"value", "align", "tooltip"},
	"divider":     {},
	"image":       {"value", "extra"},
	"copy":        {"value", "title"},
	"file":        {"value", "extra"},
	"upload":      {"value", "type", "extra"},
	"attachment":  {"value", "extra"},
	"signature":   {"value", "extra"},
	"carousel":    {"items", "value", "extra"},
	"table":       {"head", "body", "value", "type", "submitOnChange", "submitOnScroll"},
	"tab":         {"options", "value", "submitOnChange"},
	"stepper":     {"options", "value", "extra"},
	"mainMenu":    {"options", "value"},
	"comments":    {"value", "title"},
	"timer":       {"value", "extra"},
	"widget":      {"type", "extra"},
}

// §5 — the `extra.{…}` sets the table spells out, for the nested spots where a
// rule was actually derived. A spot with no derived rule is unchecked, so it
// cannot produce a false positive and is not asserted here.
var documentedExtraKeys = map[string][]string{
	"button.extra":      {"url", "target", "action", "icon", "rounded", "mobileVisible", "request", "autoSubmit", "options"},
	"edit.extra":        {"length", "lineNumbers"},
	"multiselect.extra": {"length"},
	"radio.extra":       {"direction"},
	"slider.extra":      {"min", "max", "step"},
	"otp.extra":         {"length"},
	"image.extra":       {"alt"},
	"file.extra":        {"downloadUrl", "uploadUrl", "auth"},
	"upload.extra":      {"accept", "minSize", "maxSize", "compression"},
	"attachment.extra":  {"downloadUrl"},
	"signature.extra":   {"strokeStyle", "saveButtonTitle"},
	"stepper.extra":     {"mobileVisible"},
	"timer.extra":       {"duration"},
}

func TestProbeDocumentedKeysPresentInUnion(t *testing.T) {
	r := GetRules()
	for class, keys := range documentedItemKeys {
		allowed := r.ItemProps[class]
		if len(allowed) == 0 {
			t.Errorf("%s: no ItemProps derived — the key check silently does nothing for this class", class)
			continue
		}
		for _, k := range append(append([]string{}, documentedBaseFields...), keys...) {
			if !allowed[k] {
				t.Errorf("%s: documented key %q missing from the union — a push using it would be rejected", class, k)
			}
		}
	}
}

func TestProbeDocumentedExtraKeysPresentInUnion(t *testing.T) {
	r := GetRules()
	for spot, keys := range documentedExtraKeys {
		allowed := r.NestedProps[spot]
		if len(allowed) == 0 {
			continue // no derived rule: the nested check is off for this spot
		}
		for _, k := range keys {
			if !allowed[k] {
				t.Errorf("%s: documented key %q missing — a push using it would be rejected", spot, k)
			}
		}
	}
}

// The layout wrappers are absent from the swagger on purpose (§5). They must stay
// rule-less: deriving a partial rule would reject the `items[]` the docs show.
func TestProbeLayoutWrappersHaveNoKeyRule(t *testing.T) {
	r := GetRules()
	for _, class := range []string{"row", "draggable"} {
		if len(r.ItemProps[class]) > 0 {
			t.Errorf("%s: unexpected key rule %v — the swagger does not describe this class", class, sortedKeys(r.ItemProps[class]))
		}
	}
}
