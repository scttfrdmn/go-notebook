//go:notebook
//
// Amdahl's ceiling — why more cores stop helping.
//
// Speed up a program by parallelizing it, and the speedup is not the number of cores.
// If a fraction p of the work is parallelizable and (1−p) is stubbornly serial, then
// on n cores:
//
//     speedup(n) = 1 / ( (1−p) + p/n )
//
// The serial part doesn't shrink, so as n → ∞ the speedup approaches a hard ceiling
// of 1/(1−p). And the ceiling is brutal: **5% serial caps you at 20×, forever.** Not
// 20× at some large n — 20× is the limit, and you're already past 10× by 20 cores,
// throwing hardware at a wall. Drag the serial fraction and watch the ceiling drop
// and the curve flatten against it.
//
// The honest other half is Gustafson's Law. Amdahl fixes the *problem* and asks how
// much faster; Gustafson fixes the *time* and asks how much *bigger* a problem you can
// do — and there the serial part is a constant while the parallel part grows with n,
// so speedup is roughly linear: (1−p) + p·n. Same hardware, opposite conclusion,
// because they hold different things constant. The plot shows both, so the pessimism
// and its escape hatch sit side by side.
//
// A note the design earns: this is *about* parallel speedup, and go-notebook's own
// scheduler fans cells out across cores — but that dividend is absent in this WASM
// tab (GOOS=js is single-threaded), which the turing and gpulife notebooks explore.
// Here the speedup is a model you drag, not a measurement; the numbers are the law,
// not this tab's cores. Pure arithmetic, so scrub freely.

package amdahl

import (
	"fmt"
	"strconv"
	"strings"
)

const maxCores = 256 // the plot's x-axis runs 1..maxCores

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Parallelizable fraction p, in percent. The rest, (100−p)%, is serial and sets the
// ceiling. 95% sounds great; its ceiling is only 20×.
//
//notebook:slider min=50 max=100 step=1
func parallelPct() (p int) { return 95 }

// Cores n — where the marker sits on the curve, so you can read the speedup at a
// specific core count and see how far below the ceiling it already is.
//
//notebook:slider min=1 max=256 step=1
func cores() (n int) { return 32 }

// ---------------------------------------------------------------------------
// Compute (Go) — the two laws, pure.
// ---------------------------------------------------------------------------

// The speedup model: Amdahl's curve over 1..maxCores, its asymptotic ceiling
// 1/(1−p), Gustafson's linear curve for contrast, and the values at the marker n.
// Pure in (p, n).
func model(p int, n int) (m Model) {
	par := float64(p) / 100
	ser := 1 - par

	amdahl := make([]float64, maxCores)
	gustafson := make([]float64, maxCores)
	for i := 0; i < maxCores; i++ {
		c := float64(i + 1)
		amdahl[i] = 1 / (ser + par/c) // fixed problem: how much faster
		gustafson[i] = ser + par*c    // fixed time: how much bigger
	}
	ceiling := 0.0
	if ser > 0 {
		ceiling = 1 / ser
	}
	// speedup at the chosen core count.
	cn := float64(n)
	at := 1 / (ser + par/cn)

	// efficiency at the marker: speedup per core (how much of each core you're using).
	eff := at / cn

	return Model{
		Amdahl: amdahl, Gustafson: gustafson,
		Ceiling: ceiling, ParFrac: par,
		N: n, SpeedupAtN: at, EfficiencyAtN: eff,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The speedup curves: Amdahl (what you actually get, bending over toward its
// ceiling) and Gustafson (the linear line, if you grow the problem instead), with
// the ceiling drawn as a dashed asymptote and a marker at the chosen core count. The
// gap between the Amdahl curve and the diagonal is the parallelism you paid for and
// didn't get.
//
//notebook:height=420
func curves(m Model) (chart Chart) {
	return Chart{M: m}
}

// The numbers: the ceiling the serial fraction imposes, the speedup you actually get
// at n cores, and the efficiency (how much of each core is doing useful work — it
// collapses as you add cores past the knee).
func readout(m Model) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "ceiling (n → ∞)", Value: f1(m.Ceiling) + "×", Caption: "1/(1−p) — the serial part sets it, forever"},
		{Label: "speedup at " + strconv.Itoa(m.N) + " cores", Value: f1(m.SpeedupAtN) + "×"},
		{Label: "efficiency", Value: pct(m.EfficiencyAtN), Caption: "speedup ÷ cores — useful work per core"},
		{Label: "cores wasted", Value: f0(float64(m.N)-m.SpeedupAtN) + " of " + strconv.Itoa(m.N), Caption: "cores minus the speedup they bought"},
	}}
}

// Amdahl's ceiling — why more cores stop helping.
func intro() (md Markdown) {
	return `Parallelize a fraction *p* of a program; the rest stays serial. Speedup on
*n* cores is **1 / ((1−p) + p/n)** — and the serial part never shrinks, so it hits a
ceiling of **1/(1−p)**. That ceiling bites: **95% parallel caps you at 20×**, and
you're already past 10× by 20 cores. Drag *p* and watch the curve flatten against a
wall.

The escape hatch is **Gustafson's Law** (the straighter line): if you grow the
*problem* with the cores instead of holding it fixed, the serial part is a constant
and speedup goes roughly linear. Amdahl asks "how much faster for the same work";
Gustafson asks "how much more work in the same time." Same hardware, opposite mood.

Here the speedup is a *model you drag*, not a measurement — and note the irony: this
runs single-threaded in your browser tab (WASM), so the real fan-out the design
brags about is exactly the thing absent here. Pure arithmetic; scrub freely.`
}

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string  { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string  { return strconv.FormatFloat(v, 'f', 1, 64) }
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, v := range xs {
		if v > m {
			m = v
		}
	}
	return m
}

// ===========================================================================
// Types
// ===========================================================================

// Model holds both speedup curves over 1..maxCores plus the marker readings.
type Model struct {
	Amdahl        []float64
	Gustafson     []float64
	Ceiling       float64
	ParFrac       float64
	N             int
	SpeedupAtN    float64
	EfficiencyAtN float64
}

// Chart draws the two speedup curves with the ceiling asymptote and the marker.
type Chart struct{ M Model }

func (c Chart) Render() Rendered {
	m := c.M
	const w, h, pad = 720.0, 420.0, 46.0
	// y axis: cap at a bit above the linear diagonal at maxCores would be huge, so
	// scale to the larger of the ceiling×1.3 and the speedup we actually plot, but
	// keep the diagonal visible near the origin. Use max Amdahl (≈ceiling) headroom.
	ymax := maxOf(m.Amdahl) * 1.35
	if ymax < 2 {
		ymax = 2
	}
	sx := func(coreIdx int) float64 { return pad + float64(coreIdx)/float64(maxCores-1)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/ymax*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// ceiling asymptote
	if m.Ceiling > 0 && m.Ceiling <= ymax {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#d03b3b" stroke-width="1.5" stroke-dasharray="5 4"/>`,
			pad, sy(m.Ceiling), w-pad, sy(m.Ceiling))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#d03b3b">ceiling %s×</text>`,
			w-pad-70, sy(m.Ceiling)-5, f1(m.Ceiling))
	}

	line := func(series []float64, color string, width float64, clip bool) {
		var d strings.Builder
		started := false
		for i, v := range series {
			if clip && v > ymax {
				continue
			}
			verb := " L"
			if !started {
				verb = "M"
				started = true
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(i), sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="%.1f"/>`, d.String(), color, width)
	}
	line(m.Gustafson, "#0797b8", 1.5, true) // Gustafson: the near-linear contrast line, aqua
	line(m.Amdahl, "#2a78d6", 2.5, false)   // Amdahl: the bold curve bending to the ceiling, blue

	// marker at n cores
	mx, my := sx(m.N-1), sy(m.SpeedupAtN)
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#2a78d6"/>`, mx, my)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" font-weight="600" fill="#2a78d6">%s× at %d cores</text>`,
		mx+8, my-6, f1(m.SpeedupAtN), m.N)

	// labels
	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#1b3a6b">speedup vs cores</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#2a78d6">Amdahl (fixed problem)</text>`, pad+6, h-pad-24)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#0797b8">Gustafson (grow problem)</text>`, pad+6, h-pad-8)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
