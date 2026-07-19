package chart

// The palette and chart chrome. These values are not eyeballed: the eight
// categorical hues are the dataviz method's validated order, checked with its
// validate_palette.js on both surfaces —
//
//	light (#fcfcfb): worst adjacent CVD ΔE 9.1, normal-vision ΔE 19.6 → PASS
//	dark  (#1a1a19): worst adjacent CVD ΔE 8.4, normal-vision ΔE 19.3 → PASS
//
// The slot ORDER is the colorblind-safety mechanism (it maximizes the minimum
// adjacent ΔE), so assign in this fixed order and never cycle: a ninth series
// folds to "Other" rather than reusing slot 1. On the light surface three hues
// (magenta, yellow, aqua) sit below 3:1 contrast — the method's "relief rule"
// covers that here because every series also carries a legend key and a direct
// end-label, so identity never rides on the fill color alone.
//
// A chart injected via innerHTML carries no ambient theme, so each rendered SVG
// embeds its own <style> with a prefers-color-scheme media query; these are the
// values that style block interpolates.

// series holds the light and dark hex for one categorical slot.
type slot struct{ light, dark string }

// palette is the eight categorical slots, in validated order:
// blue, green, magenta, yellow, aqua, orange, violet, red.
var palette = [8]slot{
	{"#2a78d6", "#3987e5"}, // 1 blue
	{"#008300", "#008300"}, // 2 green
	{"#e87ba4", "#d55181"}, // 3 magenta
	{"#eda100", "#c98500"}, // 4 yellow
	{"#1baf7a", "#199e70"}, // 5 aqua
	{"#eb6834", "#d95926"}, // 6 orange
	{"#4a3aa7", "#9085e9"}, // 7 violet
	{"#e34948", "#e66767"}, // 8 red
}

// otherHue is slot 9+ ("Other") — a muted gray, never a recycled categorical hue.
var otherHue = slot{"#898781", "#898781"}

// seriesHue returns the light/dark hex for series index i, folding the tail past
// eight slots to the "Other" gray.
func seriesHue(i int) slot {
	if i < len(palette) {
		return palette[i]
	}
	return otherHue
}

// Chart chrome & ink — the grays the data sits on. One shade off the surface for
// the grid; text in three weights. These are the dataviz method's chrome tokens.
const (
	surfaceLight = "#fcfcfb"
	surfaceDark  = "#1a1a19"

	inkLight = "#0b0b0b" // primary text
	inkDark  = "#ffffff"

	secondaryLight = "#52514e" // axis titles, legend text
	secondaryDark  = "#c3c2b7"

	mutedLight = "#898781" // tick labels — same on both surfaces
	mutedDark  = "#898781"

	gridLight = "#e1e0d9" // hairline gridlines
	gridDark  = "#2c2c2a"

	axisLight = "#c3c2b7" // baseline / axis rule
	axisDark  = "#383835"
)
