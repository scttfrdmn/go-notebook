//go:notebook
//
// Summary statistics lie.
//
// Anscombe's quartet (1973) and the Datasaurus dozen (2017) make the same point:
// four — or thirteen — datasets can share the *same* mean, variance, correlation,
// and least-squares line, and look nothing alike. The lesson every statistics
// course opens with and every dashboard forgets: **you have to look at the picture.**
//
// This notebook is that lesson made manipulable. The scatter's points are a
// draggable leaf. Drag one and watch two things at once:
//
//   - the SCATTER changes shape — a blob, a line, a slope, a dinosaur;
//   - the SUMMARY barely moves — mean, variance, correlation, the fitted line.
//
// The grip mechanism is curvefit's, unchanged: the RENDERER reads the leaf to draw
// the handles, the RUNTIME writes it when you drag. `scatter` depends on `points`;
// `points` does not depend on `scatter`. No cycle, no two-way binding, no JS.
//
// It is also the project's own ethos wearing the field's oldest cautionary tale:
// observe the effect, do not trust the number. Here the number is *designed* to
// deceive you, and the graph is the only honest witness.
//
// Arranged so the two things you watch sit SIDE BY SIDE — drag the scatter on the
// left, watch the summary hold on the right — which is the whole argument. Strip
// the layout lines and it still renders, just stacked.
//
//notebook:layout intro
//notebook:layout scatter | summary

package anscombe

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// datasaurus is a hand-traced dinosaur silhouette in the [0,100] window — the seed
// the notebook opens with. It has no special statistics; it is here because a
// recognizable shape makes "drag it into a line and the numbers hold" land harder
// than a blob would. It's a sauropod (long-necked, Brontosaurus-shaped) facing
// left: a small head upper-left, a long neck sloping down to a horizontal body,
// two legs planted at the bottom, and a tail sweeping off to the right. The points
// only plot as dots, so the shape has to read from the dots alone — the features
// are laid out to be legible: a head cluster, a diagonal neck, two leg columns, a
// body band, and a tapering tail.
// The raised tail is deliberate: a high head at the left and a low tail at the
// right would give a strong negative correlation, which would undercut the whole
// point (near-zero r, unmistakable shape). Sweeping the tail up to balance the
// head brings the correlation to about -0.2 — small enough to read as "no linear
// trend" while the picture is plainly a dinosaur. (Verified in the test.)
var datasaurus = []Pt{
	// head (small, upper-left) and eye
	{12, 86}, {8, 83}, {13, 80}, {16, 84},
	{12, 84}, // eye
	// neck — a diagonal line of dots sloping down from the head to the body
	{19, 77}, {24, 71}, {29, 65}, {34, 59},
	// back / spine — the top of the body, left to right, gently rising
	{40, 60}, {47, 62}, {54, 63}, {61, 64},
	// tail — sweeping UP and to the right (upper edge, then back along the lower)
	{68, 66}, {75, 70}, {82, 74}, {89, 78},
	{86, 74}, {79, 70}, {72, 66},
	// belly — the underside of the body, right to left
	{64, 54}, {56, 53}, {48, 53}, {41, 53},
	// front leg — a column of dots down to a foot
	{39, 47}, {38, 35}, {37, 22}, {42, 20}, {43, 34}, {44, 47},
	// back leg — a second column
	{59, 48}, {58, 36}, {57, 24}, {62, 22}, {63, 35}, {64, 48},
	// a few interior body dots so it fills as a solid animal, not an outline
	{46, 57}, {53, 58}, {50, 54}, {58, 57}, {44, 56},
}

// ---------------------------------------------------------------------------
// Cells
// ---------------------------------------------------------------------------

// The dataset. A draggable leaf, seeded with a rough dinosaur so the first drag
// has somewhere dramatic to go. Drag any point; the summary below is engineered
// to hardly notice.
//
//notebook:height=460
func points() (data Draggable[Pt]) {
	return Draggable[Pt]{Value: append([]Pt(nil), datasaurus...)}
}

// Summary statistics — the numbers a report would show, and the numbers that lie.
// Mean and standard deviation of each axis, Pearson correlation, and the slope and
// intercept of the least-squares line. Drag the scatter into any shape you like;
// these move in the third decimal place, if at all.
func stats(data Draggable[Pt]) (summary Stats) {
	return describe(data.Value)
}

// The least-squares line — one more thing that stays put while the picture changes.
// The result is named `fit` so it wires into `scatter`'s `fit []Pt` parameter: an
// edge is a name+type match, and `line` would not be `fit`.
func fitLine(data Draggable[Pt]) (fit []Pt) {
	s := describe(data.Value)
	loX, hiX := math.Inf(1), math.Inf(-1)
	for _, p := range data.Value {
		loX, hiX = math.Min(loX, p.X), math.Max(hiX, p.X)
	}
	if !(hiX > loX) {
		return nil
	}
	return []Pt{
		{X: loX, Y: s.Slope*loX + s.Intercept},
		{X: hiX, Y: s.Slope*hiX + s.Intercept},
	}
}

// Drag the points. The scatter is the only honest view of this data.
//
//notebook:height=460
func scatter(data Draggable[Pt], fit []Pt) (plot Chart) {
	plot = Chart{Points: data.Value, Fit: fit}
	for i, p := range data.Value {
		// The renderer says where the handle is; the runtime decides what dragging
		// it means. Grip(i) is a typed reference to element i of THIS leaf.
		plot.Grips = append(plot.Grips, Handle{At: p, Ref: data.Grip(i)})
	}
	return plot
}

// The summary, as a report would print it. This is the deception, stated in the
// units a decision would be made in: identical to three datasets that share nothing.
func summary(summary Stats) (numbers Readout) {
	return Readout{Cards: []Card{
		{Label: "mean x", Value: f3(summary.MeanX)},
		{Label: "mean y", Value: f3(summary.MeanY)},
		{Label: "std x", Value: f3(summary.StdX)},
		{Label: "std y", Value: f3(summary.StdY)},
		{Label: "correlation", Value: f3(summary.Corr), Caption: "Pearson r"},
		// Built with strconv, not fmt: this is a CELL body, and fmt's format path
		// trips the conservative fmt→os WASM flag (it's fine inside Render, which is
		// not a cell). Keeping the cell fmt-free is what lets this notebook reach the
		// browser tier.
		{Label: "fit", Value: "y = " + f2(summary.Slope) + "x + " + f2(summary.Intercept)},
	}}
}

// Summary statistics lie.
func intro() (md Markdown) {
	return `Every point below is draggable. The panel of numbers is what a report
would show; the scatter is what is actually true. Drag the cloud into a line, a
blob, a curve — the mean, the standard deviations, the correlation, and the fitted
line will scarcely move.

Anscombe made four of these by hand in 1973 to argue that you must plot your data.
The dependency graph above is the same argument in structural form: the summary is
one cell, the picture is another, and only one of them can be trusted alone.`
}

// ===========================================================================
// Statistics
// ===========================================================================

// describe computes the shared summary. These are sufficient statistics — sums
// over the points — so the cell is pure and scrubbing is free: no state, no fold.
func describe(pts []Pt) Stats {
	n := float64(len(pts))
	if n < 2 {
		return Stats{}
	}
	var sx, sy, sxx, syy, sxy float64
	for _, p := range pts {
		sx += p.X
		sy += p.Y
		sxx += p.X * p.X
		syy += p.Y * p.Y
		sxy += p.X * p.Y
	}
	mx, my := sx/n, sy/n
	varX := sxx/n - mx*mx
	varY := syy/n - my*my
	cov := sxy/n - mx*my
	corr := 0.0
	if varX > 0 && varY > 0 {
		corr = cov / math.Sqrt(varX*varY)
	}
	slope := 0.0
	if varX > 0 {
		slope = cov / varX
	}
	return Stats{
		MeanX: mx, MeanY: my,
		StdX: math.Sqrt(varX), StdY: math.Sqrt(varY),
		Corr: corr, Slope: slope, Intercept: my - slope*mx,
	}
}

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

type Stats struct {
	MeanX, MeanY     float64
	StdX, StdY       float64
	Corr             float64
	Slope, Intercept float64
}

// Draggable is a leaf you manipulate directly on the chart. Copied unchanged from
// curvefit: its grips are drawn by a DIFFERENT cell (scatter), so the leaf identity
// rides WITH the value across that boundary — that is what Grip(i) carries. The
// identity is stamped by the runtime via WithLeaf, never by the author.
type Draggable[T any] struct {
	Value []T
	leaf  string // stamped by the runtime via WithLeaf
}

// WithLeaf is the runtime stamping seam (value semantics). A notebook calling it
// is a smell — the runtime calls it when it materializes this leaf.
func (d Draggable[T]) WithLeaf(sym string) Draggable[T] { d.leaf = sym; return d }

// Grip is a typed reference to element i of THIS leaf. No string in the author's
// code; the symbol appears only at the wire (Ref marshals to "leaf:index").
func (d Draggable[T]) Grip(i int) Ref { return Ref{Leaf: d.leaf, Index: i} }

// Reconcile keeps the dragged positions as long as the arity is unchanged. This
// dataset has a fixed point count (no degree slider), so the saved flat
// [x0,y0,x1,y1,...] should always match; if it doesn't, the fresh seed stands.
func (d Draggable[T]) Reconcile(saved any) any {
	flat, ok := saved.([]float64)
	if !ok || len(flat) != 2*len(d.Value) {
		return d
	}
	out := make([]T, len(d.Value))
	for i := range out {
		if p, ok := any(Pt{X: flat[2*i], Y: flat[2*i+1]}).(T); ok {
			out[i] = p
		}
	}
	d.Value = out
	return d
}

// WidgetView carries the Draggable's live state — its point positions as the
// selection. The chart (a Render) draws them; this carries the data on the wire.
func (d Draggable[T]) WidgetView() WidgetView { return WidgetView{Value: d.Value} }

type Ref struct {
	Leaf  string
	Index int
}

// MarshalText renders a grip ref as "leaf:index" for the SVG data-grip attribute —
// the wire form the client parses to route a drag to the right leaf.
func (r Ref) MarshalText() ([]byte, error) {
	return []byte(r.Leaf + ":" + strconv.Itoa(r.Index)), nil
}

// Handle is a declarative request: "draw a grip here; dragging it writes to that
// leaf." The renderer emits it and cannot perform the write — the structural reason
// this cannot grow into a callback system.
type Handle struct {
	At  Pt
	Ref Ref
}

type Chart struct {
	Points []Pt
	Fit    []Pt
	Grips  []Handle
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 460.0, 44.0
	// Fixed data window so the axes don't jump as you drag — the whole point is to
	// compare shapes, which needs a stable frame. The Datasaurus lives in [0,100].
	const lo, hi = 0.0, 100.0
	sx := func(v float64) float64 { return pad + (v-lo)/(hi-lo)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - (v-lo)/(hi-lo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	// Frame.
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// The least-squares line — the thing that stays put.
	if len(c.Fit) == 2 {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#0797b8" `+
			`stroke-width="2" stroke-dasharray="5 4"/>`,
			sx(c.Fit[0].X), sy(c.Fit[0].Y), sx(c.Fit[1].X), sy(c.Fit[1].Y))
	}

	// Grips ARE the points — one draggable circle per datum. data-grip = "leaf:index"
	// is how the client routes a drag to the leaf this handle writes.
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#2a78d6" fill-opacity="0.75" `+
			`stroke="#fff" stroke-width="1" data-grip=%q style="cursor:grab"/>`,
			sx(g.At.X), sy(g.At.Y), string(ref))
	}
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// WidgetView is a widget's state on the wire — matched structurally by the runtime.
type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

// Render draws the summary as a vertical stat list — label above value — which
// reads cleanly in the narrow column the layout places it in, beside the scatter.
// The values are engineered strings (no user input), so no escaping is needed.
// This is what makes the notebook's point land: the numbers sit RIGHT THERE next
// to the shape you're dragging, and they barely move. Brand palette: muted label,
// navy value.
func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;flex-direction:column;gap:.55rem">`)
	for _, c := range r.Cards {
		b.WriteString(`<div>`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:600 20px/1.2 -apple-system,system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">%s</div>`, c.Value)
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#5b6472">%s</div>`, c.Caption)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }

func f3(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
