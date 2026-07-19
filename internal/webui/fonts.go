package webui

import (
	_ "embed"
	"encoding/base64"
	"strings"
)

// Atkinson Hyperlegible is the project's typeface (Braille Institute, SIL OFL —
// see fonts/OFL.txt). The woff2 files are embedded and emitted as base64
// data-URIs in the page's CSS, so EVERY notebook page — served over SSE, built
// to a wasm host directory, or opened straight off disk — carries its own fonts
// with no CDN and no extra files to copy. That is the whole point: one page, no
// network dependency, matching the "static artifact, runs offline" identity.
//
// embed is stdlib and pulls in nothing (no net/os), so it does not perturb the
// WASM-ability analysis of any notebook.
var (
	//go:embed fonts/atkinson-regular.woff2
	fontRegular []byte
	//go:embed fonts/atkinson-bold.woff2
	fontBold []byte
	//go:embed fonts/atkinson-italic.woff2
	fontItalic []byte
	//go:embed fonts/atkinson-mono.woff2
	fontMono []byte
)

// fontFaceCSS returns the @font-face rules with each woff2 inlined as a base64
// data-URI. Computed once, prepended to the stylesheet by Page.
func fontFaceCSS() string {
	face := func(family, style, weight string, data []byte) string {
		return "@font-face{font-family:\"" + family + "\";font-style:" + style +
			";font-weight:" + weight + ";font-display:swap;src:url(data:font/woff2;base64," +
			base64.StdEncoding.EncodeToString(data) + ") format(\"woff2\")}"
	}
	var b strings.Builder
	b.WriteString(face("Atkinson Hyperlegible", "normal", "400", fontRegular))
	b.WriteString("\n")
	b.WriteString(face("Atkinson Hyperlegible", "normal", "700", fontBold))
	b.WriteString("\n")
	b.WriteString(face("Atkinson Hyperlegible", "italic", "400", fontItalic))
	b.WriteString("\n")
	// The mono is a variable font; one file covers the 400–700 range.
	b.WriteString(face("Atkinson Hyperlegible Mono", "normal", "400 700", fontMono))
	b.WriteString("\n")
	return b.String()
}
