//go:notebook
//
// The roofline — is your kernel compute-bound or memory-bound?
//
// A processor has two ceilings: how fast it can compute (peak GFLOP/s) and how fast
// it can feed itself data (memory bandwidth, GB/s). Which one limits a given kernel
// depends on its **arithmetic intensity** — FLOPs performed per byte moved. Plot
// attainable performance against intensity and you get the roofline:
//
//     attainable = min( peak,  intensity × bandwidth )
//
// a diagonal bandwidth slope that rises with intensity, meeting a flat peak plateau.
// They cross at the **ridge point**, intensity = peak / bandwidth. A kernel to the
// LEFT of the ridge is **memory-bound**: it's starved for data, and its ceiling is
// intensity × bandwidth — buying faster cores does *nothing*, because the cores are
// already idling, waiting on memory. Only raising its intensity (reuse each byte for
// more FLOPs — blocking, fusion) moves it up the slope. A kernel to the RIGHT is
// compute-bound and lives under the peak plateau, where faster cores help.
//
// This is the model an HPC engineer draws before optimizing anything, because it
// says which knob is even connected. Drag the kernel's intensity across the ridge
// and watch its ceiling stop climbing — the exact moment "make the math faster" stops
// mattering and "move less data" takes over.
//
// It's a log-log plot (intensity and performance both span orders of magnitude), so
// the roofline is two straight lines in log space — hand-rolled here, pure
// arithmetic, WASM-live. Note the same corpus caveat as amdahl/turing: the numbers
// are a model you drag, not a measurement of this single-threaded browser tab.

package roofline

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Peak compute, in GFLOP/s. The flat roof — the most math the cores can do if never
// starved for data.
//
//notebook:slider min=50 max=2000 step=50
func peakGFLOPs() (peak int) { return 1000 }

// Memory bandwidth, in GB/s. The slope of the roofline — how fast data can be fed in.
//
//notebook:slider min=20 max=1000 step=10
func bandwidthGBs() (bw int) { return 200 }

// The kernel's arithmetic intensity, in hundredths of a FLOP/byte (25 → 0.25). Where
// it sits relative to the ridge decides memory-bound vs compute-bound. Drag it across
// the ridge and watch the ceiling stop rising.
//
//notebook:slider min=5 max=6400 step=5
func intensityCenti() (ai int) { return 25 }

// ---------------------------------------------------------------------------
// Compute (Go) — the roofline, pure.
// ---------------------------------------------------------------------------

// The roofline model: the ridge point (peak/bandwidth), the kernel's attainable
// performance (min of the two ceilings at its intensity), and whether it's memory-
// or compute-bound. Pure in (peak, bandwidth, intensity).
func roofline(peak int, bw int, ai int) (r Roofline) {
	p := float64(peak)
	b := float64(bw)
	intensity := float64(ai) / 100

	ridge := p / b // intensity at which bandwidth slope meets the peak plateau
	bwCeil := intensity * b
	attainable := math.Min(p, bwCeil)
	memoryBound := intensity < ridge

	// how much of peak this kernel can reach, and the headroom left on the table.
	ofPeak := attainable / p

	return Roofline{
		Peak: p, Bandwidth: b, Intensity: intensity,
		Ridge: ridge, Attainable: attainable, MemoryBound: memoryBound, OfPeak: ofPeak,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The roofline plot on log-log axes: the bandwidth slope rising to the peak plateau,
// the ridge point where they meet, and the kernel plotted as a dot sitting ON its
// ceiling (the point of the model is that a kernel can't exceed the roof). Left of
// the ridge is shaded memory-bound; right is compute-bound.
//
//notebook:height=440
func plot(r Roofline) (chart Chart) {
	return Chart{R: r}
}

// The verdict: memory- or compute-bound, the attainable performance, the ridge, and
// what to do about it. The advice flips at the ridge — that flip is the whole point.
func readout(r Roofline) (report Readout) {
	bound := "compute-bound"
	advice := "cores are the bottleneck — faster math or more FLOP/s help"
	if r.MemoryBound {
		bound = "memory-bound"
		advice = "starved for data — raise intensity (reuse bytes); faster cores do nothing"
	}
	return Readout{Cards: []Card{
		{Label: "verdict", Value: bound, Caption: advice},
		{Label: "attainable", Value: f0(r.Attainable) + " GFLOP/s", Caption: pct(r.OfPeak) + " of peak"},
		{Label: "ridge point", Value: f2(r.Ridge) + " FLOP/byte", Caption: "peak ÷ bandwidth — the memory/compute divide"},
		{Label: "kernel intensity", Value: f2(r.Intensity) + " FLOP/byte"},
	}}
}

// The roofline — is your kernel compute-bound or memory-bound?
func intro() (md Markdown) {
	return `Attainable performance is **min(peak, intensity × bandwidth)** — a flat
compute roof and a rising memory slope that meet at the **ridge point**
(peak ÷ bandwidth). Where your kernel's **arithmetic intensity** (FLOPs per byte)
falls decides everything.

Left of the ridge it's **memory-bound**: starved for data, ceiling = intensity ×
bandwidth. Buying faster cores does *nothing* — they're already waiting on memory.
The only way up is to raise intensity: reuse each byte for more FLOPs. Right of the
ridge it's **compute-bound**, and faster cores finally help. Drag the intensity
across the ridge and watch the verdict — and the advice — flip. It's the plot an HPC
engineer draws before touching anything, because it says which knob is connected.`
}

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string  { return strconv.FormatFloat(v, 'f', 0, 64) }
func f2(v float64) string  { return strconv.FormatFloat(v, 'f', 2, 64) }
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

// ===========================================================================
// Types
// ===========================================================================

// Roofline is the model's state.
type Roofline struct {
	Peak, Bandwidth float64
	Intensity       float64
	Ridge           float64
	Attainable      float64
	MemoryBound     bool
	OfPeak          float64
}

// Chart draws the roofline on log-log axes.
type Chart struct{ R Roofline }

func (c Chart) Render() Rendered {
	r := c.R
	const w, h, pad = 720.0, 440.0, 52.0
	// log-log axes. x = intensity in [xlo, xhi] FLOP/byte; y = perf in [ylo, yhi].
	const xlo, xhi = 0.03125, 128.0 // 2^-5 .. 2^7
	ylo, yhi := 1.0, r.Peak*2
	if yhi < 100 {
		yhi = 100
	}
	lx := func(v float64) float64 {
		t := (math.Log2(v) - math.Log2(xlo)) / (math.Log2(xhi) - math.Log2(xlo))
		return pad + t*(w-2*pad)
	}
	ly := func(v float64) float64 {
		if v < ylo {
			v = ylo
		}
		t := (math.Log10(v) - math.Log10(ylo)) / (math.Log10(yhi) - math.Log10(ylo))
		return h - pad - t*(h-2*pad)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// shade the memory-bound region (left of the ridge) faintly.
	ridgeX := lx(clampX(r.Ridge, xlo, xhi))
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#fde8e8" fill-opacity="0.6"/>`,
		pad, pad, ridgeX-pad, h-2*pad)

	// the roofline: bandwidth slope from xlo up to the ridge, then flat peak to xhi.
	slopeAt := func(x float64) float64 { return x * r.Bandwidth }
	// point at left edge on the slope, the ridge, and the right edge on the peak.
	x0 := xlo
	fmt.Fprintf(&b, `<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f" fill="none" stroke="#1b3a6b" stroke-width="2.5"/>`,
		lx(x0), ly(slopeAt(x0)), lx(r.Ridge), ly(r.Peak), lx(xhi), ly(r.Peak))

	// ridge marker
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-dasharray="3 3"/>`,
		ridgeX, pad, ridgeX, h-pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#64748b" text-anchor="middle">ridge %.1f</text>`,
		ridgeX, pad-4, r.Ridge)

	// the kernel: a dot sitting ON its ceiling at its intensity.
	kx, ky := lx(clampX(r.Intensity, xlo, xhi)), ly(r.Attainable)
	col := "#dc2626" // memory-bound: red
	if !r.MemoryBound {
		col = "#237a2b" // compute-bound: green
	}
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill=%q stroke="#fff" stroke-width="1.5"/>`, kx, ky, col)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" font-weight="600" fill=%q>%s GFLOP/s</text>`,
		kx+9, ky+4, col, f0(r.Attainable))

	// axis labels
	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#334155">performance (GFLOP/s) vs arithmetic intensity (FLOP/byte) — log-log</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#b45309">memory-bound</text>`, pad+8, h-pad-8)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#237a2b" text-anchor="end">compute-bound</text>`, w-pad-8, pad+16)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// clampX keeps a value inside the plotted x range so a marker never runs off-axis.
func clampX(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
