//go:notebook
//
// M/M/c fleet capacity model.
//
// A notebook is a plain Go package. The reactive machinery is derived, not imported:
//
//   - A *cell* is a top-level func with a doc comment (the comment is also its label).
//     Undocumented funcs (erlangC, svg) are ordinary helpers, invisible to the graph.
//   - An *edge* is a value: a cell's named result feeds any cell that takes a
//     parameter of the same name and type. That is the entire wiring rule.
//   - A cell's *widget* is read off its type. Types that carry Bounds() render as a
//     ranged control automatically; everything else uses //notebook: for presentation only.
//   - Outputs render by structural probe: a value with Render() Rendered is drawn as
//     rich content; scalars fall back to type-default readouts (gauge, money, duration).
//
// Note the imports: stdlib only. Nothing here knows it's in a notebook.

package capacity

import (
	"fmt"
	"math"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs (graph roots). These are the interactive controls.
// ---------------------------------------------------------------------------

// Incoming jobs per hour.
//
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }

// Jobs completed per hour, per server.
//
//notebook:slider min=1 max=200 step=1
func serviceRate() (mu PerHour) { return 20 }

// Servers in the fleet.
//
//notebook:slider min=1 max=256
func servers() (c int) { return 80 }

// On-demand price per server-hour.
//
//notebook:slider min=0 max=5 step=0.001
func hourlyPrice() (price USD) { return 1.006 }

// SLA: the largest acceptable probability that a job has to wait.
// No //notebook: needed — Probability carries Bounds(), so it renders as a [0,1] slider.
func slaTarget() (target Probability) { return 0.20 }

// ---------------------------------------------------------------------------
// Derived quantities. The DAG is implicit in the parameter names.
// ---------------------------------------------------------------------------

// Offered load in Erlangs (lambda/mu).
func offeredLoad(lambda, mu PerHour) (a Erlangs) {
	return Erlangs(float64(lambda) / float64(mu))
}

// Server utilization. Deliberately a plain float64, not a Unit: overload (rho >= 1)
// is a real, meaningful state, so clamping to [0,1] would be a lie.
func utilization(a Erlangs, c int) (rho float64) {
	return float64(a) / float64(c)
}

// Probability an arriving job finds every server busy (Erlang C).
func waitProbability(a Erlangs, c int) (pWait Probability) {
	return Probability(erlangC(c, float64(a)))
}

// Mean time a job spends waiting in queue before service.
func meanWait(pWait Probability, rho float64, c int, mu PerHour) (wq Seconds) {
	if rho >= 1 {
		return Seconds(math.Inf(1)) // unstable: the queue grows without bound
	}
	hours := float64(pWait) / (float64(c) * float64(mu) * (1 - rho))
	return Seconds(hours * 3600)
}

// Expected number of jobs in the system, via Little's law (L = lambda * W).
func jobsInSystem(lambda PerHour, wq Seconds, mu PerHour) (inSystem float64) {
	w := float64(wq)/3600 + 1/float64(mu) // total time in system, in hours
	return float64(lambda) * w
}

// Fleet cost per hour.
func hourlyCost(c int, price USD) (cost USD) {
	return USD(float64(c) * float64(price))
}

// Amortized infrastructure cost per completed job.
func costPerJob(cost USD, lambda PerHour) (perJob USD) {
	return USD(float64(cost) / float64(lambda))
}

// Whether the current fleet meets the wait-probability SLA.
func slaMet(pWait, target Probability) (meets bool) {
	return pWait <= target
}

// ---------------------------------------------------------------------------
// Views (graph leaves). Values with Render() draw as rich output.
// ---------------------------------------------------------------------------

// Cost vs. latency as the fleet scales past its stability floor.
//
//notebook:height=320
func capacityCurve(lambda, mu PerHour, price USD) (curve Chart) {
	a := float64(lambda) / float64(mu)
	minC := int(math.Ceil(a)) + 1 // fleet must exceed offered load to be stable

	var xs, waits, costs []float64
	for c := minC; c <= minC+40; c++ {
		rho := a / float64(c)
		wqSec := (erlangC(c, a) / (float64(c) * float64(mu) * (1 - rho))) * 3600
		xs = append(xs, float64(c))
		waits = append(waits, wqSec)
		costs = append(costs, float64(c)*float64(price))
	}
	return Chart{
		Title: "Cost (indigo) vs. queue wait (violet) as servers scale",
		X:     xs,
		Y1:    waits, // seconds
		Y2:    costs, // $/hr
	}
}

// Orientation shown above the controls.
func notes() (intro Markdown) {
	return `## M/M/c capacity model

Set the arrival rate, the per-server service rate, and the fleet size. The model
reports utilization, the Erlang-C probability of waiting, mean queue wait, and the
cost of the fleet — then sweeps fleet size to show where adding servers stops
buying you meaningful latency.`
}

// ===========================================================================
// Helpers (no doc comment => not cells). Ordinary Go called inside cell bodies.
// ===========================================================================

// erlangC returns the probability of waiting in an M/M/c queue with c servers and
// offered load a, computed from Erlang B via the numerically stable recursion.
func erlangC(c int, a float64) float64 {
	if c <= 0 {
		return 1
	}
	rho := a / float64(c)
	if rho >= 1 {
		return 1 // saturated: an arrival always waits
	}
	b := 1.0
	for k := 1; k <= c; k++ {
		b = (a * b) / (float64(k) + a*b)
	}
	return b / (1 - rho + rho*b)
}

// svg renders a Chart as a minimal two-series line plot.
func svg(ch Chart) string {
	const w, h, pad = 640.0, 300.0, 36.0
	n := len(ch.X)
	if n == 0 {
		return ""
	}
	minX, maxX := ch.X[0], ch.X[n-1]
	line := func(ys []float64, color string) string {
		lo, hi := ys[0], ys[0]
		for _, v := range ys {
			if math.IsInf(v, 0) {
				continue
			}
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		if hi == lo {
			hi = lo + 1
		}
		var d strings.Builder
		for i, v := range ys {
			if math.IsInf(v, 0) {
				continue
			}
			x := pad + (ch.X[i]-minX)/(maxX-minX)*(w-2*pad)
			y := h - pad - (v-lo)/(hi-lo)*(h-2*pad)
			if d.Len() == 0 {
				fmt.Fprintf(&d, "M%.1f %.1f", x, y)
			} else {
				fmt.Fprintf(&d, " L%.1f %.1f", x, y)
			}
		}
		return fmt.Sprintf(`<path d=%q fill="none" stroke=%q stroke-width="2"/>`, d.String(), color)
	}
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`+
			`<rect width="%.0f" height="%.0f" fill="white"/>%s%s`+
			`<text x="%.0f" y="20" font-family="sans-serif" font-size="13">%s</text></svg>`,
		w, h, w, h, line(ch.Y2, "#4338ca"), line(ch.Y1, "#c026d3"), pad, ch.Title)
}

// ===========================================================================
// Semantic types. In a real notebook these are one import (`notebook`) for autocomplete
// and a curated vocabulary; shown inline here to prove nothing is load-bearing.
// ===========================================================================

type (
	PerHour     float64
	USD         float64
	Erlangs     float64
	Seconds     float64
	Probability float64
	Markdown    string
)

// Bounds makes Probability render as a [0,1] control (input) or gauge (output),
// with no import and no //notebook: directive. Satisfied structurally.
func (Probability) Bounds() (lo, hi float64) { return 0, 1 }

// Rendered is a MIME-tagged blob the runtime knows how to display.
type Rendered struct {
	MIME string
	Data string
}

type Chart struct {
	Title  string
	X      []float64
	Y1, Y2 []float64
}

func (ch Chart) Render() Rendered   { return Rendered{MIME: "image/svg+xml", Data: svg(ch)} }
func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
