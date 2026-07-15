//go:notebook
//
// The retry storm — watch a healthy service tip into metastable collapse.
//
// A service is humming along under rising load. Then it crosses a line, goodput
// falls off a cliff — and here's the part every postmortem describes and no
// dashboard predicts: **backing the load off doesn't bring it back.** The system
// has two stable states, "serving" and "collapsed," and once it's in the second one
// it stays there long after the traffic that pushed it over is gone. That gap is
// **metastability**, and it's not a bug in any one component — it's a property of the
// feedback loop.
//
// The loop is the retry. When requests start failing (timeouts, 5xx), clients retry.
// Retries are *more load* — on a server already past capacity. The extra load causes
// more failures, which cause more retries. Offered load becomes demand = new work +
// everyone's retries, and the retries alone can exceed the whole capacity. So the
// server stays pinned even as the original traffic recedes: the storm is feeding
// itself.
//
// Drag the sliders and find the cliff:
//
//   - **offered peak** — how hard the load ramps (up then back down, a triangle).
//   - **retry %** — how much of each failure comes back as a retry. This is the gain
//     on the feedback loop; turn it up and the hysteresis gap widens.
//   - **circuit breaker** — the fix. When the failure rate crosses the breaker's
//     threshold it *sheds retries* (fails fast instead of piling on). Watch what it
//     does to the phase loop: it doesn't stop you tipping over, it lets you climb
//     back out — the loop closes.
//
// Two views read the same run: the **timeline** (offered vs goodput over time — see
// the cliff) and the **phase loop** (goodput vs offered — the *area* of the loop IS
// the hysteresis; a healthy system traces a line, a metastable one traces a loop you
// can't retrace). Set the breaker wide open and the loop yawns; tighten it and it
// snaps shut.
//
// The mechanism: **the storm is a fold run to a fixed horizon inside one cell.**
// storm() steps demand + retries for N ticks and returns the whole trace — a pure
// function of (peak, retry, breaker, horizon), so scrubbing any slider re-runs from
// t=0 exactly, both directions. No RNG: the load profile is deterministic, so the
// collapse is reproducible and the phase loop is stable under a scrub. Same
// fixed-horizon-pure shape as the PID and n-body notebooks. WASM-live.

package retrystorm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	ticks    = 240   // simulation horizon
	capacity = 100.0 // requests/tick the server can actually complete
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Offered peak — the top of the load ramp, in requests/tick. Load rises from zero to
// this peak and back down (a triangle), so we see both the tip-over and the (failed)
// recovery. The server's true capacity is 100/tick; push the peak past it.
//
//notebook:slider min=60 max=260 step=5
func offeredPeak() (peak int) { return 150 }

// Retry percentage — the share of each failed request that comes back as a retry.
// This is the GAIN on the feedback loop: at 0 there's no storm (failures just fail);
// crank it up and the retries alone can exceed capacity, so the system stays pinned
// even after the load recedes. The hysteresis gap grows with it.
//
//notebook:slider min=0 max=100 step=5
func retryPct() (retry int) { return 85 }

// Circuit breaker threshold — the failure rate (%) at which the breaker trips and
// sheds retries (fail fast, stop piling on). 100 = disabled (never trips): drag it
// down to arm it. This is the fix — watch it close the phase loop.
//
//notebook:slider min=20 max=100 step=5
func breakerPct() (breaker int) { return 100 }

// ---------------------------------------------------------------------------
// Compute (Go) — the storm as a fixed-horizon fold, pure.
// ---------------------------------------------------------------------------

// storm steps the retry feedback loop for `ticks` ticks and returns the offered and
// goodput traces plus the metastability verdict. Pure in (peak, retry, breaker): a
// fold to a fixed horizon, so scrubbing re-runs from t=0 exactly and the phase loop
// is stable. Each tick: demand = fresh offered load + retries still in flight. If
// demand is within capacity the server clears it all; past capacity, goodput
// collapses (the more overloaded, the worse — timeouts and contention). Failed
// requests retry at `retry`%, UNLESS the failure rate has crossed the breaker, which
// sheds them. Those surviving retries are next tick's extra demand — the loop.
func storm(peak int, retry int, breaker int) (run Storm) {
	retryFrac := float64(retry) / 100
	trip := float64(breaker) / 100 // fail-rate threshold; 1.0 ⇒ never trips

	offered := loadProfile(peak)
	good := make([]float64, ticks)

	const alpha = 3.0 // how sharply goodput collapses past capacity
	inFlight := 0.0   // retries carried into the next tick
	for t := 0; t < ticks; t++ {
		demand := offered[t] + inFlight

		completed := demand
		if demand > capacity {
			// past capacity the server thrashes: goodput decays exponentially in
			// the overload ratio, so a 2× overload clears far less than half.
			completed = capacity * math.Exp(-alpha*(demand/capacity-1))
		}
		failed := demand - completed

		failRate := 0.0
		if demand > 0 {
			failRate = failed / demand
		}
		// The breaker sheds retries once failures cross its threshold — the whole
		// point is to break the loop, not to serve more this instant.
		p := retryFrac
		if failRate > trip {
			p = 0
		}
		inFlight = failed * p
		good[t] = completed
	}

	return Storm{
		Offered: offered, Good: good,
		PeakGood:  maxOf(good),
		Collapsed: collapsed(offered, good),
		GapUp:     hysteresisUp(offered, good),
		GapDown:   hysteresisDown(offered, good),
	}
}

// ---------------------------------------------------------------------------
// Views — two reads of the same storm (the linked-views pattern).
// ---------------------------------------------------------------------------

// The timeline: offered load and goodput over time. Watch goodput track offered up
// the ramp, fall off the cliff at the tip-over — and (without a breaker) stay on the
// floor all the way back down, even as offered load returns to nothing.
//
//notebook:height=340
func timeline(run Storm) (series Timeline) {
	return Timeline{Run: run}
}

// The phase loop: goodput plotted against offered load, swept up then down. A healthy
// system retraces its own line; a metastable one traces a LOOP — it collapses at a
// high offered load on the way up but only recovers at a much lower one on the way
// down, so the up-path and down-path don't meet. The enclosed area is the
// hysteresis. Arm the breaker and the loop snaps shut.
//
//notebook:height=360
func phase(run Storm) (loop Loop) {
	return Loop{Run: run}
}

// The verdict: what state is the system in, and how wide is the trap? The distinction
// the notebook turns on: a raw overload degrades but RECOVERS at the same load (a
// straight line in phase space); the retry loop makes it METASTABLE — it recovers
// only at a much lower load, so there's a gap. The gap, not the mere fact of a dip, is
// the finding.
func verdict(run Storm) (report Readout) {
	gapWidth := 0.0
	if run.GapUp > run.GapDown {
		gapWidth = run.GapUp - run.GapDown
	}
	metastable := run.Collapsed && gapWidth > 5

	state := "served"
	switch {
	case metastable:
		state = "METASTABLE"
	case run.Collapsed:
		state = "saturated"
	}
	gap := "none"
	if gapWidth > 5 {
		gap = f0(gapWidth) + " req/tick"
	}
	return Readout{Cards: []Card{
		{Label: "outcome", Value: state, Caption: "metastable = stuck collapsed after load eases"},
		{Label: "peak goodput", Value: f0(run.PeakGood) + " /tick", Caption: "server capacity is " + f0(capacity)},
		{Label: "hysteresis gap", Value: gap, Caption: "load band where it's trapped — arm the breaker to close it"},
	}}
}

// The retry storm — watch a healthy service tip into metastable collapse.
func intro() (md Markdown) {
	return `A service under rising load crosses a line and goodput falls off a cliff.
The postmortem detail no dashboard predicts: **backing the load off doesn't bring it
back.** Two stable states — serving and collapsed — and the retry loop holds it in
the second one long after the traffic that caused it is gone.

Drag **retry %** (the gain on the loop): failures become retries, retries are *more
load* on an already-drowning server, which makes more failures. Watch the **phase
loop** (goodput vs offered) — a healthy system traces a line, a metastable one traces
a loop you can't retrace. Then arm the **circuit breaker**: it doesn't stop you
tipping over, it *closes the loop* so you can climb back out.

The storm is a fold run to a fixed horizon in one cell — pure in the params, no RNG,
so scrub freely, both directions.`
}

// ===========================================================================
// Metrics
// ===========================================================================

// loadProfile is the deterministic triangle: 0 → peak → 0 over the horizon. No RNG,
// so the storm (and its phase loop) is reproducible and stable under a scrub.
func loadProfile(peak int) []float64 {
	out := make([]float64, ticks)
	for t := 0; t < ticks; t++ {
		f := float64(t) / float64(ticks-1)
		if f <= 0.5 {
			out[t] = float64(peak) * (f / 0.5)
		} else {
			out[t] = float64(peak) * (1 - (f-0.5)/0.5)
		}
	}
	return out
}

// collapsed reports whether goodput ever cratered — dropped below half the offered
// load while the offered load was itself well above a floor (i.e. a real cliff, not
// just the ramp's endpoints).
func collapsed(offered, good []float64) bool {
	for t := range offered {
		if offered[t] > capacity*0.3 && good[t] < offered[t]*0.5 {
			return true
		}
	}
	return false
}

// hysteresisUp: the offered load at which goodput first collapses on the way UP (the
// tip-over point). 0 if it never collapses.
func hysteresisUp(offered, good []float64) float64 {
	peakT := ticks / 2
	for t := 0; t < peakT; t++ {
		if offered[t] > capacity*0.3 && good[t] < offered[t]*0.5 {
			return offered[t]
		}
	}
	return 0
}

// hysteresisDown: the offered load at which goodput is still collapsed latest on the
// way DOWN (the recovery point). 0 if it recovers immediately / never collapsed.
func hysteresisDown(offered, good []float64) float64 {
	peakT := ticks / 2
	for t := ticks - 1; t > peakT; t-- {
		if offered[t] > capacity*0.3 && good[t] < offered[t]*0.5 {
			return offered[t]
		}
	}
	return 0
}

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, v := range xs {
		if v > m {
			m = v
		}
	}
	return m
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// ===========================================================================
// Types
// ===========================================================================

// Storm is the simulated retry storm: the two traces plus the metastability verdict.
type Storm struct {
	Offered   []float64
	Good      []float64
	PeakGood  float64
	Collapsed bool
	GapUp     float64 // offered load at collapse (up-sweep)
	GapDown   float64 // offered load at recovery (down-sweep)
}

// Timeline draws offered load and goodput against time — the cliff.
type Timeline struct{ Run Storm }

func (tl Timeline) Render() Rendered {
	r := tl.Run
	const w, h, pad = 720.0, 340.0, 42.0
	hi := maxOf(r.Offered)
	if m := maxOf(r.Good); m > hi {
		hi = m
	}
	hi *= 1.1
	if hi <= 0 {
		hi = 1
	}
	sx := func(t int) float64 { return pad + float64(t)/float64(ticks-1)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/hi*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// capacity line
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#cbd5e1" stroke-dasharray="5 4"/>`,
		pad, sy(capacity), w-pad, sy(capacity))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#94a3b8">capacity %.0f</text>`,
		w-pad-70, sy(capacity)-5, capacity)

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
	line(r.Offered, "#94a3b8", 1.8) // offered load
	line(r.Good, "#dc2626", 2.6)    // goodput — the thing that cliffs

	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#334155">offered load &amp; goodput vs time (load ramps up, then back down)</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#64748b">offered</text>`, pad+6, pad+16)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#dc2626">goodput</text>`, pad+60, pad+16)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// Loop draws the phase portrait: goodput (y) against offered load (x), swept over the
// whole run. The up-path and down-path diverge into a loop when the system is
// metastable — that enclosed area is the hysteresis.
type Loop struct{ Run Storm }

func (lp Loop) Render() Rendered {
	r := lp.Run
	const w, h, pad = 720.0, 360.0, 46.0
	hx := maxOf(r.Offered) * 1.05
	hy := maxOf(r.Offered) * 1.05 // square-ish: goodput ≤ offered, share the scale
	if hx <= 0 {
		hx = 1
	}
	if hy <= 0 {
		hy = 1
	}
	sx := func(v float64) float64 { return pad + v/hx*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/hy*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, w-2*pad, h-2*pad)

	// the "healthy" diagonal: goodput == offered (what a system with no collapse traces)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#cbd5e1" stroke-dasharray="4 4"/>`,
		sx(0), sy(0), sx(math.Min(hx, hy)), sy(math.Min(hx, hy)))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#94a3b8">goodput = offered (healthy)</text>`,
		sx(hx*0.55), sy(hx*0.62))

	// the swept path, colored up (red) vs down (blue) so the loop reads directionally.
	seg := func(from, to int, color string) {
		var d strings.Builder
		for t := from; t <= to && t < ticks; t++ {
			verb := " L"
			if t == from {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(r.Offered[t]), sy(r.Good[t]))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2.4"/>`, d.String(), color)
	}
	peakT := ticks / 2
	seg(0, peakT, "#dc2626")       // up-sweep
	seg(peakT, ticks-1, "#2563eb") // down-sweep

	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#334155">phase loop — goodput vs offered (the enclosed area is the hysteresis)</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#dc2626">load rising</text>`, pad+6, h-pad-24)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#2563eb">load falling</text>`, pad+6, h-pad-8)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#64748b">offered load →</text>`, w-pad-90, h-pad+22)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

// Render draws the verdict as a row of labeled cards. A composite value with no Render
// method carries no MIME and the client leaves the cell hidden — so the cards must
// render themselves. Engine-called, not a cell, so fmt here is fine.
func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:16px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		b.WriteString(`<div style="flex:1;min-width:160px;border:1px solid #e2e8f0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#64748b">%s</div>`, esc(c.Label))
		fmt.Fprintf(&b, `<div style="font-size:24px;font-weight:700;color:#1e293b;margin:2px 0">%s</div>`, esc(c.Value))
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#94a3b8">%s</div>`, esc(c.Caption))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
