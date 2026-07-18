//go:notebook
//
// Curve fitting by hand.
//
// This is the falsification test for the design, not a port. marimo's gallery has a
// whole category of these — draggable control points, draggable pucks, draw-edges,
// 2D sliders, drawing canvases — and they share one shape:
//
//     the widget's value IS the data, and you edit it by manipulating the very
//     output it produces.
//
// That looks like a cycle, and it is where "reactivity lives only in the graph" should
// break. It doesn't, and the reason is worth stating plainly:
//
//     The RENDERER reads the leaf. The RUNTIME writes it. A write is not an edge.
//
// So `editor` depends on `ctrl` (it draws the handles) but `ctrl` does not depend on
// `editor` (dragging a handle is an edit to the head, exactly like moving a slider).
// The graph stays acyclic. Nothing new was added: `//notebook:brush on=scatter` from the
// Lego port was this mechanism's special case, and this generalizes it — a pure
// renderer emits GRIPS, and the runtime binds them to leaf writes.
//
// Three things are stressed here that no previous notebook touched:
//
//   1. The leaf has a DATA-DERIVED DEFAULT (a least-squares fit) and is still editable.
//   2. The leaf's ARITY CHANGES when you move the degree slider — the head's saved
//      positions no longer fit the schema, and must reconcile.
//   3. The handle must name the leaf it writes to. Doing that with a string would
//      re-introduce exactly the stringly-typed coupling the Lego port eliminated.
//      Draggable[T] carries an opaque token instead, stamped by the runtime when it
//      materialized the leaf — so the leaf's identity flows WITH the value, and
//      Grip(i) is type-checked. This is only possible because widgets are values.
//
// marimo does all of this by dropping to anywidget, i.e. by writing JavaScript.
// Here it is one Go cell and no JS at all.

package curvefit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Cells
// ---------------------------------------------------------------------------

// Observations to fit.
func samples() (obs []Pt) {
	// A deterministic wobble — no RNG, because a random leaf is not memoizable
	// unless the seed is an input, and that is a decision I have not made yet.
	obs = make([]Pt, 0, 40)
	for i := range 40 {
		x := float64(i) / 39
		y := 0.5 + 0.42*math.Sin(2.4*math.Pi*x)*math.Exp(-1.1*x) +
			0.03*math.Sin(31*x) - 0.02*math.Cos(17*x)
		obs = append(obs, Pt{x, y})
	}
	return obs
}

// Control points.
//
//notebook:slider min=3 max=9
func degree() (n int) { return 5 }

// Control points. Seeded by a least-squares Bézier fit — then dragged by hand.
//
// This cell computes the SCHEMA (how many points, and where they start).
// The head holds your edits. When degree changes, the arity changes, and the runtime
// reconciles: positions beyond the new arity are dropped, missing ones are taken from
// this cell's fresh default. Identical rule to Multi[Theme] dropping options that no
// longer exist — one reconcile rule, three widget kinds.
func controlPoints(obs []Pt, n int) (ctrl Draggable[Pt]) {
	return Draggable[Pt]{Value: leastSquaresBezier(obs, n)}
}

// The fitted curve.
func curve(ctrl Draggable[Pt]) (fit []Pt) {
	pts := make([]Pt, 0, 160)
	for i := range 160 {
		pts = append(pts, deCasteljau(ctrl.Value, float64(i)/159))
	}
	return pts
}

// Drag the control points.
//
//notebook:height=420
func editor(obs []Pt, ctrl Draggable[Pt], fit []Pt) (plot Chart) {
	plot = Chart{Samples: obs, Curve: fit, Hull: ctrl.Value}
	for i, p := range ctrl.Value {
		// ctrl.Grip(i) is a typed reference to element i of THIS leaf. The renderer
		// says where the handle is; the runtime decides what dragging it means.
		plot.Grips = append(plot.Grips, Handle{At: p, Ref: ctrl.Grip(i)})
	}
	return plot
}

// Fit quality.
func residual(obs []Pt, fit []Pt) (rms float64) {
	var sum float64
	for _, o := range obs {
		d := math.Inf(1)
		for _, f := range fit {
			d = math.Min(d, math.Hypot(o.X-f.X, o.Y-f.Y))
		}
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(obs)))
}

// Curve fitting by hand.
func intro() (md Markdown) {
	return `The control points start where least squares puts them. Drag them anywhere.
The curve, the convex hull, and the RMS residual all follow.

Nothing here is bound two ways. The editor *reads* the control points to draw the
handles; dragging a handle is an edit, the same kind of event as moving a slider.`
}

// ===========================================================================
// Math
// ===========================================================================

// deCasteljau evaluates a Bézier curve at t.
func deCasteljau(ctrl []Pt, t float64) Pt {
	buf := append([]Pt(nil), ctrl...)
	for k := len(buf) - 1; k > 0; k-- {
		for i := range k {
			buf[i] = Pt{
				X: buf[i].X + t*(buf[i+1].X-buf[i].X),
				Y: buf[i].Y + t*(buf[i+1].Y-buf[i].Y),
			}
		}
	}
	return buf[0]
}

// leastSquaresBezier fits n control points to obs by chord-length parameterization
// and normal equations. This is the "you need scipy" step; it is forty lines.
func leastSquaresBezier(obs []Pt, n int) []Pt {
	m := len(obs)
	if m < n {
		return nil
	}
	t := chordLength(obs)

	// Bernstein design matrix.
	b := make([][]float64, m)
	for i := range m {
		b[i] = make([]float64, n)
		for j := range n {
			b[i][j] = bernstein(n-1, j, t[i])
		}
	}
	// Normal equations: (BᵀB) c = Bᵀ v, solved once per coordinate.
	bt := make([][]float64, n)
	for j := range n {
		bt[j] = make([]float64, n)
		for k := range n {
			for i := range m {
				bt[j][k] += b[i][j] * b[i][k]
			}
		}
	}
	rhs := func(get func(Pt) float64) []float64 {
		r := make([]float64, n)
		for j := range n {
			for i := range m {
				r[j] += b[i][j] * get(obs[i])
			}
		}
		return r
	}
	xs := solve(clone2(bt), rhs(func(p Pt) float64 { return p.X }))
	ys := solve(clone2(bt), rhs(func(p Pt) float64 { return p.Y }))

	out := make([]Pt, n)
	for j := range n {
		out[j] = Pt{xs[j], ys[j]}
	}
	return out
}

func bernstein(n, j int, t float64) float64 {
	return float64(choose(n, j)) * math.Pow(t, float64(j)) * math.Pow(1-t, float64(n-j))
}

func choose(n, k int) int {
	c := 1
	for i := range k {
		c = c * (n - i) / (i + 1)
	}
	return c
}

// chordLength assigns each observation a parameter proportional to distance travelled.
func chordLength(obs []Pt) []float64 {
	t := make([]float64, len(obs))
	for i := 1; i < len(obs); i++ {
		t[i] = t[i-1] + math.Hypot(obs[i].X-obs[i-1].X, obs[i].Y-obs[i-1].Y)
	}
	if total := t[len(t)-1]; total > 0 {
		for i := range t {
			t[i] /= total
		}
	}
	return t
}

// solve does Gaussian elimination with partial pivoting. It replaces numpy.linalg.
func solve(a [][]float64, v []float64) []float64 {
	n := len(v)
	for c := range n {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		a[c], a[p] = a[p], a[c]
		v[c], v[p] = v[p], v[c]
		if a[c][c] == 0 {
			continue
		}
		for r := c + 1; r < n; r++ {
			f := a[r][c] / a[c][c]
			for k := c; k < n; k++ {
				a[r][k] -= f * a[c][k]
			}
			v[r] -= f * v[c]
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := v[r]
		for k := r + 1; k < n; k++ {
			s -= a[r][k] * x[k]
		}
		if a[r][r] != 0 {
			x[r] = s / a[r][r]
		}
	}
	return x
}

func clone2(m [][]float64) [][]float64 {
	out := make([][]float64, len(m))
	for i, r := range m {
		out[i] = append([]float64(nil), r...)
	}
	return out
}

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

// Draggable is a leaf you can manipulate directly on a chart. Its grips are
// drawn by a DIFFERENT cell (editor draws handles for these control points), so
// the leaf's identity must ride WITH the value across that cell boundary — that
// is what Grip(i) carries. The identity is stamped by the runtime, never by the
// author: WithLeaf is a runtime seam (the runtime writes leaf identity; the
// notebook reads it via Grip).
type Draggable[T any] struct {
	Value []T
	leaf  string // the leaf symbol, stamped by the runtime via WithLeaf
}

// WithLeaf is the runtime stamping seam (value semantics — returns a copy). The
// runtime calls it when it materializes this leaf; a notebook calling it is a
// smell.
func (d Draggable[T]) WithLeaf(sym string) Draggable[T] { d.leaf = sym; return d }

// Grip is a typed reference to element i of THIS leaf — no string in the author's
// code; the symbol appears only at the wire (Ref marshals to "leaf:index").
func (d Draggable[T]) Grip(i int) Ref { return Ref{Leaf: d.leaf, Index: i} }

// Reconcile RESETS the control points on an arity change: point #3 of a quintic
// is not point #3 of a septic, so partial retention is incoherent. When the
// saved selection has the same count as the fresh schema, adopt it; otherwise
// the fresh default stands. This is the draggable's place in the taxonomy.
func (d Draggable[T]) Reconcile(saved any) any {
	// The selection arrives as a flat [x0,y0,x1,y1,...] — the wire carries
	// primitives; this notebook owns the point shape and pairs them back.
	flat, ok := saved.([]float64)
	if !ok || len(flat) != 2*len(d.Value) {
		return d // arity changed (or unusable) → reset to the fresh schema
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

// WidgetView states the Draggable's live state: its point positions as the
// selection. State only — the chart (a Render) draws them; this carries the data.
func (d Draggable[T]) WidgetView() WidgetView { return WidgetView{Value: d.Value} }

type Ref struct {
	Leaf  string
	Index int
}

// MarshalText renders a grip ref as "leaf:index" for the SVG data-grip attribute
// — the wire form the client parses to route a drag to the right leaf.
func (r Ref) MarshalText() ([]byte, error) {
	return []byte(r.Leaf + ":" + itoa(r.Index)), nil
}

// Handle is a declarative request: "draw a grip here; dragging it writes to that leaf."
// The renderer emits it. The renderer does not, and cannot, perform the write —
// which is the structural reason this cannot grow into a callback system.
type Handle struct {
	At  Pt
	Ref Ref
}

type Chart struct {
	Samples []Pt
	Curve   []Pt
	Hull    []Pt
	Grips   []Handle
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 420.0, 40.0
	sx := func(v float64) float64 { return pad + v*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	path := func(pts []Pt, stroke, dash string, width float64) {
		if len(pts) == 0 {
			return
		}
		var d strings.Builder
		for i, p := range pts {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(p.X), sy(p.Y))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="%.1f" stroke-dasharray=%q/>`,
			d.String(), stroke, width, dash)
	}

	path(c.Hull, "#0797b8", "5 4", 1) // control polygon
	path(c.Curve, "#2a78d6", "", 2.5) // the fitted curve
	for _, p := range c.Samples {
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="2.5" fill="#1b3a6b" fill-opacity="0.45"/>`,
			sx(p.X), sy(p.Y))
	}
	// Grips. data-grip = "leaf:index" is how the client routes a drag to the leaf
	// this handle writes; the notebook never wrote the leaf symbol (the runtime
	// stamped it, Grip carries it, MarshalText renders it here).
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="#fff" stroke="#0797b8" `+
			`stroke-width="2.5" data-grip=%q style="cursor:grab"/>`,
			sx(g.At.X), sy(g.At.Y), string(ref))
	}
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// WidgetView is a widget's state on the wire — matched structurally by the
// runtime (like Rendered). State only; each kind fills what it uses.
type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

func itoa(n int) string { return strconv.Itoa(n) }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
