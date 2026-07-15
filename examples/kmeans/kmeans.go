//go:notebook
//
// k-means, and why initialization ruins your day.
//
// k-means clusters points by repeating two steps (Lloyd's algorithm): assign each
// point to its nearest centroid, then move each centroid to the mean of its points.
// It always converges. It does NOT always converge to the same thing — where it
// lands depends entirely on where the centroids START. That is the footgun every
// practitioner hits and no tutorial lets you feel, because a tutorial runs it once.
//
// Here you can feel it two ways:
//
//   - **Drag the initial centroids.** They're grips. Move them and watch the final
//     clustering change — sometimes cleanly splitting the blobs, sometimes wedging
//     one centroid between two clusters while another starves. Same data, same k,
//     different answer, because you moved the start.
//   - **Scrub the seed.** The seed places the initial centroids randomly; step it
//     and watch the *converged* inertia (total within-cluster distance²) jump around.
//     Lower is better, and two seeds an integer apart can differ by a lot. That jump
//     IS the local-minimum problem — the reason real k-means restarts from many seeds
//     and keeps the best.
//
// The mechanism it puts on stage: **fixed-horizon iteration as a pure cell.** Lloyd's
// algorithm is a loop, but the notebook runs it to convergence INSIDE one cell —
// `cluster` is a pure function of (points, initial centroids, iterations). Drag a
// centroid and it re-runs from scratch; scrub the iteration slider and you see the
// algorithm mid-flight, exactly, at any step, both directions. A fold would only go
// forward; here every frame is recomputed, so you can rewind the convergence.

package kmeans

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	plane = 100.0
	kMax  = 6
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Number of clusters k. The centroids cell re-seeds to match; the data has three
// natural blobs, so k=3 fits — try k=2 or k=5 and watch it over/under-split.
//
//notebook:slider min=2 max=6 step=1
func clusterCount() (k int) { return 3 }

// Seed for the initial centroid placement. Step it and watch the converged inertia
// jump — that jump is the local-minimum problem. A leaf, not a global RNG.
//
//notebook:slider min=1 max=40 step=1
func seed() (s int) { return 3 }

// Iterations of Lloyd's algorithm — the scrub axis. 0 shows the raw start; step up
// to watch assign/update converge; the clustering usually settles within ~8 steps.
//
//notebook:slider min=0 max=20 step=1
func iterations() (steps int) { return 12 }

// The initial centroids — a draggable leaf, seeded randomly from (k, seed). Drag one
// and the whole clustering re-runs from your placement. When k or seed changes the
// arity changes and the leaf reconciles to the fresh seeding.
//
//notebook:height=480
func centroids(k int, s int) (init Draggable[Pt]) {
	rng := newLCG(uint64(s)*1000 + uint64(k))
	pts := make([]Pt, k)
	for i := range pts {
		pts[i] = Pt{X: 10 + rng.f()*80, Y: 10 + rng.f()*80}
	}
	return Draggable[Pt]{Value: pts}
}

// ---------------------------------------------------------------------------
// Data + compute (Go), pure.
// ---------------------------------------------------------------------------

// The data: three Gaussian-ish blobs, fixed (a constant seed so the points don't
// move when you drag centroids — only the clustering should change). Deterministic.
func data() (points []Pt) {
	rng := newLCG(20240714)
	centers := []Pt{{28, 30}, {70, 35}, {50, 74}}
	for _, c := range centers {
		for i := 0; i < 40; i++ {
			points = append(points, Pt{
				X: c.X + (rng.f()-0.5)*26,
				Y: c.Y + (rng.f()-0.5)*26,
			})
		}
	}
	return points
}

// The clustering: run `steps` of Lloyd's algorithm from the initial centroids over
// the data, and report the assignment, the moved centroids, and the inertia (total
// within-cluster squared distance — the thing k-means minimizes). Pure in (points,
// init, steps): drag re-runs from scratch, and scrubbing steps shows the algorithm
// at exactly that iteration, both directions. No fold.
func cluster(points []Pt, init Draggable[Pt], steps int) (result Clustering) {
	cent := append([]Pt(nil), init.Value...)
	k := len(cent)
	assign := make([]int, len(points))

	for it := 0; it < steps; it++ {
		// assign step
		for i, p := range points {
			assign[i] = nearest(p, cent)
		}
		// update step: each centroid → mean of its points (empty clusters stay put)
		sumX := make([]float64, k)
		sumY := make([]float64, k)
		count := make([]int, k)
		for i, p := range points {
			a := assign[i]
			sumX[a] += p.X
			sumY[a] += p.Y
			count[a]++
		}
		for c := 0; c < k; c++ {
			if count[c] > 0 {
				cent[c] = Pt{sumX[c] / float64(count[c]), sumY[c] / float64(count[c])}
			}
		}
	}
	// final assignment against the settled centroids (so step 0 shows the raw start)
	for i, p := range points {
		assign[i] = nearest(p, cent)
	}
	return Clustering{Centroids: cent, Assign: assign, Inertia: inertia(points, cent, assign)}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The clustering: points coloured by assignment, the settled centroids as ✕, and the
// draggable INITIAL centroids as grips. Drag a grip and watch the whole thing re-run
// from the new start — the same data can cluster differently.
//
//notebook:height=480
func plot(points []Pt, init Draggable[Pt], result Clustering) (chart Chart) {
	chart = Chart{Points: points, Assign: result.Assign, Centroids: result.Centroids}
	for i, p := range init.Value {
		chart.Grips = append(chart.Grips, Handle{At: p, Ref: init.Grip(i)})
	}
	return chart
}

// The numbers: converged inertia (lower is better), and k. Step the seed slider and
// watch inertia jump between seeds — different local minima, same data and k.
func readout(result Clustering) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "inertia", Value: f0(result.Inertia), Caption: "total within-cluster distance² — lower is better; it jumps between seeds"},
		{Label: "k", Value: strconv.Itoa(len(result.Centroids))},
	}}
}

// k-means, and why initialization ruins your day.
func intro() (md Markdown) {
	return `k-means repeats two steps until it settles: assign each point to its
nearest centroid, move each centroid to its points' mean. It always converges — but
**where it lands depends on where the centroids start.**

Drag the initial centroids (the ✕ grips) and watch the final clustering change: same
points, same *k*, a different answer. Or step the **seed** slider and watch the
converged **inertia** jump — each seed is a different local minimum. That jump is why
real k-means restarts from many seeds and keeps the best; here you can watch it
happen instead of reading about it.

Lloyd's algorithm is a loop, but ` + "`cluster`" + ` runs it to convergence inside one
cell — a pure function of (points, start, iterations) — so drag re-runs from scratch
and the iteration slider rewinds the convergence exactly. No fold.`
}

// ===========================================================================
// k-means helpers
// ===========================================================================

func nearest(p Pt, cent []Pt) int {
	best, bi := math.Inf(1), 0
	for i, c := range cent {
		d := (p.X-c.X)*(p.X-c.X) + (p.Y-c.Y)*(p.Y-c.Y)
		if d < best {
			best, bi = d, i
		}
	}
	return bi
}

func inertia(points, cent []Pt, assign []int) float64 {
	var sum float64
	for i, p := range points {
		c := cent[assign[i]]
		sum += (p.X-c.X)*(p.X-c.X) + (p.Y-c.Y)*(p.Y-c.Y)
	}
	return sum
}

// ===========================================================================
// RNG — a small LCG, so seeding is deterministic (no global rand).
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

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }

// palette maps a cluster index to a distinct colour. Unnamed return so it's a helper.
func palette(i int) string {
	colors := []string{"#2563eb", "#dc2626", "#16a34a", "#d97706", "#7c3aed", "#0891b2"}
	return colors[i%len(colors)]
}

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

// Clustering is the result of running Lloyd's algorithm: the settled centroids, the
// per-point assignment, and the inertia (the objective).
type Clustering struct {
	Centroids []Pt
	Assign    []int
	Inertia   float64
}

// Draggable — curvefit's grip leaf; here the initial centroids are dragged.
type Draggable[T any] struct {
	Value []T
	leaf  string
}

func (d Draggable[T]) WithLeaf(sym string) Draggable[T] { d.leaf = sym; return d }
func (d Draggable[T]) Grip(i int) Ref                   { return Ref{Leaf: d.leaf, Index: i} }

// Reconcile RESETS on an arity change: when k changes the centroid count changes, so
// saved positions for a different k don't fit — the fresh seeding stands. When the
// count matches, adopt the dragged positions.
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

// Chart draws the points coloured by cluster, the settled centroids as ✕, and the
// draggable initial centroids as grips.
type Chart struct {
	Points    []Pt
	Assign    []int
	Centroids []Pt
	Grips     []Handle
}

func (c Chart) Render() Rendered {
	const box = 480.0
	sx := func(v float64) float64 { return v / plane * box }
	sy := func(v float64) float64 { return box - v/plane*box }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, box, box)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, box, box)
	fmt.Fprintf(&b, `<rect x="1" y="1" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`, box-2, box-2)

	// points, coloured by assignment
	for i, p := range c.Points {
		col := "#94a3b8"
		if i < len(c.Assign) {
			col = palette(c.Assign[i])
		}
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3.5" fill=%q fill-opacity="0.65"/>`, sx(p.X), sy(p.Y), col)
	}
	// settled centroids as ✕
	for i, cc := range c.Centroids {
		col := palette(i)
		x, y := sx(cc.X), sy(cc.Y)
		fmt.Fprintf(&b, `<path d="M%.1f %.1f L%.1f %.1f M%.1f %.1f L%.1f %.1f" stroke=%q stroke-width="3"/>`,
			x-7, y-7, x+7, y+7, x-7, y+7, x+7, y-7, col)
	}
	// draggable initial centroids as ringed grips
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="9" fill="none" stroke="#0f172a" stroke-width="2" `+
			`stroke-dasharray="3 2" data-grip=%q style="cursor:grab"/>`, sx(g.At.X), sy(g.At.Y), string(ref))
	}
	fmt.Fprintf(&b, `<text x="12" y="24" font-family="sans-serif" font-size="12" fill="#334155">`+
		`dashed = initial (drag me) · ✕ = settled centroid</text>`)
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
