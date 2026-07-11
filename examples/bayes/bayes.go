//go:notebook
//
// Watching a posterior form.
//
// Bayesian linear regression, Bishop §3.3: the prior over weights collapses toward the
// truth as observations arrive. Drag the slider to add data; drag the data points too.
//
// This notebook exists to answer a question the queue simulator raised. Bayesian
// updating is LITERALLY CALLED updating — posterior_n = f(posterior_{n-1}, point_n).
// Having just built Prev[T], the fold looks like the obvious tool.
//
// It is the wrong tool, and using it would break the notebook.
//
// The posterior depends on the data only through its SUFFICIENT STATISTICS, and those
// are sums: ΦᵀΦ and Φᵀt. Sums are a monoid — associative, commutative, order-free. So
// the "accumulated state" is a pure function of the data, and the data is already a
// value in the graph. There is nothing to accumulate ACROSS EPOCHS. `posterior` is an
// ordinary cell.
//
//     Path-dependent state (a queue depth, a PRNG walk) → Prev[T].
//     A sufficient statistic (a sum, a count, a max) → an ordinary cell.
//
// The test is whether the accumulator is a pure function of a value the graph already
// holds. If it is, a fold is not just unnecessary, it is HARMFUL — and here is the
// concrete harm: a fold can only go forward. Scrub the slider back from 40 points to 5
// and a fold has to replay from zero. A pure cell just... recomputes. Scrubbing
// backward — the thing that makes this demo worth looking at — only works because
// nothing here is stateful.
//
// The other half is randomness, and it splits the same way. The queue's PRNG had to
// live inside the fold, because a random walk IS path-dependent. Here the randomness
// is not: `seed` is an ordinary input leaf, and every cell that samples takes it as an
// argument. Pure, memoizable, reproducible, and no global RNG anywhere — which is the
// hidden-state bug that np.random ships by default.
//
// One more thing, and it is the third appearance of the same move: `evidence` computes
// CUMULATIVE sufficient statistics and does not depend on the slider. So `posterior` at
// any n is an O(1) lookup rather than an O(n) sum. Identical in shape to hoisting the
// seam order above the scale slider. Whenever a slider indexes into a prefix of
// something, the prefix belongs in its own cell.

package bayes

import (
	"fmt"
	"math"
	"strings"
)

// The line we are trying to recover: t = w0 + w1·x + noise.
var truth = Weights{-0.3, 0.5}

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Random seed. Every stochastic cell takes this as an argument, so there is no
// global RNG and no hidden state. Change it to redraw the world.
func seed() (seed int64) { return 7 }

// Observations. Generated from the truth, then draggable — the grips from the curve
// editor, unchanged. Drag a point and the posterior moves.
func data(seed int64) (points Draggable[Pt]) {
	r := Rand(seed | 1)
	pts := make([]Pt, 40)
	for i := range pts {
		x := -1 + 2*r.float()
		pts[i] = Pt{x, truth.at(x) + 0.2*r.normal()}
	}
	return Draggable[Pt]{Value: pts}
}

// Observations used.
//
//notebook:slider min=0 max=40
func visible() (n int) { return 2 }

// Prior precision (α). Large α means a tight prior around zero.
//
//notebook:slider min=0.1 max=10 step=0.1
func priorPrecision() (alpha float64) { return 2.0 }

// Noise precision (β). This is 1/σ² of the observation noise.
//
//notebook:slider min=1 max=100
func noisePrecision() (beta float64) { return 25.0 }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Cumulative sufficient statistics. Note what is NOT a parameter here: the slider.
// The evidence contributed by the first n points is a prefix of this, so this cell
// runs once per data change, not once per slider move.
func evidence(points Draggable[Pt]) (prefix Prefix) {
	prefix.Phi = make([]Mat2, len(points.Value)+1)
	prefix.T = make([]Vec2, len(points.Value)+1)
	for i, p := range points.Value {
		phi := basis(p.X)
		prefix.Phi[i+1] = prefix.Phi[i].add(phi.outer(phi))
		prefix.T[i+1] = prefix.T[i].add(phi.scale(p.Y))
	}
	return prefix
}

// Posterior over the weights, given the first n observations.
//
// Bishop 3.53–3.54:  S⁻¹ = αI + β·ΦᵀΦ ,  m = β·S·Φᵀt
// Two lookups and a 2×2 inverse. O(1) in n — that is what hoisting the prefix bought.
func posterior(prefix Prefix, n int, alpha, beta float64) (post Posterior) {
	precision := eye(alpha).add(prefix.Phi[n].scale(beta))
	cov := precision.inv()
	return Posterior{
		Mean: cov.apply(prefix.T[n].scale(beta)),
		Cov:  cov,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// Weight space. The prior contracts onto the truth (the cross) as evidence arrives.
//
//notebook:row=panels
func parameterSpace(post Posterior) (wspace Chart) {
	c := Chart{Title: "posterior over (w₀, w₁)", XLo: -1, XHi: 1, YLo: -1, YHi: 1}
	// Density on a grid — a 2×2 Gaussian, evaluated directly. No scipy required.
	for i := range 64 {
		for j := range 64 {
			w := Vec2{-1 + 2*float64(i)/63, -1 + 2*float64(j)/63}
			c.Field = append(c.Field, Cell{X: w[0], Y: w[1], V: post.density(w)})
		}
	}
	c.Cross = &Pt{truth[0], truth[1]}
	return c
}

// Data space. Six lines drawn from the posterior, plus the observations so far.
//
//notebook:row=panels
func dataSpace(points Draggable[Pt], n int, post Posterior, seed int64) (xspace Chart) {
	c := Chart{Title: "lines sampled from the posterior", XLo: -1, XHi: 1, YLo: -1.5, YHi: 1.5}

	// Sampling needs randomness; randomness needs a seed; the seed is an argument.
	// Nothing about this cell is impure, so it memoizes like any other.
	r := Rand(uint64(seed)*2654435761 + 1)
	l := post.Cov.chol()
	for range 6 {
		w := post.Mean.add(l.apply(Vec2{r.normal(), r.normal()}))
		c.Lines = append(c.Lines, [2]Pt{{-1, Weights(w).at(-1)}, {1, Weights(w).at(1)}})
	}

	for i, p := range points.Value {
		if i < n {
			c.Grips = append(c.Grips, Handle{At: p, Ref: points.Grip(i)})
		}
	}
	return c
}

// Watching a posterior form.
func intro() (md Markdown) {
	return `With no data, every line is plausible and the posterior is the prior. Each
observation multiplies in a likelihood; the cloud in weight space contracts toward the
cross, and the sampled lines converge on the truth.

Slide **observations used** back and forth. Backwards works — which it would not if any
of this were a fold.`
}

// ===========================================================================
// Math. Two dimensions, so everything closes in form.
// ===========================================================================

type (
	Vec2    [2]float64
	Mat2    [2][2]float64
	Weights Vec2
)

func basis(x float64) Vec2 { return Vec2{1, x} } // φ(x) = [1, x]

func (w Weights) at(x float64) float64 { return w[0] + w[1]*x }

func eye(s float64) Mat2 { return Mat2{{s, 0}, {0, s}} }

func (a Mat2) add(b Mat2) Mat2 {
	return Mat2{{a[0][0] + b[0][0], a[0][1] + b[0][1]}, {a[1][0] + b[1][0], a[1][1] + b[1][1]}}
}

func (a Mat2) scale(s float64) Mat2 {
	return Mat2{{a[0][0] * s, a[0][1] * s}, {a[1][0] * s, a[1][1] * s}}
}

func (a Mat2) inv() Mat2 {
	d := a[0][0]*a[1][1] - a[0][1]*a[1][0]
	if d == 0 {
		d = 1e-12
	}
	return Mat2{{a[1][1] / d, -a[0][1] / d}, {-a[1][0] / d, a[0][0] / d}}
}

func (a Mat2) apply(v Vec2) Vec2 {
	return Vec2{a[0][0]*v[0] + a[0][1]*v[1], a[1][0]*v[0] + a[1][1]*v[1]}
}

// chol is the lower Cholesky factor, used to draw correlated samples.
func (a Mat2) chol() Mat2 {
	l00 := math.Sqrt(math.Max(a[0][0], 1e-12))
	l10 := a[1][0] / l00
	l11 := math.Sqrt(math.Max(a[1][1]-l10*l10, 1e-12))
	return Mat2{{l00, 0}, {l10, l11}}
}

func (v Vec2) add(o Vec2) Vec2      { return Vec2{v[0] + o[0], v[1] + o[1]} }
func (v Vec2) scale(s float64) Vec2 { return Vec2{v[0] * s, v[1] * s} }
func (v Vec2) outer(o Vec2) Mat2 {
	return Mat2{{v[0] * o[0], v[0] * o[1]}, {v[1] * o[0], v[1] * o[1]}}
}

type Prefix struct {
	Phi []Mat2 // cumulative ΦᵀΦ; Phi[n] covers the first n points
	T   []Vec2 // cumulative Φᵀt
}

type Posterior struct {
	Mean Vec2
	Cov  Mat2
}

func (p Posterior) density(w Vec2) float64 {
	d := Vec2{w[0] - p.Mean[0], w[1] - p.Mean[1]}
	prec := p.Cov.inv()
	q := d[0]*(prec[0][0]*d[0]+prec[0][1]*d[1]) + d[1]*(prec[1][0]*d[0]+prec[1][1]*d[1])
	return math.Exp(-0.5 * q)
}

// Rand is xorshift64*, seeded from an input rather than a global.
type Rand uint64

func (r *Rand) next() uint64 {
	x := uint64(*r)
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*r = Rand(x)
	return x * 2685821657736338717
}

func (r *Rand) float() float64 { return float64(r.next()>>11) / (1 << 53) }

func (r *Rand) normal() float64 {
	u1 := math.Max(r.float(), 1e-12)
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*r.float())
}

// ===========================================================================
// Rendering
// ===========================================================================

type Pt struct{ X, Y float64 }
type Cell struct{ X, Y, V float64 }

type Draggable[T any] struct {
	Value []T
	at    leafToken
}

func (d Draggable[T]) Grip(i int) Ref { return Ref{leaf: d.at, index: i} }

type leafToken uint64
type Ref struct {
	leaf  leafToken
	index int
}
type Handle struct {
	At  Pt
	Ref Ref
}

type Chart struct {
	Title                  string
	XLo, XHi, YLo, YHi     float64
	Field                  []Cell     // density grid
	Lines                  [][2]Pt    // sampled regression lines
	Grips                  []Handle   // draggable observations
	Cross                  *Pt        // the truth
}

func (c Chart) Render() Rendered {
	const w, h, pad = 350.0, 350.0, 34.0
	sx := func(v float64) float64 { return pad + (v-c.XLo)/(c.XHi-c.XLo)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - (v-c.YLo)/(c.YHi-c.YLo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	if len(c.Field) > 0 {
		cw := (w - 2*pad) / 63
		for _, f := range c.Field {
			if f.V < 0.01 {
				continue
			}
			fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" `+
				`fill="#4338ca" fill-opacity="%.3f"/>`,
				sx(f.X)-cw/2, sy(f.Y)-cw/2, cw+0.5, cw+0.5, math.Min(f.V, 1)*0.85)
		}
	}
	for _, l := range c.Lines {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" `+
			`stroke="#c026d3" stroke-width="1.5" stroke-opacity="0.65"/>`,
			sx(l[0].X), sy(l[0].Y), sx(l[1].X), sy(l[1].Y))
	}
	for _, g := range c.Grips {
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#fff" stroke="#0f172a" `+
			`stroke-width="1.5" data-grip=%q style="cursor:grab"/>`, sx(g.At.X), sy(g.At.Y), g.Ref)
	}
	if c.Cross != nil {
		x, y := sx(c.Cross.X), sy(c.Cross.Y)
		fmt.Fprintf(&b, `<path d="M%.1f %.1f L%.1f %.1f M%.1f %.1f L%.1f %.1f" `+
			`stroke="#f8fafc" stroke-width="2.5"/>`, x-7, y, x+7, y, x, y-7, x, y+7)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="18" font-family="sans-serif" font-size="11">%s</text>`,
		pad, c.Title)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
