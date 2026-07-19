//go:notebook
//
// draggable — direct-manipulation points you drag on a chart.
//
// A type with a slice `Value` field and a `Grip(i)` method renders as a
// draggable: each element is a handle the user drags. The mechanism has three
// moving parts, and they are separate on purpose:
//
//   - the LEAF (`points`) holds the draggable data. Its identity is stamped by
//     the runtime through WithLeaf — the author never writes the leaf's name.
//   - a VIEW cell (`chart`) draws the points and emits a Handle per point:
//     "draw a grip here; a drag writes to that leaf element". The renderer can
//     only *request* the write, never perform it — which is the structural
//     reason grips can't grow into a callback system.
//   - a DERIVED cell (`centroid`) is a pure function of the dragged positions.
//
// Because the view that draws the grips is a *different* cell from the leaf that
// owns them, the leaf identity has to ride with the value across that boundary —
// that is what Grip(i) carries (it marshals to "leaf:index" only at the wire).
//
//	go tool notebook run ./examples/minimal/draggable
//
// Demonstrates: Grip() -> draggable, grip Handles in Render, flat-array Reconcile.
// See docs/reference-controls.html.

package draggable

import (
	"fmt"
	"strconv"
	"strings"
)

// Three points you can drag. This is the leaf.
func points() (pts Draggable) {
	return Draggable{Value: []Pt{{20, 30}, {50, 70}, {80, 40}}}
}

// The mean point — a pure function of wherever the points currently sit.
func centroid(pts Draggable) (mean Pt) {
	var sx, sy float64
	for _, p := range pts.Value {
		sx, sy = sx+p.X, sy+p.Y
	}
	n := float64(len(pts.Value))
	return Pt{X: sx / n, Y: sy / n}
}

// The chart draws the points as draggable grips, plus the centroid as a fixed
// marker. It takes both the leaf and the derived mean, so it redraws live.
//
//notebook:height=380
func chart(pts Draggable, mean Pt) (view Chart) {
	c := Chart{Mean: mean}
	for i, p := range pts.Value {
		// Grip(i) is a typed reference to element i of THIS leaf — no string in
		// author code; the leaf name appears only when Ref marshals to the wire.
		c.Grips = append(c.Grips, Handle{At: p, Ref: pts.Grip(i)})
	}
	return c
}

// ===========================================================================
// The draggable leaf type. Value + Grip() make it a draggable; WithLeaf is the
// runtime stamping seam; Reconcile keeps the dragged positions across a wave.
// ===========================================================================

type Pt struct{ X, Y float64 }

type Draggable struct {
	Value []Pt
	leaf  string // stamped by the runtime via WithLeaf, never by the author
}

// WithLeaf is the runtime stamping seam (value semantics). The runtime calls it
// when it materializes the leaf; a notebook calling it itself is a smell.
func (d Draggable) WithLeaf(sym string) Draggable { d.leaf = sym; return d }

// Grip is a typed reference to element i of this leaf.
func (d Draggable) Grip(i int) Ref { return Ref{Leaf: d.leaf, Index: i} }

// Reconcile keeps the dragged positions when the point count is unchanged. The
// saved selection arrives as a flat [x0,y0,x1,y1,…] float array.
func (d Draggable) Reconcile(saved any) any {
	flat, ok := saved.([]float64)
	if !ok || len(flat) != 2*len(d.Value) {
		return d // arity changed or wrong shape — the fresh seed stands
	}
	out := make([]Pt, len(d.Value))
	for i := range out {
		out[i] = Pt{X: flat[2*i], Y: flat[2*i+1]}
	}
	d.Value = out
	return d
}

// WidgetView carries the draggable's live positions on the wire.
func (d Draggable) WidgetView() WidgetView { return WidgetView{Value: d.Value} }

// Ref is a reference to one draggable element. It marshals to "leaf:index" — the
// wire form the client parses to route a drag back to the right leaf element.
type Ref struct {
	Leaf  string
	Index int
}

func (r Ref) MarshalText() ([]byte, error) { return []byte(r.Leaf + ":" + strconv.Itoa(r.Index)), nil }

// Handle is a declarative request: "draw a grip here; dragging it writes to that
// leaf element." The renderer emits it; it cannot perform the write.
type Handle struct {
	At  Pt
	Ref Ref
}

// WidgetView is a widget's state on the wire — matched structurally by the runtime.
type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

// Chart draws the draggable points and the centroid. fmt lives here, in Render.
type Chart struct {
	Mean  Pt
	Grips []Handle
}

func (c Chart) Render() Rendered {
	const w, h, pad = 320.0, 320.0, 20.0
	sx := func(v float64) float64 { return pad + v/100*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/100*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// The centroid — a fixed marker that follows the dragged points.
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="none" stroke="#d0433b" stroke-width="2"/>`,
		sx(c.Mean.X), sy(c.Mean.Y))

	// One draggable circle per point. data-grip="leaf:index" routes the drag.
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="#2a78d6" fill-opacity="0.8" `+
			`stroke="#fff" stroke-width="1.5" data-grip=%q style="cursor:grab"/>`,
			sx(g.At.X), sy(g.At.Y), string(ref))
	}
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// Rendered is the tiny display envelope, redeclared locally (no import).
type Rendered struct{ MIME, Data string }
