package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// This file is the shared drawing engine the point-based and categorical forms
// mount on: nice ticks, scales, the plot-rectangle layout, and a canvas that
// emits a self-contained themed SVG. Nothing here is exported — the forms
// (line.go, scatter.go, bar.go, ...) are the surface; this is the machinery they
// share so the axis, grid, legend, and dark-mode styling are drawn one way.

// ---------------------------------------------------------------------------
// Nice ticks — the 1-2-5 algorithm
// ---------------------------------------------------------------------------

// niceScale expands [lo, hi] to rounded bounds and returns the tick positions
// inside them, using the 1-2-5 rule: a human-friendly step is the power of ten
// times 1, 2, or 5 nearest to span/want. This is what makes an axis read
// 0 / 20 / 40 / 60 instead of 0 / 17.3 / 34.6. want is the desired tick count; the
// result usually lands within one of it.
func niceScale(lo, hi float64, want int) (nlo, nhi float64, ticks []float64) {
	if want < 2 {
		want = 2
	}
	if !(hi > lo) {
		// Degenerate range (all points equal, or a single point): open a unit
		// window around the value so the mark isn't pinned to an edge.
		if lo == 0 {
			lo, hi = -1, 1
		} else {
			pad := math.Abs(lo) * 0.5
			lo, hi = lo-pad, hi+pad
		}
	}
	step := niceStep((hi - lo) / float64(want-1))
	nlo = math.Floor(lo/step) * step
	nhi = math.Ceil(hi/step) * step
	// Guard against float drift adding a phantom final tick.
	for v := nlo; v <= nhi+step*1e-9; v += step {
		ticks = append(ticks, snap(v, step))
	}
	return nlo, nhi, ticks
}

// niceStep rounds a raw step up to the nearest 1, 2, or 5 times a power of ten.
func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsInf(raw, 0) || math.IsNaN(raw) {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch norm := raw / mag; {
	case norm < 1.5:
		return 1 * mag
	case norm < 3:
		return 2 * mag
	case norm < 7:
		return 5 * mag
	default:
		return 10 * mag
	}
}

// snap removes floating-point crumbs (0.30000000000000004) by rounding a tick to
// the precision implied by its step.
func snap(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	return math.Round(v/step) * step
}

// logTicks returns the powers of ten spanning [lo, hi] (both must be positive),
// e.g. 1, 10, 100, 1000. Log axes carry decade ticks, not 1-2-5 steps.
func logTicks(lo, hi float64) (nlo, nhi float64, ticks []float64) {
	if lo <= 0 {
		lo = math.SmallestNonzeroFloat64
	}
	loE := math.Floor(math.Log10(lo))
	hiE := math.Ceil(math.Log10(hi))
	if hiE <= loE {
		hiE = loE + 1
	}
	for e := loE; e <= hiE+1e-9; e++ {
		ticks = append(ticks, math.Pow(10, e))
	}
	return math.Pow(10, loE), math.Pow(10, hiE), ticks
}

// ---------------------------------------------------------------------------
// Scale — data value to pixel
// ---------------------------------------------------------------------------

// scale maps a data value onto a pixel range [p0, p1]. For the y-axis, callers
// pass p0 as the bottom (larger px) and p1 as the top so up means larger value.
type scale struct {
	lo, hi float64
	p0, p1 float64
	log    bool
}

func (s scale) at(v float64) float64 {
	var t float64
	if s.log {
		lo, hi := math.Log10(s.lo), math.Log10(s.hi)
		if v <= 0 {
			v = s.lo
		}
		t = (math.Log10(v) - lo) / (hi - lo)
	} else {
		t = (v - s.lo) / (s.hi - s.lo)
	}
	return s.p0 + t*(s.p1-s.p0)
}

// ---------------------------------------------------------------------------
// Text width — no font metrics, so estimate
// ---------------------------------------------------------------------------

// textW estimates the rendered width of a system-sans string at a given px size.
// SVG in the browser has no measurement API available to pure Go, so we assume an
// average glyph advance of ~0.58em (a reasonable mean for system-ui digits and
// lowercase). It is used only for layout decisions — reserving the left margin for
// y-tick labels, and detecting when direct end-labels would collide — where a
// small over- or under-estimate costs a pixel of padding, never a clipped glyph.
func textW(s string, px float64) float64 {
	// Digits and separators are narrower than the 0.58 average; most tick and
	// axis labels are numeric, so bias slightly narrow to avoid over-reserving.
	return float64(len([]rune(s))) * px * 0.56
}

// ---------------------------------------------------------------------------
// Number formatting
// ---------------------------------------------------------------------------

// fmtNum formats an axis/label number cleanly: thousands-separated integers, and
// short decimals with trailing zeros trimmed. It is the one place tick and label
// text is produced, so every axis reads consistently.
func fmtNum(v float64) string {
	if v == 0 {
		return "0"
	}
	abs := math.Abs(v)
	// Large magnitudes: comma-group the integer part (0 / 1,000 / 20,000).
	if abs >= 1000 && v == math.Trunc(v) {
		return group(strconv.FormatInt(int64(v), 10))
	}
	// Otherwise pick a decimal precision from the magnitude, then trim.
	prec := 0
	switch {
	case abs < 0.1:
		prec = 3
	case abs < 1:
		prec = 2
	case abs < 100:
		prec = 2
	default:
		prec = 0
	}
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// group inserts thousands separators into a plain integer string (with optional
// leading minus).
func group(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	n := len(s)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if n > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Canvas — a self-contained themed SVG
// ---------------------------------------------------------------------------

// canvas accumulates SVG markup and, on finish, wraps it with a viewBox and an
// embedded <style> block carrying the palette as CSS custom properties. Because a
// chart is injected into the page via innerHTML with no ambient theme, the style
// block is what makes the same SVG legible on the host's light surface today and
// a dark one under prefers-color-scheme — the drawing code references roles
// (var(--grid), var(--ink)) and never a raw light/dark hex.
type canvas struct {
	w, h int
	b    strings.Builder
	// classSeq numbers the series color classes actually used, so the style block
	// only defines the slots this chart references.
	series int
}

func newCanvas(w, h int) *canvas { return &canvas{w: w, h: h} }

// use registers that series slot i is drawn, returning the CSS class name whose
// fill/stroke resolves to that slot's themed hue.
func (c *canvas) use(i int) string {
	if i+1 > c.series {
		c.series = i + 1
	}
	return "s" + strconv.Itoa(i)
}

// raw appends preformatted SVG markup.
func (c *canvas) raw(s string) { c.b.WriteString(s) }

// rawf appends formatted SVG markup.
func (c *canvas) rawf(format string, a ...any) { fmt.Fprintf(&c.b, format, a...) }

// finish returns the complete SVG document string.
func (c *canvas) finish() string {
	var out strings.Builder
	fmt.Fprintf(&out,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
			`width="100%%" style="max-width:%dpx;height:auto;font-family:system-ui,-apple-system,'Segoe UI',sans-serif" `+
			`role="img">`,
		c.w, c.h, c.w)
	out.WriteString(c.style())
	fmt.Fprintf(&out, `<rect width="%d" height="%d" fill="var(--surface)"/>`, c.w, c.h)
	out.WriteString(c.b.String())
	out.WriteString(`</svg>`)
	return out.String()
}

// style emits the scoped stylesheet: chrome tokens plus the series slots in use,
// each defined once for light and re-declared under a prefers-color-scheme:dark
// media query. Scoped to this SVG's own class so two charts on a page don't leak
// variables into each other.
func (c *canvas) style() string {
	var b strings.Builder
	b.WriteString(`<style>`)
	b.WriteString(`svg{--surface:` + surfaceLight + `;--ink:` + inkLight +
		`;--secondary:` + secondaryLight + `;--muted:` + mutedLight +
		`;--grid:` + gridLight + `;--axis:` + axisLight + `}`)
	for i := 0; i < c.series; i++ {
		h := seriesHue(i)
		fmt.Fprintf(&b, `svg{--c%d:%s}`, i, h.light)
	}
	// Dark surface.
	b.WriteString(`@media (prefers-color-scheme:dark){`)
	b.WriteString(`svg{--surface:` + surfaceDark + `;--ink:` + inkDark +
		`;--secondary:` + secondaryDark + `;--muted:` + mutedDark +
		`;--grid:` + gridDark + `;--axis:` + axisDark + `}`)
	for i := 0; i < c.series; i++ {
		h := seriesHue(i)
		fmt.Fprintf(&b, `svg{--c%d:%s}`, i, h.dark)
	}
	b.WriteString(`}`)
	// Series class → the CSS `color` property only, so each mark chooses
	// stroke/fill via currentColor through a presentation attribute. Setting
	// stroke/fill in the stylesheet would override a mark's own fill="none"
	// (a stylesheet beats an SVG presentation attribute), turning lines into
	// filled blobs — color+currentColor sidesteps that entirely.
	for i := 0; i < c.series; i++ {
		fmt.Fprintf(&b, `.s%d{color:var(--c%d)}`, i, i)
	}
	b.WriteString(`</style>`)
	return b.String()
}

// esc escapes text for safe inclusion in SVG text nodes / attributes. Series and
// axis labels come from the notebook author (trusted-ish) but may contain <, &, or
// quotes, which would corrupt the markup; escape defensively.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
