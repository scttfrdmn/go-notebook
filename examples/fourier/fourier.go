//go:notebook
//
// Fourier epicycles — draw a shape, watch circles trace it back.
//
// Any closed curve is a sum of rotating circles. Take the shape, walk around it as
// a stream of complex points, and the discrete Fourier transform hands you a circle
// for each frequency: a radius (how big) and a phase (where it starts). Stack the
// circles tip to tail, spin each at its integer frequency, and the end of the chain
// retraces the curve. This is the animation everyone has seen; here it is the math
// behind it, and you can drag the shape and scrub the circles.
//
//   - **Drag the outline.** The shape is a draggable polygon (a star, to start). Move
//     a vertex and the whole spectrum recomputes — the circles you'd need change with
//     the curve they trace.
//   - **Scrub the number of terms, both ways.** With a few circles the reconstruction
//     is a rounded blob; add more and it sharpens onto the corners. Watch **Gibbs
//     ringing** — the little overshoot ripples near each sharp corner — grow as you
//     add terms and *vanish again* as you take them away. That reversibility is the
//     point: the reconstruction is a pure function of (shape, terms), not a fold, so
//     you slide back and forth through the approximation. A fold could only sharpen
//     forward; here the ringing is a thing you can summon and dismiss.
//
// The bayes lesson (sufficient statistics, scrub both ways) on the transform that is
// *made of* frequencies. The whole picture — spectrum, reconstruction, the frozen
// epicycle chain — is pure Go: DFT by hand, no library, so it compiles to wasm and
// runs in the tab.

package fourier

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	plane   = 100.0 // the drawing lives in [0, plane]²
	samples = 256   // the shape is resampled to this many points before the DFT
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// The outline. Draggable vertices of a closed polygon — seeded as a five-pointed
// star, whose sharp corners make Gibbs ringing vivid. Drag a vertex and the circles
// that trace it recompute.
//
//notebook:height=480
func shape() (outline Draggable[Pt]) {
	return Draggable[Pt]{Value: star(5, 50, 50, 38, 16)}
}

// Number of Fourier terms (circles) — the scrub axis. Few: a rounded blob. Many: the
// corners sharpen, with Gibbs ripples that appear and vanish as you slide.
//
//notebook:slider min=1 max=64 step=1
func termCount() (n int) { return 6 }

// ---------------------------------------------------------------------------
// Compute (Go) — the DFT, pure.
// ---------------------------------------------------------------------------

// The spectrum: resample the polygon to `samples` points around its perimeter, treat
// each as a complex number x+iy, and take the discrete Fourier transform. Pure — a
// function of the shape alone — and separated from the term count so scrubbing terms
// re-uses this and only the cheap reconstruction re-runs.
func spectrum(outline Draggable[Pt]) (spec Spectrum) {
	pts := resample(outline.Value, samples)
	coeffs := make([]complex128, samples)
	for n := 0; n < samples; n++ {
		var sum complex128
		for k, z := range pts {
			ang := -2 * math.Pi * float64(n) * float64(k) / float64(samples)
			sum += z * complex(math.Cos(ang), math.Sin(ang))
		}
		coeffs[n] = sum / complex(float64(samples), 0)
	}
	return Spectrum{Coeffs: coeffs}
}

// The reconstruction from the lowest `n` frequencies, plus the epicycle chain frozen
// at one phase. Pure in (spectrum, n) — which is what lets terms scrub both ways and
// makes Gibbs ringing something you can summon and dismiss.
func reconstruction(spec Spectrum, n int) (recon Recon) {
	kept := lowestFreqs(len(spec.Coeffs), n) // frequency indices: 0, ±1, ±2, …

	// Trace the reconstructed curve over one period.
	const res = 400
	curve := make([]Pt, res+1)
	for i := 0; i <= res; i++ {
		t := float64(i) / float64(res)
		var z complex128
		for _, f := range kept {
			z += spec.Coeffs[wrap(f, len(spec.Coeffs))] * expi(2*math.Pi*float64(f)*t)
		}
		curve[i] = Pt{real(z), imag(z)}
	}

	// The epicycle chain at a fixed phase: tip-to-tail sum of each term's vector,
	// with the circle each one rides. Ordered by |frequency| so big circles come
	// first — the conventional picture.
	const phase = 0.12
	chain := make([]Circle, 0, len(kept))
	var tip complex128
	for _, f := range kept {
		c := spec.Coeffs[wrap(f, len(spec.Coeffs))]
		v := c * expi(2*math.Pi*float64(f)*phase)
		chain = append(chain, Circle{CX: real(tip), CY: imag(tip), R: cabs(c)})
		tip += v
	}
	return Recon{Curve: curve, Chain: chain, TipX: real(tip), TipY: imag(tip)}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The picture: the drawn outline (faint, with draggable vertices), the reconstruction
// from the current number of circles (bold), and the frozen epicycle chain that sums
// to a point on it. Add terms and the bold curve snaps onto the corners.
//
//notebook:height=480
func view(outline Draggable[Pt], recon Recon) (plot Chart) {
	plot = Chart{Recon: recon.Curve, Chain: recon.Chain, TipX: recon.TipX, TipY: recon.TipY}
	plot.Target = append(plot.Target, outline.Value...)
	for i, p := range outline.Value {
		plot.Grips = append(plot.Grips, Handle{At: p, Ref: outline.Grip(i)})
	}
	return plot
}

// The numbers: circles in use, and the fraction of the shape's spectral energy those
// circles capture — it climbs toward 100% as you add terms.
func readout(spec Spectrum, n int) (report Readout) {
	kept := lowestFreqs(len(spec.Coeffs), n)
	var keptE, totalE float64
	for f := range spec.Coeffs {
		e := cabs(spec.Coeffs[f])
		totalE += e * e
	}
	for _, f := range kept {
		e := cabs(spec.Coeffs[wrap(f, len(spec.Coeffs))])
		keptE += e * e
	}
	frac := 0.0
	if totalE > 0 {
		frac = keptE / totalE
	}
	return Readout{Cards: []Card{
		{Label: "circles", Value: strconv.Itoa(len(kept))},
		{Label: "energy captured", Value: pct(frac), Caption: "→ 100% as circles are added"},
	}}
}

// Fourier epicycles — draw a shape, watch circles trace it back.
func intro() (md Markdown) {
	return `Any closed curve is a sum of rotating circles. Drag the star's vertices to
change the shape; scrub **terms** to change how many circles trace it.

With a few circles the reconstruction is a rounded blob. Add more and it sharpens
onto the corners — but watch the little **Gibbs ripples** overshoot near each corner,
grow as you add terms, and *vanish again* as you take them away. That reversibility
is the point: the reconstruction is a **pure function of (shape, terms)**, not a
fold, so you slide back and forth through the approximation. A fold could only sharpen
forward.`
}

// ===========================================================================
// Complex helpers
// ===========================================================================

func expi(theta float64) complex128 { return complex(math.Cos(theta), math.Sin(theta)) }
func cabs(z complex128) float64     { return math.Hypot(real(z), imag(z)) }

// lowestFreqs returns the n signed frequencies closest to zero: 0, +1, -1, +2, -2, …
func lowestFreqs(size, n int) []int {
	if n > size {
		n = size
	}
	out := make([]int, 0, n)
	out = append(out, 0)
	for f := 1; len(out) < n; f++ {
		out = append(out, f)
		if len(out) < n {
			out = append(out, -f)
		}
	}
	return out
}

// wrap maps a signed frequency to its DFT index (negative f → size+f).
func wrap(f, size int) int {
	i := f % size
	if i < 0 {
		i += size
	}
	return i
}

// resample walks the closed polygon and returns m points equally spaced by arc
// length, as complex numbers — the signal the DFT consumes.
func resample(verts []Pt, m int) []complex128 {
	if len(verts) < 2 {
		return make([]complex128, m)
	}
	// cumulative perimeter
	n := len(verts)
	seg := make([]float64, n) // length of edge i → i+1 (closed)
	total := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		seg[i] = math.Hypot(verts[j].X-verts[i].X, verts[j].Y-verts[i].Y)
		total += seg[i]
	}
	out := make([]complex128, m)
	if total == 0 {
		return out
	}
	step := total / float64(m)
	edge, along := 0, 0.0
	for k := 0; k < m; k++ {
		target := float64(k) * step
		// advance to the edge containing `target`
		acc := 0.0
		for e := 0; e < n; e++ {
			if acc+seg[e] >= target {
				edge = e
				along = target - acc
				break
			}
			acc += seg[e]
		}
		i, j := edge, (edge+1)%n
		frac := 0.0
		if seg[edge] > 0 {
			frac = along / seg[edge]
		}
		x := verts[i].X + frac*(verts[j].X-verts[i].X)
		y := verts[i].Y + frac*(verts[j].Y-verts[i].Y)
		out[k] = complex(x, y)
	}
	return out
}

// star returns the 2·points vertices of a star with the given center and radii.
func star(points int, cx, cy, outer, inner float64) []Pt {
	verts := make([]Pt, 0, 2*points)
	for i := 0; i < 2*points; i++ {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		ang := math.Pi/2 + float64(i)*math.Pi/float64(points) // start at the top
		verts = append(verts, Pt{cx + r*math.Cos(ang), cy + r*math.Sin(ang)})
	}
	return verts
}

func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

// Spectrum is the DFT of the resampled shape — one complex coefficient per frequency.
type Spectrum struct {
	Coeffs []complex128
}

// Circle is one epicycle: a circle of radius R centred at (CX, CY) in the frozen chain.
type Circle struct {
	CX, CY, R float64
}

// Recon is the reconstructed curve plus the frozen epicycle chain and its tip.
type Recon struct {
	Curve      []Pt
	Chain      []Circle
	TipX, TipY float64
}

// Draggable — curvefit's grip leaf; here the polygon vertices are dragged.
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

// Chart draws the target outline (faint + grips), the reconstruction (bold), and the
// frozen epicycle chain.
type Chart struct {
	Target     []Pt
	Grips      []Handle
	Recon      []Pt
	Chain      []Circle
	TipX, TipY float64
}

func (c Chart) Render() Rendered {
	const box = 480.0
	sx := func(v float64) float64 { return v / plane * box }
	sy := func(v float64) float64 { return box - v/plane*box } // y up

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, box, box)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, box, box)

	// Target outline — faint, closed.
	if len(c.Target) > 1 {
		var d strings.Builder
		for i, p := range c.Target {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(p.X), sy(p.Y))
		}
		d.WriteString(" Z")
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#cbd5e1" stroke-width="1.5" stroke-dasharray="4 3"/>`, d.String())
	}

	// Frozen epicycle chain — the circles and the radius spokes.
	for _, circ := range c.Chain {
		if circ.R > 0.4 { // skip invisibly-tiny circles
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#e5b3f0" stroke-width="1"/>`,
				sx(circ.CX), sy(circ.CY), circ.R/plane*box)
		}
	}
	// spokes: connect successive circle centres, then to the tip.
	var spoke strings.Builder
	for i, circ := range c.Chain {
		verb := " L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&spoke, "%s%.1f %.1f", verb, sx(circ.CX), sy(circ.CY))
	}
	fmt.Fprintf(&spoke, " L%.1f %.1f", sx(c.TipX), sy(c.TipY))
	fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#c026d3" stroke-width="1" stroke-opacity="0.7"/>`, spoke.String())

	// Reconstruction — bold.
	if len(c.Recon) > 1 {
		var d strings.Builder
		for i, p := range c.Recon {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(p.X), sy(p.Y))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#4338ca" stroke-width="2.5"/>`, d.String())
	}

	// Draggable vertices.
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#fff" stroke="#4338ca" stroke-width="2" `+
			`data-grip=%q style="cursor:grab"/>`, sx(g.At.X), sy(g.At.Y), string(ref))
	}
	fmt.Fprintf(&b, `<text x="12" y="22" font-family="sans-serif" font-size="12" fill="#334155">drag the vertices · scrub the circles</text>`)
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
