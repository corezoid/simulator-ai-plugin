package mcpserver

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// looksLikeSVG returns true when the bytes appear to start with an SVG payload.
// We rely on this in addition to the filename extension because some upstreams
// serve SVGs with the wrong filename (e.g. `slack` with no extension).
func looksLikeSVG(b []byte) bool {
	// Skip BOM / leading whitespace.
	head := bytes.TrimLeft(b, " \t\r\n")
	if len(head) > 256 {
		head = head[:256]
	}
	s := strings.ToLower(string(head))
	return strings.Contains(s, "<svg") || strings.Contains(s, "<!doctype svg") ||
		strings.Contains(s, "<?xml") && strings.Contains(s, "svg")
}

// svgInjectFill inserts a `fill="<color>"` attribute on the first <svg ...> tag.
// Used to paint monochrome simpleicons (which inherit colour from CSS) in
// a brand colour before rasterising. No-op when the SVG already declares fill.
var svgOpenRe = regexp.MustCompile(`(?is)<svg\b([^>]*)>`)

func svgInjectFill(svg []byte, color string) []byte {
	if color == "" {
		return svg
	}
	return svgOpenRe.ReplaceAllFunc(svg, func(m []byte) []byte {
		// If a fill attribute is already present, leave it untouched.
		if regexp.MustCompile(`(?i)\sfill\s*=`).Match(m) {
			return m
		}
		return []byte(strings.Replace(string(m), "<svg", `<svg fill="`+color+`"`, 1))
	})
}

// rasterizeSVG renders an SVG to a PNG of the requested size using pure-Go
// oksvg + rasterx. Returns the PNG bytes.
func rasterizeSVG(svg []byte, width, height int) ([]byte, error) {
	if width <= 0 {
		width = 256
	}
	if height <= 0 {
		height = 256
	}

	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg), oksvg.StrictErrorMode)
	if err != nil {
		// Retry with the more lenient mode — many simpleicons fail strict.
		icon, err = oksvg.ReadIconStream(bytes.NewReader(svg), oksvg.IgnoreErrorMode)
		if err != nil {
			return nil, fmt.Errorf("parse svg: %w", err)
		}
	}
	icon.SetTarget(0, 0, float64(width), float64(height))

	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	scanner := rasterx.NewScannerGV(width, height, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(width, height, scanner)
	icon.Draw(raster, 1.0)

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// maybeConvertSVG inspects the file bytes and, if they appear to be SVG,
// rasterises to PNG and returns the new bytes + filename + content type.
// Otherwise returns the inputs unchanged.
//
// width / height default to 256×256. fillColor, when non-empty, is injected
// onto the root <svg> tag before rasterising (helps monochrome simpleicons).
func maybeConvertSVG(
	fileBytes []byte, filename string,
	width, height int, fillColor string,
) ([]byte, string, string, error) {
	ext := strings.ToLower(filename)
	isSVG := strings.HasSuffix(ext, ".svg") || looksLikeSVG(fileBytes)
	if !isSVG {
		return fileBytes, filename, guessContentType(filename), nil
	}
	if fillColor != "" {
		fileBytes = svgInjectFill(fileBytes, fillColor)
	}
	png, err := rasterizeSVG(fileBytes, width, height)
	if err != nil {
		return nil, "", "", err
	}
	// Replace .svg with .png in the filename; if no extension, append .png.
	newName := strings.TrimSuffix(filename, ".svg")
	newName = strings.TrimSuffix(newName, ".SVG")
	if !strings.HasSuffix(strings.ToLower(newName), ".png") {
		newName += ".png"
	}
	return png, newName, "image/png", nil
}
