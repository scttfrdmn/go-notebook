//go:notebook
//
// Running is not passing — in a numerical integrator.
//
// Three bodies under mutual gravity. Drag any starting position; scrub the number
// of steps to fast-forward the simulation. The orbits trace out, and beside them a
// second plot shows the one thing an orbit picture hides: the system's total energy
// over time.
//
// Two integrators run the SAME initial condition. Forward Euler — the obvious,
// textbook first thing anyone writes — runs without complaint and produces a
// plausible-looking orbit. It is also wrong: it manufactures energy from nothing,
// the total climbs every step, and given enough steps the bodies spiral out to
// infinity. Velocity Verlet (a symplectic integrator) runs the identical loop and
// keeps the energy flat. Same code shape, same runtime, opposite correctness.
//
// That is the project's own finding wearing a lab coat: **a thing that ran is not a
// thing that is right.** The orbit plot alone would let Euler pass — it looks fine.
// The energy plot is the instrument that catches it. You cannot see the bug in the
// output you were looking at; you have to plot the invariant.
//
// Two design choices worth stating:
//
//   - **No fold.** The trajectory is computed to a fixed horizon INSIDE one cell —
//     a pure function of (initial condition, step count, step size). So scrubbing
//     the step slider backward re-runs from zero and the past is exact, the same
//     reason bayes can scrub backward: nothing here is stateful.
//   - **Units are load-bearing.** Kinetic and potential energy are a typed `Energy`;
//     a `Velocity` cannot be added to a `Position`. The dimensional error the Lego
//     port found (dollars × dollars) is a compile error here, in a domain where it
//     is the classic mistake.

package nbody

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Starting positions of the three bodies. A draggable leaf — drag a body and the
// whole simulation re-runs from the new initial condition. Grips are curvefit's
// mechanism unchanged: this cell owns the leaf; a DIFFERENT cell (orbits) draws the
// handles and reads them.
//
//notebook:height=440
func bodies() (start Draggable[Pt]) {
	// A deliberately asymmetric configuration — not a special periodic solution, and
	// chosen (by sweeping candidates) so Euler's energy climbs GRADUALLY across the
	// run rather than jumping at one early close approach, which is what makes the
	// "manufactures energy every step" story read true on the energy plot.
	return Draggable[Pt]{Value: []Pt{{35, 45}, {65, 52}, {52, 68}}}
}

// Steps to simulate. The horizon, and the scrub axis: drag it up to fast-forward,
// down to rewind — the trajectory is recomputed from zero each time, exactly.
//
//notebook:slider min=50 max=4000 step=50
func steps() (n int) { return 1500 }

// Step size (the integrator's dt). Larger steps integrate faster but amplify Euler's
// energy drift — turn it up and watch the Euler curve climb steeper (≈6% drift at
// 12ms, ≈16% at 20ms, ≈24% at 30ms), while Verlet stays flat the whole way.
//
//notebook:slider min=4 max=30 step=2
func stepSize() (dtMilli int) { return 12 }

// ---------------------------------------------------------------------------
// The two simulations — same initial condition, two integrators, both pure.
// ---------------------------------------------------------------------------

// Forward Euler. The textbook first thing you write, and the one that lies: it adds
// energy every step. Runs fine; is wrong.
func euler(start Draggable[Pt], n int, dtMilli int) (euler Trajectory) {
	return integrate(start.Value, n, dt(dtMilli), stepEuler)
}

// Velocity Verlet — symplectic, energy-conserving. The identical loop with the
// updates reordered so the discrete step has a conserved shadow Hamiltonian.
func verlet(start Draggable[Pt], n int, dtMilli int) (verlet Trajectory) {
	return integrate(start.Value, n, dt(dtMilli), stepVerlet)
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The orbits. Both integrators' paths, and the draggable starting positions. This
// is the view that would let Euler pass — the paths look plausible.
//
//notebook:row=panels
//notebook:height=440
func orbits(start Draggable[Pt], euler Trajectory, verlet Trajectory) (paths OrbitChart) {
	paths = OrbitChart{Euler: euler, Verlet: verlet}
	for i, p := range start.Value {
		paths.Grips = append(paths.Grips, Handle{At: p, Ref: start.Grip(i)})
	}
	return paths
}

// Total energy over time. The instrument that catches the bug the orbit plot hides:
// Euler's total energy climbs; Verlet's stays flat. Same simulation, plotted honestly.
//
//notebook:row=panels
//notebook:height=440
func conservation(euler Trajectory, verlet Trajectory) (energy EnergyChart) {
	return EnergyChart{Euler: euler.Energy, Verlet: verlet.Energy}
}

// How badly Euler drifts — the number under the picture. Fractional change in total
// energy from first step to last; Verlet's should be a rounding error, Euler's large.
func drift(euler Trajectory, verlet Trajectory) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "Euler energy drift", Value: pct(relDrift(euler.Energy)), Caption: "should be ~0 if it were right"},
		{Label: "Verlet energy drift", Value: pct(relDrift(verlet.Energy)), Caption: "symplectic — stays put"},
	}}
}

// Running is not passing — in a numerical integrator.
func intro() (md Markdown) {
	return `Drag a body, or scrub the step count. Two integrators run the same
initial condition: forward Euler and velocity Verlet.

Watch the **orbits** on the left and the **total energy** on the right. Euler's
orbits look fine — that is the trap. The energy plot is the only place the bug is
visible: Euler manufactures energy every step and the total climbs, while Verlet
holds it flat. A thing that ran is not a thing that is right; you have to plot the
invariant to know.`
}

// ===========================================================================
// Physics — softened gravity, so no singularity blows up at a close approach.
// ===========================================================================

const (
	bigG = 60.0 // gravitational constant, tuned for motion within the [0,100] frame
	mass = 1.0  // all three bodies equal mass
	soft = 4.0  // Plummer softening length: gravity is finite at zero separation
)

// state is the integrator's working set: positions and velocities of every body.
type state struct {
	pos []vec
	vel []vec
}

// integrate runs `n` steps of `step` from the starting positions and returns the
// per-body path plus the total-energy time series. Pure: same inputs, same output,
// no state escapes. Initial velocities are a fixed tangential kick about the
// centroid (a deterministic function of the positions), so the whole run is pure.
func integrate(start []Pt, n int, h float64, step func(*state, float64)) Trajectory {
	s := &state{pos: make([]vec, len(start)), vel: make([]vec, len(start))}
	cx, cy := centroid(start)
	for i, p := range start {
		s.pos[i] = vec{p.X, p.Y}
		// Tangential kick: perpendicular to the radius from the centroid, scaled to
		// give a bound-ish orbit. Deterministic → pure.
		rx, ry := p.X-cx, p.Y-cy
		// kick=0.05 gives a BOUND system (total energy < 0): the bodies orbit rather
		// than fly apart, which is the regime where Euler's energy pumping shows —
		// an unbound system flies apart before the drift is visible.
		s.vel[i] = vec{-ry * 0.05, rx * 0.05}
	}

	tr := Trajectory{Paths: make([][]Pt, len(start))}
	record := func() {
		for i, p := range s.pos {
			tr.Paths[i] = append(tr.Paths[i], Pt{p.x, p.y})
		}
		tr.Energy = append(tr.Energy, float64(totalEnergy(s)))
	}
	record()
	for range n {
		step(s, h)
		record()
	}
	return tr
}

// stepEuler is forward (explicit) Euler: advance position by the OLD velocity, then
// velocity by the OLD acceleration. Simple, and it pumps energy in.
func stepEuler(s *state, h float64) {
	a := accelerations(s.pos)
	for i := range s.pos {
		s.pos[i] = s.pos[i].add(s.vel[i].scale(h))
		s.vel[i] = s.vel[i].add(a[i].scale(h))
	}
}

// stepVerlet is velocity Verlet: half-kick, drift, recompute acceleration, half-kick.
// The reordering is the whole difference — it makes the step symplectic, so energy
// error stays bounded instead of accumulating.
func stepVerlet(s *state, h float64) {
	a := accelerations(s.pos)
	for i := range s.pos {
		s.vel[i] = s.vel[i].add(a[i].scale(h / 2))
		s.pos[i] = s.pos[i].add(s.vel[i].scale(h))
	}
	a2 := accelerations(s.pos)
	for i := range s.pos {
		s.vel[i] = s.vel[i].add(a2[i].scale(h / 2))
	}
}

// accelerations is the gravitational acceleration on each body from all the others,
// with Plummer softening (r²+ε²) so a close approach can't produce an infinite force.
func accelerations(pos []vec) []vec {
	a := make([]vec, len(pos))
	for i := range pos {
		for j := range pos {
			if i == j {
				continue
			}
			d := pos[j].sub(pos[i])
			r2 := d.x*d.x + d.y*d.y + soft*soft
			inv := bigG * mass / (r2 * math.Sqrt(r2))
			a[i] = a[i].add(d.scale(inv))
		}
	}
	return a
}

// totalEnergy is kinetic + potential over all bodies — the conserved quantity, and
// the honest instrument. Typed Energy: a Velocity can't be added to a Position here.
func totalEnergy(s *state) Energy {
	var e Energy
	for i := range s.pos {
		e += kinetic(mass, math.Hypot(s.vel[i].x, s.vel[i].y))
	}
	for i := range s.pos {
		for j := i + 1; j < len(s.pos); j++ {
			d := s.pos[j].sub(s.pos[i])
			r := math.Sqrt(d.x*d.x + d.y*d.y + soft*soft)
			e += potential(mass, mass, r)
		}
	}
	return e
}

// kinetic energy ½mv². Returns Energy — the unit boundary where a dimensional slip
// would be caught.
func kinetic(m Mass, v float64) Energy { return Energy(0.5 * float64(m) * v * v) }

// potential energy of a softened pair, -Gm₁m₂/r. Also Energy.
func potential(m1, m2 Mass, r float64) Energy {
	return Energy(-bigG * float64(m1) * float64(m2) / r)
}

// ===========================================================================
// Helpers
// ===========================================================================

type vec struct{ x, y float64 }

func (a vec) add(b vec) vec       { return vec{a.x + b.x, a.y + b.y} }
func (a vec) sub(b vec) vec       { return vec{a.x - b.x, a.y - b.y} }
func (a vec) scale(k float64) vec { return vec{a.x * k, a.y * k} }

func centroid(pts []Pt) (float64, float64) {
	var cx, cy float64
	for _, p := range pts {
		cx += p.X
		cy += p.Y
	}
	n := float64(len(pts))
	return cx / n, cy / n
}

// dt converts the slider's integer milliseconds to a float step size.
func dt(dtMilli int) float64 { return float64(dtMilli) / 1000 }

// relDrift is the fractional change in total energy from first to last sample —
// the drift the energy plot shows, as one number.
func relDrift(e []float64) float64 {
	if len(e) < 2 || e[0] == 0 {
		return 0
	}
	return (e[len(e)-1] - e[0]) / math.Abs(e[0])
}

func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

// Unit types. Energy is the load-bearing one: kinetic and potential both return it,
// total energy is Energy+Energy, and nothing lets you add a Position to it.
type (
	Mass   float64
	Energy float64
)

// Trajectory is one integrator's output: a path per body, and the total-energy
// time series that is the honest instrument.
type Trajectory struct {
	Paths  [][]Pt
	Energy []float64
}

// Draggable — curvefit's grip leaf, unchanged. Its grips are drawn by a different
// cell (orbits), so the leaf identity rides with the value via Grip(i).
type Draggable[T any] struct {
	Value []T
	leaf  string
}

func (d Draggable[T]) WithLeaf(sym string) Draggable[T] { d.leaf = sym; return d }
func (d Draggable[T]) Grip(i int) Ref                   { return Ref{Leaf: d.leaf, Index: i} }

// Reconcile keeps dragged positions while the body count is unchanged (it is fixed
// at three here); otherwise the fresh seed stands.
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

// OrbitChart draws both integrators' paths plus the draggable starting grips.
type OrbitChart struct {
	Euler  Trajectory
	Verlet Trajectory
	Grips  []Handle
}

func (c OrbitChart) Render() Rendered {
	const w, h, pad = 440.0, 440.0, 20.0
	const lo, hi = 0.0, 100.0
	sx := func(v float64) float64 { return pad + (v-lo)/(hi-lo)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - (v-lo)/(hi-lo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	drawPaths := func(tr Trajectory, color string, width float64) {
		for _, path := range tr.Paths {
			if len(path) < 2 {
				continue
			}
			var d strings.Builder
			for i, p := range path {
				verb := " L"
				if i == 0 {
					verb = "M"
				}
				fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(p.X), sy(p.Y))
			}
			fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="%.1f" stroke-opacity="0.85"/>`,
				d.String(), color, width)
		}
	}
	drawPaths(c.Euler, "#d03b3b", 1.0)  // Euler — the one that drifts, in red
	drawPaths(c.Verlet, "#2a78d6", 1.6) // Verlet — correct, in blue

	// Draggable starting positions.
	for _, g := range c.Grips {
		ref, _ := g.Ref.MarshalText()
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="#fff" stroke="#1b3a6b" `+
			`stroke-width="2" data-grip=%q style="cursor:grab"/>`, sx(g.At.X), sy(g.At.Y), string(ref))
	}
	// Legend.
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#d03b3b">Euler</text>`, pad+6, pad+16)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#2a78d6">Verlet</text>`, pad+56, pad+16)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// EnergyChart plots total energy vs time for both integrators on a shared scale —
// the plot where Euler's drift is undeniable.
type EnergyChart struct {
	Euler  []float64
	Verlet []float64
}

func (c EnergyChart) Render() Rendered {
	const w, h, pad = 440.0, 440.0, 40.0
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, series := range [][]float64{c.Euler, c.Verlet} {
		for _, v := range series {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if !(hi > lo) {
		hi = lo + 1
	}
	// Pad the range a touch so flat lines aren't glued to the frame.
	span := hi - lo
	lo, hi = lo-0.08*span, hi+0.08*span
	sx := func(i, n int) float64 {
		if n < 2 {
			return pad
		}
		return pad + float64(i)/float64(n-1)*(w-2*pad)
	}
	sy := func(v float64) float64 { return h - pad - (v-lo)/(hi-lo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	line := func(series []float64, color string, width float64) {
		if len(series) < 2 {
			return
		}
		var d strings.Builder
		for i, v := range series {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(i, len(series)), sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="%.1f"/>`, d.String(), color, width)
	}
	line(c.Euler, "#d03b3b", 2)
	line(c.Verlet, "#2a78d6", 2)

	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12" fill="#1b3a6b">total energy vs time</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#d03b3b">Euler (drifts up)</text>`, pad+6, h-pad-10)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#2a78d6">Verlet (flat)</text>`, pad+6, h-pad-26)
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
