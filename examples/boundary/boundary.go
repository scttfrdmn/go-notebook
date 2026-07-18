//go:notebook
//
// What did the classifier actually learn?
//
// Two clouds of points, two classes. A logistic-regression classifier draws the
// line it thinks separates them, and shades the plane by how confident it is on
// each side. Every point is draggable — so you can *interrogate* the model:
//
//   - Drag a point across the line and watch the boundary swing to chase it. A
//     single training example moves the whole decision surface. That is the thing
//     tutorials describe and never let you feel.
//   - Now drag a point far out into the wrong class — an outlier, or a mislabel —
//     and watch the boundary tilt to accommodate one bad datum. Then turn the
//     **regularization** slider up: the same outlier barely moves the line. L2
//     shrinks the weights, so no single point can shout. The footgun and its
//     seatbelt, both draggable.
//
// Two things this puts on stage the corpus hadn't:
//
//   - **Two grip leaves on one chart.** classA and classB are two separate
//     draggable leaves, both drawn on the same decision surface. A grip on a blue
//     point routes to the classA leaf, a red point to classB — the leaf identity
//     rides with each value across the cell boundary (curvefit's mechanism, now
//     with two leaves feeding one view, which is what stresses it).
//   - **A fitted model as a pure cell.** `fit` runs logistic-regression gradient
//     descent to convergence *inside one cell* — a pure function of (points,
//     regularization). Drag a point and the fit re-runs from scratch; there is no
//     training state to carry, so the boundary is always exactly the fit for what's
//     on screen. Scrub regularization up and down and it's exact both ways.
//
// The surface and points share a `//notebook:area=panels` slot with the readout.

package boundary

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strconv"
	"strings"
)

const span = 10.0 // the plane is [0, span] × [0, span]

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Class A (blue). Draggable training points. Drag one across the line and watch the
// boundary chase it.
//
//notebook:height=460
func classA() (a Draggable[Pt]) {
	return Draggable[Pt]{Value: []Pt{{2, 3}, {3, 2}, {2.5, 4}, {3.5, 3}, {1.5, 2.5}, {4, 2}}}
}

// Class B (red). The other cloud. Drag one of these out into A's territory to see a
// single outlier tilt the whole boundary.
//
//notebook:height=460
func classB() (b Draggable[Pt]) {
	return Draggable[Pt]{Value: []Pt{{7, 8}, {8, 7}, {7.5, 6}, {6.5, 7.5}, {8.5, 8}, {6, 8}}}
}

// Regularization strength (L2), in hundredths — 20 → λ = 0.20. Turn it up and the
// boundary stops chasing outliers: L2 shrinks the weights so no single point can
// dominate. Turn it down and the model bends to every datum.
//
//notebook:slider min=1 max=100 step=1
func regularizationCenti() (lambda int) { return 10 }

// ---------------------------------------------------------------------------
// Compute (Go) — logistic regression, pure.
// ---------------------------------------------------------------------------

// The fitted classifier: logistic regression on the two clouds, by gradient descent
// with L2 regularization, run to a fixed horizon inside this one cell. Pure — a
// function of (classA, classB, λ) alone — so dragging a point re-fits from scratch
// and scrubbing λ is exact both ways. Features are centred on the plane's middle so
// gradient descent is well-conditioned.
func fit(a Draggable[Pt], b Draggable[Pt], lambda int) (model Model) {
	lam := float64(lambda) / 100
	type sample struct {
		x, y float64
		t    float64 // target: 1 for A, 0 for B
	}
	var data []sample
	for _, p := range a.Value {
		data = append(data, sample{p.X - span/2, p.Y - span/2, 1})
	}
	for _, p := range b.Value {
		data = append(data, sample{p.X - span/2, p.Y - span/2, 0})
	}
	if len(data) == 0 {
		return Model{}
	}

	// gradient descent on the L2-regularized cross-entropy loss.
	var w0, w1, bias float64
	const iters = 400
	const lr = 0.3
	n := float64(len(data))
	for it := 0; it < iters; it++ {
		var g0, g1, gb float64
		for _, s := range data {
			z := w0*s.x + w1*s.y + bias
			p := 1 / (1 + math.Exp(-z))
			e := p - s.t
			g0 += e * s.x
			g1 += e * s.y
			gb += e
		}
		// mean gradient + L2 on the weights (not the bias, by convention).
		w0 -= lr * (g0/n + lam*w0)
		w1 -= lr * (g1/n + lam*w1)
		bias -= lr * (gb / n)
	}
	return Model{W0: w0, W1: w1, B: bias}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The decision surface: the plane shaded by the model's confidence (blue for A, red
// for B), the boundary line where it's a coin flip, and every training point as a
// draggable grip. This is the picture of what the model learned — and you can grab
// any point and change it.
//
//notebook:area=panels
//notebook:height=460
func surface(a Draggable[Pt], b Draggable[Pt], model Model) (plot Surface) {
	plot = Surface{Model: model}
	for i, p := range a.Value {
		plot.A = append(plot.A, Handle{At: p, Ref: a.Grip(i)})
	}
	for i, p := range b.Value {
		plot.B = append(plot.B, Handle{At: p, Ref: b.Grip(i)})
	}
	return plot
}

// The numbers: training accuracy, the weight magnitude (how confident/steep the
// surface is — L2 pulls this down), and the boundary's angle. Watch |w| shrink as
// you turn regularization up.
//
//notebook:area=panels
func readout(a Draggable[Pt], b Draggable[Pt], model Model) (report Readout) {
	acc := accuracy(a.Value, b.Value, model)
	wmag := math.Hypot(model.W0, model.W1)
	return Readout{Cards: []Card{
		{Label: "training accuracy", Value: pct(acc)},
		{Label: "|w| (weight magnitude)", Value: f2(wmag), Caption: "regularization pulls this down"},
		{Label: "boundary angle", Value: f0(angleDeg(model)) + "°"},
	}}
}

// What did the classifier actually learn?
func intro() (md Markdown) {
	return `Two clouds, two classes, and the line a logistic-regression classifier
draws between them — with the plane shaded by its confidence. **Every point is
draggable.**

Drag one across the line: the boundary swings to chase it — one training example
moves the whole surface. Now drag a point deep into the wrong class, an outlier, and
watch the line tilt to accommodate it. Then turn **regularization** up: the same
outlier barely moves it, because L2 shrinks the weights so no single point can
shout. The footgun and its seatbelt, both under your mouse.

The fit is a pure cell — gradient descent to convergence, a function of the points
and λ alone — so drag re-fits from scratch and scrubbing λ is exact both ways.`
}

// ===========================================================================
// Model
// ===========================================================================

// Model is the fitted logistic regression: P(A | x,y) = σ(W0·x' + W1·y' + B), where
// x' = x − span/2 (features are centred on the plane).
type Model struct {
	W0, W1, B float64
}

// prob returns P(class A) at plane coordinates (x, y).
func (m Model) prob(x, y float64) float64 {
	z := m.W0*(x-span/2) + m.W1*(y-span/2) + m.B
	return 1 / (1 + math.Exp(-z))
}

func accuracy(a, b []Pt, m Model) float64 {
	correct, total := 0, 0
	for _, p := range a {
		if m.prob(p.X, p.Y) >= 0.5 {
			correct++
		}
		total++
	}
	for _, p := range b {
		if m.prob(p.X, p.Y) < 0.5 {
			correct++
		}
		total++
	}
	if total == 0 {
		return 0
	}
	return float64(correct) / float64(total)
}

// angleDeg is the orientation of the boundary line (perpendicular to the weight
// vector), in degrees.
func angleDeg(m Model) float64 {
	return math.Atan2(-m.W0, m.W1) * 180 / math.Pi
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

type Pt struct{ X, Y float64 }

// Draggable — curvefit's grip leaf. Two of these (classA, classB) feed the one
// surface; each carries its own stamped leaf identity, so a grip routes to the
// class it belongs to.
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
		if p, ok := any(Pt{X: flat[2*i], Y: flat[2*i+1]}).(T); ok {
			out[i] = p
		}
	}
	d.Value = out
	return d
}

func (d Draggable[T]) WidgetView() WidgetView { return WidgetView{Value: d.Value} }

type Ref struct {
	Leaf  string
	Index int
}

func (r Ref) MarshalText() ([]byte, error) { return []byte(r.Leaf + ":" + strconv.Itoa(r.Index)), nil }

type Handle struct {
	At  Pt
	Ref Ref
}

// Surface renders the decision surface: a confidence heatmap, the boundary, and the
// two classes of draggable points.
type Surface struct {
	Model Model
	A     []Handle
	B     []Handle
}

func (s Surface) Render() Rendered {
	const box, grid = 460.0, 92
	sx := func(v float64) float64 { return v / span * box }
	sy := func(v float64) float64 { return box - v/span*box } // y up

	// Confidence heatmap as a PNG data URI, embedded in the SVG. Blue (A) ↔ red (B),
	// pale at the boundary. Small grid, upscaled pixelated.
	uri := pngHeatmap(s.Model, grid)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, box, box)
	fmt.Fprintf(&b, `<image x="0" y="0" width="%.0f" height="%.0f" href=%q style="image-rendering:pixelated"/>`,
		box, box, uri)

	// Boundary line: where W0·x' + W1·y' + B = 0. Draw it across the plane.
	if x0, y0, x1, y1, ok := boundarySegment(s.Model); ok {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#1b3a6b" stroke-width="2"/>`,
			sx(x0), sy(y0), sx(x1), sy(y1))
	}

	// Grips: blue for class A, red for class B; data-grip routes each to its leaf.
	drawPts := func(hs []Handle, fill string) {
		for _, hnd := range hs {
			ref, _ := hnd.Ref.MarshalText()
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill=%q stroke="#fff" stroke-width="2" `+
				`data-grip=%q style="cursor:grab"/>`, sx(hnd.At.X), sy(hnd.At.Y), fill, string(ref))
		}
	}
	drawPts(s.A, "#2a78d6") // class A, blue
	drawPts(s.B, "#e34948") // class B, red
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// pngHeatmap renders the confidence field as a base64 PNG data URI.
func pngHeatmap(m Model, grid int) string {
	img := image.NewRGBA(image.Rect(0, 0, grid, grid))
	for j := 0; j < grid; j++ {
		for i := 0; i < grid; i++ {
			x := (float64(i) + 0.5) / float64(grid) * span
			y := span - (float64(j)+0.5)/float64(grid)*span // image y is top-down
			r, g, bl := confColor(m.prob(x, y))             // P(A)
			o := (j*grid + i) * 4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, g, bl, 255
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// confColor maps P(A) in [0,1] to a colour: red (B) at 0, pale at 0.5, blue (A) at 1.
func confColor(p float64) (uint8, uint8, uint8) {
	// blend between red and blue through a pale middle
	if p >= 0.5 {
		t := (p - 0.5) * 2 // 0..1 toward blue
		return lerp(240, 219, t), lerp(238, 234, t), lerp(245, 254, t)
	}
	t := (0.5 - p) * 2 // 0..1 toward red
	return lerp(240, 254, t), lerp(238, 226, t), lerp(245, 226, t)
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

// boundarySegment returns the endpoints of the decision line clipped to the plane,
// and false if the model is degenerate (no line). Unnamed results, so the analyzer
// treats it as a helper (a cell is a documented func with NAMED results).
func boundarySegment(m Model) (float64, float64, float64, float64, bool) {
	// W0·(x−c) + W1·(y−c) + B = 0, c = span/2. Solve for y across x∈[0,span].
	c := span / 2
	if math.Abs(m.W1) < 1e-9 {
		if math.Abs(m.W0) < 1e-9 {
			return 0, 0, 0, 0, false
		}
		// vertical line: x = c − (W1·(y−c)+B)/W0, independent of y → constant x
		xv := c - m.B/m.W0
		return xv, 0, xv, span, true
	}
	yAt := func(x float64) float64 { return c - (m.W0*(x-c)+m.B)/m.W1 }
	return 0, yAt(0), span, yAt(span), true
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
