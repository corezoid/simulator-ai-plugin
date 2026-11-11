package osint

import (
	"fmt"
	"hash/fnv"
)

// avatarPalette are background colours; an entity always gets the same one.
var avatarPalette = []string{"#2563eb", "#7c3aed", "#db2777", "#dc2626", "#ea580c", "#ca8a04", "#16a34a", "#0d9488", "#0891b2", "#475569"}

// avatarVersion changes when the drawings do, so that a build replaces the
// pictures made by an older version.
const avatarVersion = "v1"

func avatarColor(id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return avatarPalette[int(h.Sum32()%uint32(len(avatarPalette)))]
}

// avatarKey names the generated picture of an entity; it is stored on the
// actor so that a build can tell the picture is already there.
func avatarKey(entityType, id string) string {
	return fmt.Sprintf("avatar:%s:%s:%s", avatarVersion, entityType, avatarColor(id))
}

// avatarSVG draws a placeholder picture for an entity that has none: a white
// glyph for its type on a colour of its own. It is a drawing, not a likeness —
// a generated case has no real people to show.
func avatarSVG(entityType, id string) []byte {
	bg := avatarColor(id)
	var glyph string
	switch entityType {
	case "person":
		glyph = `<circle cx="128" cy="98" r="46" fill="#fff"/>` +
			`<path d="M40 256 C40 190 80 160 128 160 C176 160 216 190 216 256 Z" fill="#fff"/>`
	case "organization":
		glyph = `<rect x="66" y="56" width="124" height="160" fill="#fff"/>` +
			windows(bg, 84, 76, 3, 4) +
			`<rect x="112" y="184" width="32" height="32" fill="` + bg + `"/>`
	case "location":
		glyph = `<path d="M128 228 L72 128 C56 92 84 44 128 44 C172 44 200 92 184 128 Z" fill="#fff"/>` +
			`<circle cx="128" cy="104" r="26" fill="` + bg + `"/>`
	case "asset":
		glyph = `<path d="M128 40 L216 104 L128 220 L40 104 Z" fill="#fff"/>` +
			`<path d="M128 76 L176 106 L128 172 L80 106 Z" fill="` + bg + `"/>`
	case "document":
		glyph = `<path d="M72 40 L156 40 L188 72 L188 216 L72 216 Z" fill="#fff"/>` +
			`<rect x="92" y="96" width="76" height="12" fill="` + bg + `"/>` +
			`<rect x="92" y="124" width="76" height="12" fill="` + bg + `"/>` +
			`<rect x="92" y="152" width="52" height="12" fill="` + bg + `"/>`
	case "digital":
		glyph = `<circle cx="128" cy="128" r="84" fill="#fff"/>` +
			`<circle cx="128" cy="128" r="60" fill="` + bg + `"/>` +
			`<circle cx="128" cy="128" r="30" fill="#fff"/>`
	default:
		glyph = `<circle cx="128" cy="128" r="64" fill="#fff"/>`
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">` +
		`<rect x="0" y="0" width="256" height="256" fill="` + bg + `"/>` + glyph + `</svg>`)
}

// windows draws a grid of squares in the building glyph.
func windows(color string, x, y, cols, rows int) string {
	out := ""
	for r := range rows {
		for c := range cols {
			out += fmt.Sprintf(`<rect x="%d" y="%d" width="20" height="16" fill="%s"/>`, x+c*34, y+r*26, color)
		}
	}
	return out
}
