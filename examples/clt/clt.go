//go:notebook
//
// The Central Limit Theorem, and why it needs purity.
//
// Sculpt a population into any shape you like — bimodal, skewed, a spike, a mess —
// by dragging its bars. Then draw many samples of size n from it and histogram the
// sample *means*. The theorem says: however wild the population, the distribution
// of the mean approaches a normal as n grows, centred on the population mean μ with
// standard deviation σ/√n. Drag the bars into something hideous and the bell on the
// right barely notices.
//
// The part this notebook exists to show is what happens when you scrub n *back
// down*. At n = 1 the "sampling distribution" is just the population itself —
// whatever mess you drew. Turn n up and it collapses toward a tight bell. Turn it
// back down and the bell *widens again, un-converging*, returning exactly to the
// mess. That reversibility is the whole point, and it is only possible because
// nothing here is stateful:
//
//     the sampling distribution is a PURE function of (population, n, trials, seed).
//
// A fold could not do this. A fold that accumulated sample means as n grew could
// only go forward; scrubbing n down would have to replay from zero, and the
// "distribution at n = 5" would be lost the moment you passed it. Here there is no
// history to lose — every frame is recomputed from the sliders, so the CLT is a
// thing you can slide back and forth through, not just watch happen once. This is
// the bayes lesson (sufficient statistics, not a fold) applied to the theorem that
// is *literally about what averaging does*.
//
// The two views share a `//notebook:row=panels` directive, so the population you're
// sculpting sits beside the bell it produces.

package clt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const bins = 12 // the population is a histogram over [0, bins); value = bin + U(0,1)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// The population. Draggable bar heights — sculpt any shape and watch the mean's
// distribution stay a bell. Seeded deliberately wild (bimodal with a spike) so the
// theorem has something to defy.
//
//notebook:row=panels
//notebook:height=420
func population() (pop Draggable[Pt]) {
	// Bar i sits at x = i + 0.5 (the bin centre); its Y is the draggable height.
	seed := []float64{0.20, 0.95, 0.70, 0.18, 0.10, 0.10, 0.12, 0.20, 0.65, 0.90, 0.35, 0.12}
	pts := make([]Pt, bins)
	for i := range pts {
		pts[i] = Pt{X: float64(i) + 0.5, Y: seed[i]}
	}
	return Draggable[Pt]{Value: pts}
}

// Sample size n — the scrub axis. n = 1 shows the population itself; turn it up and
// the mean's distribution collapses to a bell; turn it back down and it un-converges.
//
//notebook:slider min=1 max=50 step=1
func sampleSize() (n int) { return 5 }

// How many sample means to draw — the resolution of the bell on the right. More
// trials, smoother histogram; it does not change the shape, only its clarity.
//
//notebook:slider min=500 max=8000 step=500
func trials() (m int) { return 3000 }

// Seed for the sampling. A leaf, not a global RNG, so the whole picture is a pure,
// reproducible function of the sliders.
//
//notebook:slider min=1 max=999 step=1
func seed() (s int) { return 12 }

// ---------------------------------------------------------------------------
// Compute (Go) — pure.
// ---------------------------------------------------------------------------

// The population as a probability distribution plus its exact moments. Because each
// bar is a uniform slab (value = bin + U(0,1)), the mean and variance are
// closed-form — no sampling needed for the *population's* own statistics.
func distribution(pop Draggable[Pt]) (dist Dist) {
	h := make([]float64, bins)
	var total float64
	for i, p := range pop.Value {
		if i >= bins {
			break
		}
		if p.Y > 0 {
			h[i] = p.Y
		}
		total += h[i]
	}
	if total == 0 { // a fully-flattened population — fall back to uniform
		for i := range h {
			h[i] = 1
		}
		total = bins
	}
	prob := make([]float64, bins)
	var mean float64
	for i := range h {
		prob[i] = h[i] / total
		mean += prob[i] * (float64(i) + 0.5) // E[value | bin i] = i + 0.5
	}
	var second float64
	for i := range prob {
		// E[value² | bin i] = Var(U)+E² = 1/12 + (i+0.5)²
		e2 := 1.0/12.0 + (float64(i)+0.5)*(float64(i)+0.5)
		second += prob[i] * e2
	}
	variance := second - mean*mean
	return Dist{Prob: prob, Mean: mean, SD: math.Sqrt(variance)}
}

// The sampling distribution of the mean: draw `trials` samples of size n from the
// population and histogram their means. Pure — a function of (dist, n, trials, seed)
// — which is what lets n scrub both ways. Also returns the CLT's prediction:
// N(μ, σ²/n), the bell the histogram should approach.
func sampling(dist Dist, n int, m int, s int) (samp Sampling) {
	cdf := make([]float64, bins)
	acc := 0.0
	for i, p := range dist.Prob {
		acc += p
		cdf[i] = acc
	}
	rng := newLCG(uint64(s))

	hist := make([]float64, meansBins)
	for t := 0; t < m; t++ {
		sum := 0.0
		for k := 0; k < n; k++ {
			u := rng.f() // pick a bin by inverse-CDF, then a uniform point inside it
			b := 0
			for b < bins-1 && u > cdf[b] {
				b++
			}
			sum += float64(b) + rng.f()
		}
		mean := sum / float64(n)
		idx := int(mean / float64(bins) * float64(meansBins))
		if idx >= 0 && idx < meansBins {
			hist[idx]++
		}
	}
	// Normalize the histogram to a density (area 1) over the [0, bins) range.
	binW := float64(bins) / float64(meansBins)
	for i := range hist {
		hist[i] /= float64(m) * binW
	}
	predSD := dist.SD / math.Sqrt(float64(n))
	return Sampling{Hist: hist, PredMean: dist.Mean, PredSD: predSD, N: n}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The population you sculpt. Drag any bar. The mean's distribution beside it will
// stay a bell no matter how wild you make this.
//
//notebook:row=panels
//notebook:height=420
func populationView(pop Draggable[Pt], dist Dist) (popChart PopChart) {
	popChart = PopChart{Mean: dist.Mean}
	for i, p := range pop.Value {
		popChart.Bars = append(popChart.Bars, Bar{X: p.X, H: p.Y})
		popChart.Grips = append(popChart.Grips, Handle{At: p, Ref: pop.Grip(i)})
	}
	return popChart
}

// The sampling distribution of the mean, with the CLT's predicted normal overlaid.
// At n = 1 it mirrors the population; as n grows it collapses onto the bell.
//
//notebook:row=panels
//notebook:height=420
func samplingView(samp Sampling) (meansChart MeansChart) {
	return MeansChart{Hist: samp.Hist, Mean: samp.PredMean, SD: samp.PredSD}
}

// The numbers: the population's own mean and spread, the CLT's prediction for the
// mean's spread (σ/√n), and n. Watch σ/√n shrink as you turn n up.
func readout(dist Dist, samp Sampling) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "population μ", Value: f2(dist.Mean)},
		{Label: "population σ", Value: f2(dist.SD)},
		{Label: "n", Value: strconv.Itoa(samp.N)},
		{Label: "predicted σ/√n", Value: f2(samp.PredSD), Caption: "the mean's spread — shrinks as n grows"},
	}}
}

// The Central Limit Theorem, and why it needs purity.
func intro() (md Markdown) {
	return `Drag the population's bars into any shape — bimodal, skewed, a spike, a
mess. Beside it is the distribution of the **sample mean** for samples of size *n*,
with the normal the theorem predicts overlaid. However ugly the population, that
bell holds.

Now scrub *n*. At **n = 1** the right panel just mirrors your mess; turn *n* up and
it collapses to a tight bell (width σ/√n); turn it **back down** and it *un-converges*
— widening back to the mess, exactly. That reversibility is the point: the sampling
distribution is a **pure function of (population, n, trials, seed)**, not a fold, so
you can slide back and forth *through* the theorem. A fold could only go forward.`
}

// ===========================================================================
// RNG — a small LCG, so sampling is deterministic in the seed (no global rand).
// ===========================================================================

type lcg struct{ state uint64 }

func newLCG(s uint64) *lcg { return &lcg{state: s*2654435761 + 1} }

func (r *lcg) f() float64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float64(r.state>>11) / float64(1<<53)
}

// ===========================================================================
// Helpers
// ===========================================================================

const meansBins = 60 // resolution of the sampling-distribution histogram

func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

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

type Pt struct{ X, Y float64 }

// Dist is the population as a normalized distribution plus its exact moments.
type Dist struct {
	Prob []float64
	Mean float64
	SD   float64
}

// Sampling is the histogram of sample means (as a density) plus the CLT's predicted
// normal N(Mean, SD²).
type Sampling struct {
	Hist     []float64
	PredMean float64
	PredSD   float64
	N        int
}

// Draggable — curvefit's grip leaf. Here the bars' heights are dragged; X is the
// fixed bin centre and the compute side ignores any X drift.
type Draggable[T any] struct {
	Value []T
	leaf  string
}

func (d Draggable[T]) WithLeaf(sym string) Draggable[T] { d.leaf = sym; return d }
func (d Draggable[T]) Grip(i int) Ref                   { return Ref{Leaf: d.leaf, Index: i} }

func (d Draggable[T]) Reconcile(saved any) any {
	flat, ok := saved.([]float64)
	if !ok || len(flat) != 2*len(d.Value) {
		return d
	}
	out := make([]T, len(d.Value))
	for i := range out {
		// Clamp X back to the bin centre: only the height (Y) is a real degree of
		// freedom, so a horizontal drag can't slide a bar off its bin.
		if p, ok := any(Pt{X: float64(i) + 0.5, Y: clamp01hi(flat[2*i+1])}).(T); ok {
			out[i] = p
		}
	}
	d.Value = out
	return d
}

func (d Draggable[T]) WidgetView() WidgetView { return WidgetView{Value: d.Value} }

// clamp01hi keeps a dragged bar height in a sane [0, 1] range.
func clamp01hi(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type Ref struct {
	Leaf  string
	Index int
}

func (r Ref) MarshalText() ([]byte, error) { return []byte(r.Leaf + ":" + strconv.Itoa(r.Index)), nil }

type Handle struct {
	At  Pt
	Ref Ref
}

type Bar struct {
	X, H float64
}

// PopChart draws the draggable population histogram, grips at the bar tops.
type PopChart struct {
	Bars  []Bar
	Grips []Handle
	Mean  float64
}

func (c PopChart) Render() Rendered {
	const w, h, pad = 440.0, 420.0, 36.0
	sx := func(v float64) float64 { return pad + v/float64(bins)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v*(h-2*pad) } // heights in [0,1]

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)
	bw := (w - 2*pad) / float64(bins)
	for _, bar := range c.Bars {
		x := sx(bar.X - 0.5)
		top := sy(bar.H)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#93a4c9" fill-opacity="0.7"/>`,
			x+1, top, bw-2, h-pad-top)
	}
	// mean marker
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="#c026d3" stroke-width="1.5" stroke-dasharray="4 3"/>`,
		sx(c.Mean), pad, sx(c.Mean), h-pad)
	// grips at bar tops
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#fff" stroke="#4338ca" stroke-width="2" `+
			`data-grip=%q style="cursor:ns-resize"/>`, sx(g.At.X), sy(g.At.Y), string(ref))
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12" fill="#334155">population — drag the bars</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// MeansChart draws the sampling distribution of the mean with the predicted normal.
type MeansChart struct {
	Hist []float64
	Mean float64
	SD   float64
}

func (c MeansChart) Render() Rendered {
	const w, h, pad = 440.0, 420.0, 36.0
	// y scale: the taller of the histogram peak and the normal peak.
	normPeak := 0.0
	if c.SD > 0 {
		normPeak = 1 / (c.SD * math.Sqrt(2*math.Pi))
	}
	ymax := maxOf(c.Hist)
	if normPeak > ymax {
		ymax = normPeak
	}
	if ymax <= 0 {
		ymax = 1
	}
	sx := func(v float64) float64 { return pad + v/float64(bins)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/ymax*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)
	// histogram of means
	bw := (w - 2*pad) / float64(meansBins)
	for i, v := range c.Hist {
		x := pad + float64(i)*bw
		top := sy(v)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#3ea6ff" fill-opacity="0.55"/>`,
			x, top, bw, h-pad-top)
	}
	// predicted normal curve
	if c.SD > 0 {
		var d strings.Builder
		for px := 0; px <= 120; px++ {
			xv := float64(px) / 120 * float64(bins)
			z := (xv - c.Mean) / c.SD
			y := normPeak * math.Exp(-0.5*z*z)
			verb := " L"
			if px == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(xv), sy(y))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#c026d3" stroke-width="2.5"/>`, d.String())
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12" fill="#334155">distribution of the mean — with N(μ, σ²/n)</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
