//go:notebook
//
// Your autoscaler is a PID controller — here's the step response.
//
// An autoscaler watches a signal (queue depth, CPU, latency) and adds or removes
// capacity to hold it near a target. That is a **PID controller**, whether or not
// anyone called it one: the correction each tick is
//
//     u = Kp·e  +  Ki·∫e  +  Kd·(de/dt)
//
// proportional to the current error, its accumulated history, and its rate of
// change. Most autoscalers are tuned by vibes — bump the threshold, add a cooldown,
// hope. This notebook shows the thing the vibes are groping at: the **step response**.
// Load jumps at t=0; watch how the queue depth recovers.
//
// The three failures every operator has caused, now visible on the plot (two lines:
// the blue queue depth it's controlling, the amber replica count it's choosing):
//
//   - **Too much Kp** → it overshoots and *oscillates* — over-provisions, then
//     over-culls, then over-provisions, ringing hard around the target.
//   - **No Ki** → it settles, but *off* the target and stays there forever. With a
//     load increase the proportional term can only hold the queue where Kp·error
//     exactly covers the extra arrivals — which is *above* target (a steady-state
//     offset called droop). Only the integral term, accumulating that residual error,
//     drives it to zero.
//   - **Too much Kd** → the derivative differentiates the sensor noise, so the
//     *replica count saws up and down* — the autoscaler thrashes nodes every tick
//     even while the queue still looks calm. Push it further and the loop destabilizes
//     outright. This is why the amber line is on the plot: D's failure shows up in what
//     the controller *does*, not just what the queue *is*.
//
// Good tuning threads between them: rise fast, small overshoot, settle at target,
// don't thrash. Drag the three gains and find it — the intuition a Bode plot gives
// an EE and a dashboard never gives an SRE.
//
// The mechanism: **the control loop is a fold, run to a fixed horizon inside one
// cell.** simulate() steps the plant + controller for N ticks and returns the whole
// trace — a pure function of (gains, load, horizon), so scrubbing any slider re-runs
// from t=0 exactly, both directions. Same fixed-horizon-pure shape as the n-body and
// reaction-diffusion notebooks; here the "physics" is a feedback loop. WASM-live.

package pid

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ticks  = 160  // simulation horizon
	target = 50.0 // desired queue depth (the setpoint)
)

// ---------------------------------------------------------------------------
// Inputs — PID gains in hundredths, plus the load step.
// ---------------------------------------------------------------------------

// Proportional gain Kp (×100). Push proportional to the current error. Too high and
// it overshoots and oscillates; too low and it responds sluggishly.
//
//notebook:slider min=0 max=200 step=1
func kpCenti() (kp int) { return 60 }

// Integral gain Ki (×100). Push proportional to accumulated error — this is what
// erases the steady-state offset. Zero it and watch the queue settle off target
// (above it, under load) forever; too high and it winds up and overshoots.
//
//notebook:slider min=0 max=100 step=1
func kiCenti() (ki int) { return 30 }

// Derivative gain Kd (×100). Push against the rate of change — a little damps
// overshoot, but too high and it differentiates the sensor noise, sawing the replica
// count and then destabilizing the loop.
//
//notebook:slider min=0 max=200 step=1
func kdCenti() (kd int) { return 45 }

// The load step: arrivals per tick after the jump at t=0 (before, the system is at
// rest). Bigger steps are harder to absorb without overshoot.
//
//notebook:slider min=20 max=120 step=5
func loadStep() (load int) { return 45 }

// ---------------------------------------------------------------------------
// Compute (Go) — the closed-loop simulation, pure & fixed-horizon.
// ---------------------------------------------------------------------------

// The step response: step the plant and PID controller for `ticks` ticks and return
// the queue-depth and replica-count traces, plus summary metrics (overshoot, settling
// time, steady-state error). Pure in (kp, ki, kd, load) — a fold to a fixed horizon,
// so scrubbing re-runs from t=0 exactly. The plant: each replica drains `service`
// items/tick; the queue is arrivals − drained, floored at zero; the controller sets
// replicas from the PID output.
func simulate(kp int, ki int, kd int, load int) (run Run) {
	p := float64(kp) / 100
	i := float64(ki) / 100
	d := float64(kd) / 100
	const service = 1.0 // items each replica clears per tick

	// Start in steady state AT the target: the system was handling a baseline load
	// (arrivals0) with the queue held at `target`, then load steps up at t=0. This is
	// the true "step response" — the controller must find the new replica count that
	// drains the higher arrival rate while pulling the queue back to target. Starting
	// at rest instead would conflate the step with a cold start.
	const arrivals0 = 40.0
	arrivals := float64(load)

	queue := target
	// baseline replicas provisioned for the pre-step load (queue steady ⇒ drain = in).
	baseline := arrivals0 / service
	integral := 0.0
	prevErr := 0.0

	// Sensor noise: the controller never sees the true queue, only a noisy reading.
	// This is what gives Kd its OWN failure mode — the derivative term differentiates
	// the noise, so a large Kd amplifies it into replica jitter. Deterministic (a fixed
	// LCG, seeded by a constant, NOT the clock), so simulate() stays pure and scrubbing
	// re-runs the identical sequence. Amplitude 0.3 items (~0.6% of target): invisible
	// at the default gains (settles clean), a growing saw-tooth as Kd climbs.
	noise := lcgNoise(ticks, 0.3)

	depth := make([]float64, ticks)
	reps := make([]float64, ticks)
	for t := 0; t < ticks; t++ {
		// error from the MEASURED queue (true depth + sensor noise).
		measured := queue + noise[t]
		err := measured - target
		integral += err
		deriv := err - prevErr
		prevErr = err

		// POSITIONAL PID: replicas are set directly to baseline + the PID terms — not
		// accumulated. This is what gives each gain its textbook role. With Ki=0, the
		// proportional term can only hold the queue where P·err exactly offsets the
		// extra load, which is OFF target (droop); the integral term is the ONLY thing
		// that drives the residual error to zero. An incremental (replicas += …) form
		// would hide that, because the running sum would itself act like an integrator.
		replicas := baseline + p*err + i*integral + d*deriv
		if replicas < 0 {
			replicas = 0
		}

		// plant step: arrivals in, replicas×service drained out.
		drained := replicas * service
		queue += arrivals - drained
		if queue < 0 {
			queue = 0
		}
		depth[t] = queue
		reps[t] = replicas
	}

	return Run{
		Depth: depth, Reps: reps,
		Overshoot:   overshoot(depth),
		SettleTick:  settleTick(depth),
		SteadyError: depth[ticks-1] - target,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The step response: queue depth over time (the controlled signal) with the target
// line, and the replica count the controller chose. Read the shape — overshoot,
// ringing, droop, jitter — the way an EE reads a Bode plot.
//
//notebook:height=420
func response(run Run) (chart Chart) {
	return Chart{Run: run}
}

// The tuning verdict: overshoot, settling time, and steady-state error — the three
// numbers the gains trade off. The captions name which gain to reach for.
func metrics(run Run) (report Readout) {
	settle := "never"
	if run.SettleTick >= 0 {
		settle = strconv.Itoa(run.SettleTick) + " ticks"
	}
	return Readout{Cards: []Card{
		{Label: "overshoot", Value: pct(run.Overshoot / target), Caption: "too high? lower Kp, nudge Kd"},
		{Label: "settling time", Value: settle, Caption: "within ±5% of target"},
		{Label: "steady-state error", Value: f1(run.SteadyError), Caption: "stuck off-target? raise Ki"},
	}}
}

// Your autoscaler is a PID controller — here's the step response.
func intro() (md Markdown) {
	return `An autoscaler holding a queue near a target *is* a PID controller:
correction = **Kp·error + Ki·∫error + Kd·d(error)/dt**. Load jumps at t=0; watch the
blue queue depth recover and the amber replica count the controller chose. Drag the
three gains and see the failures every operator has caused:

- **too much Kp** → overshoot and *oscillation* (over-provision, over-cull, ring);
- **no Ki** → the queue settles *above* target forever — Kp alone under-provisions, and only the integral closes that droop;
- **too much Kd** → it differentiates the sensor noise, *sawing the replica line* — thrashing nodes while the queue looks fine, then destabilizing.

Good tuning threads between them. It's the control-theory intuition a Bode plot
gives an EE and a dashboard never gives an SRE. The loop is a fold run to a fixed
horizon inside one cell — pure in the gains, so scrub freely, both directions.`
}

// ===========================================================================
// Metrics
// ===========================================================================

// overshoot: how far the queue's peak exceeded the target (0 if it never did).
func overshoot(depth []float64) float64 {
	peak := 0.0
	for _, v := range depth {
		if v > peak {
			peak = v
		}
	}
	if peak <= target {
		return 0
	}
	return peak - target
}

// settleTick: first tick after which the queue stays within ±5% of target for the
// rest of the run; -1 if it never settles.
func settleTick(depth []float64) int {
	band := target * 0.05
	for t := 0; t < len(depth); t++ {
		ok := true
		for u := t; u < len(depth); u++ {
			if depth[u] < target-band || depth[u] > target+band {
				ok = false
				break
			}
		}
		if ok {
			return t
		}
	}
	return -1
}

// ===========================================================================
// Helpers
// ===========================================================================

func f1(v float64) string  { return strconv.FormatFloat(v, 'f', 1, 64) }
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

// lcgNoise returns n samples of zero-mean sensor noise in [-amp, +amp], from a fixed
// linear-congruential generator seeded by a constant. Deterministic — no clock, no
// math/rand global — so simulate() is a pure function of its inputs (the corpus rule)
// and scrubbing a slider replays the identical noise. This is what makes Kd's noise-
// amplification failure visible without breaking purity.
func lcgNoise(n int, amp float64) []float64 {
	out := make([]float64, n)
	var s uint32 = 0x9e3779b9 // fixed seed
	for i := range out {
		s = s*1664525 + 1013904223          // Numerical Recipes LCG
		u := float64(s>>8) / float64(1<<24) // → [0,1)
		out[i] = (u*2 - 1) * amp            // → [-amp, +amp]
	}
	return out
}

// unnamed results so the analyzer sees a helper, not a two-output cell.
func rangeOf(xs []float64) (float64, float64) {
	lo, hi := xs[0], xs[0]
	for _, v := range xs {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// ===========================================================================
// Types
// ===========================================================================

// Run is the simulated step response plus its tuning metrics.
type Run struct {
	Depth       []float64
	Reps        []float64
	Overshoot   float64
	SettleTick  int
	SteadyError float64
}

// Chart draws the queue-depth response with the target line and the replica trace.
type Chart struct{ Run Run }

func (c Chart) Render() Rendered {
	r := c.Run
	const w, h, pad = 720.0, 420.0, 44.0
	// One y scale spanning BOTH traces (queue depth and replica count), with the target
	// visible. Plotting both is the point: at high Kd the queue can look calm while the
	// replica line saws up and down — the autoscaler thrashing nodes on sensor noise.
	dlo, dhi := rangeOf(r.Depth)
	rlo, rhi := rangeOf(r.Reps)
	lo, hi := dlo, dhi
	if rlo < lo {
		lo = rlo
	}
	if rhi > hi {
		hi = rhi
	}
	if target > hi {
		hi = target
	}
	if lo > 0 {
		lo = 0
	}
	hi *= 1.1
	if hi <= lo {
		hi = lo + 1
	}
	sx := func(t int) float64 { return pad + float64(t)/float64(ticks-1)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - (v-lo)/(hi-lo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// target line
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-dasharray="5 4"/>`,
		pad, sy(target), w-pad, sy(target))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#64748b">target %.0f</text>`,
		w-pad-56, sy(target)-5, target)

	line := func(series []float64, color string, width float64) {
		var d strings.Builder
		for t, v := range series {
			verb := " L"
			if t == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(t), sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="%.1f"/>`, d.String(), color, width)
	}
	line(r.Reps, "#f59e0b", 1.5)  // replica count — what the controller is DOING
	line(r.Depth, "#4338ca", 2.5) // queue depth — the controlled signal (drawn on top)

	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#334155">queue depth &amp; replica count vs time (step response)</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#4338ca">queue depth</text>`, pad+6, h-pad-8)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#d97706">replicas</text>`, pad+96, h-pad-8)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

// Render draws the readout as a row of labeled cards. Without a Render method a
// composite value carries no MIME and the client leaves the cell hidden — the cards
// would compute and reach no one. This is engine-called, not a cell, so fmt here is
// fine (the fmt→os WASM gate only bites cell bodies).
func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:16px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e2e8f0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#64748b">%s</div>`, esc(c.Label))
		fmt.Fprintf(&b, `<div style="font-size:26px;font-weight:700;color:#1e293b;margin:2px 0">%s</div>`, esc(c.Value))
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#94a3b8">%s</div>`, esc(c.Caption))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
