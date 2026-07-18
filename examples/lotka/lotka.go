//go:notebook
//
// Predator and prey — the same trajectory, two ways.
//
// The Lotka-Volterra equations are the textbook model of two coupled populations:
// prey grow, predators eat prey and starve without them. It is the canonical
// picture of an oscillating ecosystem — hares and lynx, the cycle every biology
// course draws.
//
//     dx/dt =  α·x − β·x·y      (prey: born, eaten)
//     dy/dt = −γ·y + δ·x·y      (predators: die, fed)
//
// This notebook shows one trajectory in the two ways it is always drawn, both
// wired to the same source:
//
//   - the **time series** — prey and predators over time, each a rising-and-
//     crashing wave, the predators lagging the prey;
//   - the **phase portrait** — the same run plotted as prey-vs-predators, which
//     closes into a loop, because the system is conservative and returns to where
//     it started.
//
// Two things this puts on stage that the corpus hadn't:
//
//   - **Linked views from one graph.** `series` and `portrait` are two cells that
//     both read the one `trajectory`. Drag any parameter and both redraw from the
//     same recomputed run — not two plots kept in sync by hand, but two projections
//     of a single value the graph already holds. That is the reactive-graph thesis
//     as a picture: the dependency graph forks trajectory → {series, portrait}.
//     They share a `//notebook:row=panels` directive, so on a wide screen they sit
//     side by side (and wrap to stacked when it's narrow).
//   - **A grip in phase space.** The starting populations are a draggable point on
//     the phase portrait itself — drag it and you pick a different orbit. curvefit's
//     grip mechanism, but the handle lives in the abstract (x, y) plane, not on a
//     spatial chart.
//
// Fixed-horizon and pure, like nbody: the trajectory is integrated to a fixed number
// of steps inside one cell, a pure function of (params, start, steps), so scrubbing
// any slider is exact and reversible. No fold.

package lotka

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Prey birth rate α, in hundredths (110 → 1.10). How fast prey breed unchecked.
//
//notebook:slider min=40 max=200 step=5
func alphaCenti() (alpha int) { return 110 }

// Predation rate β, in hundredths — how efficiently predators convert prey into
// fewer prey.
//
//notebook:slider min=20 max=120 step=5
func betaCenti() (beta int) { return 40 }

// Predator death rate γ, in hundredths — how fast predators starve without prey.
//
//notebook:slider min=40 max=200 step=5
func gammaCenti() (gamma int) { return 100 }

// Predator growth rate δ, in hundredths — how much each meal helps predators breed.
//
//notebook:slider min=10 max=100 step=5
func deltaCenti() (delta int) { return 30 }

// How long to simulate, in tenths of a time unit — the horizon and scrub axis.
//
//notebook:slider min=50 max=400 step=10
func horizonTenths() (span int) { return 200 }

// Starting populations: a draggable point on the phase portrait. Drag it to pick a
// different orbit. x = prey, y = predators (each in tens of individuals).
//
//notebook:height=460
func start() (ic Draggable[Pt]) {
	return Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}
}

// ---------------------------------------------------------------------------
// Compute (Go) — one trajectory, pure.
// ---------------------------------------------------------------------------

// The trajectory: integrate Lotka-Volterra from the starting populations to the
// horizon with RK4, returning prey and predators at every step. Pure — a function
// of (α, β, γ, δ, start, span) alone — so every slider scrubs exactly, both ways.
// One run; the two views below are both projections of it.
func trajectory(alpha, beta, gamma, delta int, ic Draggable[Pt], span int) (run Trajectory) {
	a, b := float64(alpha)/100, float64(beta)/100
	g, d := float64(gamma)/100, float64(delta)/100
	p0 := ic.Value[0]
	steps := span * 4 // span is in tenths; integrate at dt = 0.025
	dt := 0.025

	deriv := func(x, y float64) (float64, float64) {
		return a*x - b*x*y, -g*y + d*x*y
	}

	xs := make([]float64, 0, steps+1)
	ys := make([]float64, 0, steps+1)
	x, y := p0.X, p0.Y
	for i := 0; i <= steps; i++ {
		xs = append(xs, x)
		ys = append(ys, y)
		// classic RK4 step
		k1x, k1y := deriv(x, y)
		k2x, k2y := deriv(x+0.5*dt*k1x, y+0.5*dt*k1y)
		k3x, k3y := deriv(x+0.5*dt*k2x, y+0.5*dt*k2y)
		k4x, k4y := deriv(x+dt*k3x, y+dt*k3y)
		x += dt / 6 * (k1x + 2*k2x + 2*k3x + k4x)
		y += dt / 6 * (k1y + 2*k2y + 2*k3y + k4y)
		// populations can't go negative; clamp to keep a dragged-to-extreme IC sane
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
	}
	return Trajectory{Prey: xs, Pred: ys, DT: dt}
}

// ---------------------------------------------------------------------------
// Views — two projections of the one trajectory.
// ---------------------------------------------------------------------------

// Populations over time. Prey and predators as two waves; watch the predators lag
// the prey — the crash follows the feast.
//
//notebook:row=panels
//notebook:height=460
func series(run Trajectory) (timeplot TimeChart) {
	return TimeChart{Prey: run.Prey, Pred: run.Pred, DT: run.DT}
}

// The phase portrait: prey on x, predators on y, the same run as a path. It closes
// into a loop because the system is conservative. The starting point is a grip —
// drag it to a different orbit.
//
//notebook:row=panels
//notebook:height=460
func portrait(run Trajectory, ic Draggable[Pt]) (phase PhaseChart) {
	return PhaseChart{Prey: run.Prey, Pred: run.Pred, Grip: Handle{At: ic.Value[0], Ref: ic.Grip(0)}}
}

// The numbers: the period of the cycle (peak-to-peak in prey) and the peak
// populations — a readout of the oscillation the plots show.
func summary(run Trajectory) (report Readout) {
	period := run.periodEstimate()
	preyMax, predMax := run.peaks()
	pstr := "—"
	if period > 0 {
		pstr = f1(period)
	}
	return Readout{Cards: []Card{
		{Label: "cycle period", Value: pstr, Caption: "time units, prey peak to peak"},
		{Label: "peak prey", Value: f1(preyMax)},
		{Label: "peak predators", Value: f1(predMax)},
	}}
}

// Predator and prey — the same trajectory, two ways.
func intro() (md Markdown) {
	return `The **Lotka-Volterra** model: prey breed, predators eat prey and starve
without them. Drag the rates, or drag the **starting point on the phase portrait**
to pick a different orbit.

Two views, one source. The **time series** shows prey and predators rising and
crashing, the predators lagging the prey. Beside it, the **phase portrait** plots
the same run as prey-vs-predators, and it closes into a *loop* — the system is
conservative and returns to where it started. They are two cells reading the one
` + "`trajectory`" + ` cell, so a single edit redraws both: the dependency graph
forks trajectory → {series, portrait}, and that fork is the reactive thesis as a
picture.

It's fixed-horizon and pure, so scrub any slider forward and back — every frame is
exact.`
}

// ===========================================================================
// Trajectory analysis
// ===========================================================================

// periodEstimate finds the mean spacing between successive prey peaks (local
// maxima), in time units — 0 if fewer than two peaks are seen.
func (t Trajectory) periodEstimate() float64 {
	var peaks []int
	for i := 1; i < len(t.Prey)-1; i++ {
		if t.Prey[i] > t.Prey[i-1] && t.Prey[i] >= t.Prey[i+1] {
			peaks = append(peaks, i)
		}
	}
	if len(peaks) < 2 {
		return 0
	}
	var sum int
	for i := 1; i < len(peaks); i++ {
		sum += peaks[i] - peaks[i-1]
	}
	return float64(sum) / float64(len(peaks)-1) * t.DT
}

func (t Trajectory) peaks() (prey, pred float64) {
	for _, v := range t.Prey {
		if v > prey {
			prey = v
		}
	}
	for _, v := range t.Pred {
		if v > pred {
			pred = v
		}
	}
	return prey, pred
}

// ===========================================================================
// Helpers
// ===========================================================================

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// axisMax returns a rounded-up upper bound for a set of series, so both charts have
// a stable, legible frame.
func axisMax(series ...[]float64) float64 {
	m := 0.0
	for _, s := range series {
		for _, v := range s {
			if v > m {
				m = v
			}
		}
	}
	if m <= 0 {
		return 1
	}
	return m * 1.1
}

// ===========================================================================
// Types
// ===========================================================================

type Pt struct{ X, Y float64 }

// Trajectory is the one integrated run: prey and predator populations at each step,
// plus the timestep so views can label a time axis.
type Trajectory struct {
	Prey []float64
	Pred []float64
	DT   float64
}

// Draggable — curvefit's grip leaf, unchanged. Here the single point is a starting
// (prey, predator) pair, gripped on the phase portrait.
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

// TimeChart draws prey and predators against time.
type TimeChart struct {
	Prey []float64
	Pred []float64
	DT   float64
}

func (c TimeChart) Render() Rendered {
	const w, h, pad = 440.0, 460.0, 40.0
	ymax := axisMax(c.Prey, c.Pred)
	n := len(c.Prey)
	sx := func(i int) float64 {
		if n < 2 {
			return pad
		}
		return pad + float64(i)/float64(n-1)*(w-2*pad)
	}
	sy := func(v float64) float64 { return h - pad - v/ymax*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)
	line := func(s []float64, color string) {
		if len(s) < 2 {
			return
		}
		var d strings.Builder
		for i, v := range s {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(i), sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2"/>`, d.String(), color)
	}
	line(c.Prey, "#2a78d6") // prey, blue
	line(c.Pred, "#0797b8") // predators, aqua
	fmt.Fprintf(&b, `<text x="%.0f" y="24" font-family="sans-serif" font-size="12" fill="#1b3a6b">populations over time</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#2a78d6">prey</text>`, pad+6, h-pad-24)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#0797b8">predators</text>`, pad+6, h-pad-8)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// PhaseChart draws the trajectory in the prey-predator plane, with the draggable
// starting point.
type PhaseChart struct {
	Prey []float64
	Pred []float64
	Grip Handle
}

func (c PhaseChart) Render() Rendered {
	const w, h, pad = 440.0, 460.0, 44.0
	xmax := axisMax(c.Prey)
	ymax := axisMax(c.Pred)
	sx := func(v float64) float64 { return pad + v/xmax*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/ymax*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, w-2*pad, h-2*pad)
	if len(c.Prey) >= 2 {
		var d strings.Builder
		for i := range c.Prey {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(c.Prey[i]), sy(c.Pred[i]))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#2a78d6" stroke-width="1.6" stroke-opacity="0.85"/>`, d.String())
	}
	// The draggable starting point.
	ref, _ := c.Grip.Ref.MarshalText()
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="#fff" stroke="#2a78d6" stroke-width="2.5" `+
		`data-grip=%q style="cursor:grab"/>`, sx(c.Grip.At.X), sy(c.Grip.At.Y), string(ref))
	fmt.Fprintf(&b, `<text x="%.0f" y="24" font-family="sans-serif" font-size="12" fill="#1b3a6b">phase portrait — prey vs predators</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="11" fill="#5b6472">prey →</text>`, w-pad-44, h-pad+16)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// WidgetView is a widget's state on the wire.
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
