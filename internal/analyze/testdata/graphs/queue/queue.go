//go:notebook
//
// A queue you can watch.
//
// This tests the timer — marimo's mo.ui.refresh — and it is the first thing in this
// whole design that required a genuinely new concept. Everything until now fell out of
// "cells are functions." This does not.
//
// A timer is easy: it is a leaf the RUNTIME writes on a schedule instead of the user
// writing it by dragging. Mechanically it changes nothing; a robot moving a slider.
//
// What is not easy is what a timer is FOR. Nobody wants a clock; they want to accumulate
// something over ticks — a rolling window, a running total, a simulation. And a pure DAG
// of pure functions cannot accumulate. A history is state that outlives an epoch.
//
// So the design needs exactly one new thing, and there is no way around it:
//
//     Prev[T] — a parameter holding this cell's OWN previous output.
//
// That is a self-edge, which would be a cycle, except it is a DELAYED one: it reads the
// last epoch, not this one. This is the oldest trick in synchronous dataflow (Lustre's
// `pre`, Elm's foldp, a clocked register in VHDL), and it comes with that tradition's
// rule, which the toolchain enforces:
//
//     A cell taking Prev[T] must also take a Tick. A register needs a clock.
//     It steps when the Tick advances — and NOT when any other input changes.
//
// That last clause matters and I nearly got it wrong. `sim` depends on the arrival-rate
// slider. In a pure DAG, moving that slider re-runs the cell — which would advance the
// simulation by one step every time you nudged the slider. Wrong. The Tick is the clock;
// everything else is a parameter absorbed into the next step. The signature says which
// is which, so the runtime doesn't have to guess.
//
// Two things fall out of this that I did not go looking for:
//
//   RANDOMNESS BECOMES REPRODUCIBLE. The PRNG state lives INSIDE the folded value, so
//   `seed` is an ordinary input leaf and the whole run is a deterministic function of
//   (seed, tick count, slider history). Compare np.random's global state — the classic
//   hidden-state reproducibility disaster that notebooks are famous for. Here the state
//   has nowhere to hide: it's a field.
//
//   THE NOTEBOOK BECOMES A SIMULATOR. Fold + clock is a discrete-event loop. And since
//   the head persists and the binary runs headless, `go tool notebook run queue.go --headless
//   --ticks 100000 --set lambda=1400 --json` is a batch simulation of the same file
//   you were just dragging sliders on.
//
// The honest cost is at the bottom.

package queue

import (
	"fmt"
	"math"
	"strings"
)

// simulated hours advanced per tick (one second of queue time)
const dt = 1.0 / 3600.0

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Simulation clock. The runtime writes this leaf; nobody else does.
//
//notebook:timer interval=100ms
func clock() (tick Tick) { return 0 }

// Random seed. An ordinary input — which is the whole point.
func seed() (seed int64) { return 20260710 }

// Incoming jobs per hour.
//
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }

// Jobs completed per hour, per server.
//
//notebook:slider min=1 max=200
func serviceRate() (mu PerHour) { return 20 }

// Servers in the fleet. Drag it while the simulation runs.
//
//notebook:slider min=1 max=200
func servers() (c int) { return 70 }

// ---------------------------------------------------------------------------
// The fold
// ---------------------------------------------------------------------------

// Queue state. The one stateful cell in the notebook, and it says so in its signature.
//
// Everything this cell needs to be reproducible is an argument: the previous state
// (which carries the PRNG), the clock, the seed, and the current parameters. Nothing
// is ambient. Re-run with the same seed and the same tick count and you get the same
// queue, every time, on any machine.
func sim(prev Prev[Sim], tick Tick, seed int64, lambda, mu PerHour, c int) (state Sim) {
	s := prev.Value
	if tick == 0 || s.Rand == 0 {
		s = Sim{Rand: Rand(seed | 1)} // first tick, or a seed change: start over
	}

	// Completions: each busy server finishes with probability 1 - exp(-mu·dt).
	pDone := 1 - math.Exp(-float64(mu)*dt)
	done := 0
	for range s.Busy {
		if s.Rand.float() < pDone {
			done++
		}
	}
	s.Busy -= done
	s.Served += done

	// Arrivals: Poisson(lambda·dt).
	s.Queue += s.Rand.poisson(float64(lambda) * dt)

	// Fill idle servers from the queue.
	free := max(c-s.Busy, 0)
	start := min(free, s.Queue)
	s.Busy += start
	s.Queue -= start

	// Jobs still waiting have waited one more dt. This is the sum that Little's law
	// relates to queue length, accumulated the honest way rather than assumed.
	s.WaitHours += float64(s.Queue) * dt

	s.Tick = tick
	s.History = appendRing(s.History, Sample{
		Tick: tick, Queue: s.Queue, Busy: s.Busy, Servers: c,
	}, 240)
	return s
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// Queue depth over the last four minutes of simulated time.
//
//notebook:height=300
func depth(state Sim) (plot Chart) {
	plot = Chart{Title: "jobs waiting (violet) · servers busy (indigo)"}
	for _, h := range state.History {
		plot.Queue = append(plot.Queue, float64(h.Queue))
		plot.Busy = append(plot.Busy, float64(h.Busy)/float64(max(h.Servers, 1)))
	}
	return plot
}

// Right now.
func readout(state Sim, c int) (now Readout) {
	util := float64(state.Busy) / float64(max(c, 1))
	var meanWait float64
	if state.Served > 0 {
		meanWait = state.WaitHours / float64(state.Served) * 3600
	}
	return Readout{Cards: []Card{
		{"waiting", fmt.Sprintf("%d", state.Queue), "jobs in queue"},
		{"utilization", fmt.Sprintf("%.0f%%", util*100), "servers busy"},
		{"served", fmt.Sprintf("%d", state.Served), "jobs completed"},
		{"mean wait", fmt.Sprintf("%.1fs", meanWait), "per completed job"},
	}}
}

// A queue you can watch.
func intro() (md Markdown) {
	return `An M/M/c queue, stepped once per tick. Drag the fleet size down while it runs
and watch the backlog build; drag it back and watch the queue drain.

Moving a slider does **not** advance the clock — the ` + "`Tick`" + ` does. That distinction
is in the signature of the one cell that has any state at all.`
}

// ===========================================================================
// Helpers
// ===========================================================================

// Rand is xorshift64*. Its state is a uint64 living inside the folded value, which is
// what makes the whole simulation a pure function of its seed.
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

// poisson samples by Knuth's method — fine for the small means a per-tick arrival
// count produces.
func (r *Rand) poisson(mean float64) int {
	l, k, p := math.Exp(-mean), 0, 1.0
	for {
		p *= r.float()
		if p <= l {
			return k
		}
		k++
		if k > 1000 {
			return k
		}
	}
}

func appendRing[T any](xs []T, x T, n int) []T {
	xs = append(xs, x)
	if len(xs) > n {
		xs = xs[len(xs)-n:]
	}
	return xs
}

// ===========================================================================
// Types
// ===========================================================================

type (
	Tick    uint64
	PerHour float64
)

// Prev holds a cell's own previous output. A cell that takes one is stateful, and the
// toolchain requires it to take a Tick as well: a register needs a clock.
type Prev[T any] struct{ Value T }

type Sample struct {
	Tick           Tick
	Queue, Busy    int
	Servers        int
}

type Sim struct {
	Tick      Tick
	Rand      Rand
	Busy      int
	Queue     int
	Served    int
	WaitHours float64
	History   []Sample
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Chart struct {
	Title       string
	Queue, Busy []float64
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 300.0, 36.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	line := func(ys []float64, hi float64, color string) {
		if len(ys) < 2 {
			return
		}
		var d strings.Builder
		for i, v := range ys {
			x := pad + float64(i)/float64(len(ys)-1)*(w-2*pad)
			y := h - pad - math.Min(v/hi, 1)*(h-2*pad)
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, x, y)
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2"/>`, d.String(), color)
	}

	qmax := 1.0
	for _, v := range c.Queue {
		qmax = math.Max(qmax, v)
	}
	line(c.Busy, 1, "#4338ca") // utilization, already a fraction
	line(c.Queue, qmax, "#c026d3")
	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12">%s</text>`,
		pad, c.Title)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
