//go:notebook
//
// AIMD — the sawtooth that makes the internet fair.
//
// TCP congestion control has one job: every flow sharing a link should send as fast
// as it can *without* collapsing the link, and no flow should hog it. The rule that
// does this is **AIMD** — additive increase, multiplicative decrease — and it is the
// reason a thousand connections through one bottleneck settle into an orderly,
// roughly-equal share instead of a shouting match.
//
// The rule, applied to the congestion window (`cwnd`, how many packets a flow keeps
// in flight) once per round trip:
//
//   - **no loss → cwnd += 1** (additive increase): probe gently for more bandwidth.
//   - **loss → cwnd × β** (multiplicative decrease, β≈0.5): back off hard the instant
//     the pipe overflows.
//
// A single flow under this rule traces the famous **sawtooth**: climb linearly until
// the window overruns the pipe and a packet drops, halve, climb again, forever. It
// never sits still at the capacity — which is why a lone flow only ever gets ~¾ of the
// link (the area under a sawtooth), and why β is a real knob: a gentler β keeps the
// window higher and utilization up, but reacts to congestion more slowly.
//
// The deep part is the second chart. Put **two** flows on the pipe, starting wildly
// unequal, and AIMD pulls them *together* — they converge to a fair, equal share.
// This is not obvious, and it is entirely due to the *multiplicative* decrease: when
// both flows cut by the same factor, the absolute gap between them shrinks; additive
// increase then adds the same amount to each, preserving the (now smaller) gap. Over
// many sawteeth the gap goes to zero. Flip the **decrease rule** toggle to
// *additive* decrease (cut by a fixed amount, not a factor) and watch fairness break:
// the gap never closes, because subtracting a constant from both flows leaves their
// difference untouched. That toggle is the whole Chiu–Jain argument in one drag —
// multiplicative decrease is not a detail, it's the mechanism.
//
// The mechanism here: **each run is a fold to a fixed horizon inside one cell** —
// step cwnd for N round trips and return the whole trace. Pure in (capacity, β,
// decrease-rule, horizon), no RNG (loss is deterministic: it happens exactly when the
// window overruns the pipe), so scrubbing any slider re-runs from t=0 exactly. Same
// fixed-horizon-pure shape as the retry-storm and PID notebooks. WASM-live.

package aimd

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ticks    = 200  // round trips simulated (the fixed horizon)
	capacity = 40.0 // packets the bottleneck can hold in flight before it drops
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Decrease factor β (×100) — how hard a flow cuts its window on loss. Classic TCP is
// 50 (halve). Gentler (higher) keeps utilization up but reacts slowly; harsher (lower)
// is twitchy and wastes bandwidth.
//
//notebook:slider min=20 max=95 step=5
func betaPct() (beta int) { return 50 }

// Additive-increase step — how many packets the window grows per loss-free round trip.
// Classic TCP is 1. Bigger climbs the sawtooth faster (steeper teeth).
//
//notebook:slider min=1 max=8 step=1
func increaseStep() (ai int) { return 1 }

// Decrease rule: 0 = MULTIPLICATIVE (cwnd × β — real TCP, converges to fair sharing),
// 1 = ADDITIVE (cwnd − k — the counterexample: fairness never converges). Flip it on
// the two-flow chart and watch the gap refuse to close.
//
//notebook:slider min=0 max=1 step=1
func decreaseRule() (rule int) { return 0 }

// ---------------------------------------------------------------------------
// Compute (Go) — the AIMD folds, pure & fixed-horizon.
// ---------------------------------------------------------------------------

// single runs one flow under the chosen AIMD parameters and returns its cwnd trace
// plus utilization (delivered ÷ capacity over the run). Deterministic loss: the window
// overruns the pipe exactly when cwnd > capacity, so the sawtooth is reproducible.
// Pure in (beta, ai, rule).
func single(beta int, ai int, rule int) (flow Flow) {
	b := float64(beta) / 100
	step := float64(ai)
	cut := decrease(rule, b)

	cwnd := 1.0
	trace := make([]float64, ticks)
	delivered := 0.0
	losses := 0
	for t := 0; t < ticks; t++ {
		if cwnd > capacity {
			cwnd = cut(cwnd)
			losses++
		} else {
			cwnd += step
		}
		trace[t] = cwnd
		sent := cwnd
		if sent > capacity {
			sent = capacity
		}
		delivered += sent
	}
	return Flow{
		Cwnd:        trace,
		Utilization: delivered / (float64(ticks) * capacity),
		Losses:      losses,
	}
}

// pair runs TWO flows sharing the one pipe, starting deliberately unequal, and returns
// both cwnd traces plus the fairness gap over time (|a−b|). With multiplicative
// decrease the gap converges to zero (fair); with additive decrease it does not. Both
// flows see the same loss event (the shared pipe overflowed) and react together. Pure.
func pair(beta int, ai int, rule int) (duo Pair) {
	b := float64(beta) / 100
	step := float64(ai)
	cut := decrease(rule, b)

	// start wildly unequal so convergence (or its absence) is unmistakable.
	fa, fb := 1.0, capacity*0.85
	ta := make([]float64, ticks)
	tb := make([]float64, ticks)
	gap := make([]float64, ticks)
	for t := 0; t < ticks; t++ {
		if fa+fb > capacity {
			fa = cut(fa)
			fb = cut(fb)
		} else {
			fa += step
			fb += step
		}
		if fa < 1 {
			fa = 1
		}
		if fb < 1 {
			fb = 1
		}
		ta[t], tb[t] = fa, fb
		d := fa - fb
		if d < 0 {
			d = -d
		}
		gap[t] = d
	}
	return Pair{A: ta, B: tb, Gap: gap, FinalGap: gap[ticks-1], Rule: rule}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The sawtooth: one flow's congestion window over time. Climb by the increase step,
// halve (or subtract) on loss, repeat. The dashed line is the pipe capacity — the
// window is forever probing just past it and backing off. The gap between the teeth
// and the ceiling is the bandwidth a single flow leaves on the table.
//
//notebook:height=320
func sawtooth(flow Flow) (saw Saw) {
	return Saw{Flow: flow}
}

// The fairness chart: two flows sharing the pipe, started unequal. With multiplicative
// decrease they converge to an equal share (the gap collapses to zero); flip to
// additive decrease and the gap never closes — the whole reason TCP's decrease is
// multiplicative, shown not asserted.
//
//notebook:height=320
func fairness(duo Pair) (fair Fair) {
	return Fair{Pair: duo}
}

// The verdict: single-flow utilization and the two-flow fairness outcome. Utilization
// rises with a gentler β; fairness converges only under multiplicative decrease.
func verdict(flow Flow, duo Pair) (report Readout) {
	fair := "converging → fair share"
	if duo.FinalGap > 1.0 {
		fair = "STUCK unfair (gap " + f1(duo.FinalGap) + ")"
	}
	rule := "multiplicative (×β)"
	if duo.Rule == 1 {
		rule = "additive (−k) — the counterexample"
	}
	return Readout{Cards: []Card{
		{Label: "single-flow utilization", Value: pct(flow.Utilization), Caption: "a lone flow leaves the rest on the table (sawtooth)"},
		{Label: "losses", Value: strconv.Itoa(flow.Losses), Caption: "each is one tooth of the saw"},
		{Label: "decrease rule", Value: rule, Caption: "the toggle that makes or breaks fairness"},
		{Label: "two-flow fairness", Value: fair, Caption: "equal share only under multiplicative decrease"},
	}}
}

// AIMD — the sawtooth that makes the internet fair.
func intro() (md Markdown) {
	return `TCP congestion control is **AIMD**: each round trip, no loss → window
**+1** (probe gently), loss → window **×β** (back off hard). One flow traces the famous
**sawtooth** — climb, overrun the pipe, halve, climb — so it only ever gets ~¾ of the
link (the area under the teeth). β is a real knob: gentler keeps utilization up but
reacts slowly.

The deep part is the second chart. Two flows starting **unequal** converge to a **fair,
equal share** — and *only* because the decrease is multiplicative: cutting both by the
same factor shrinks the gap between them; additive increase preserves it, so over many
teeth the gap → 0. Flip the **decrease rule** to *additive* and fairness breaks — the
gap never closes. That toggle is the whole Chiu–Jain argument in one drag.

Each run is a fold to a fixed horizon in one cell — pure, deterministic loss, no RNG.
Scrub freely.`
}

// ===========================================================================
// The AIMD rule
// ===========================================================================

// decrease returns the window-cut function for the chosen rule: multiplicative (× β)
// or additive (− a fixed amount). This one switch is the entire subject of the
// fairness chart — multiplicative shrinks the gap between flows, additive doesn't.
func decrease(rule int, beta float64) func(float64) float64 {
	if rule == 1 {
		// additive decrease: subtract a fixed fraction of capacity (a constant), which
		// is what leaves the gap between two flows untouched.
		const k = capacity * 0.4
		return func(c float64) float64 {
			if c-k < 1 {
				return 1
			}
			return c - k
		}
	}
	return func(c float64) float64 { return c * beta }
}

// ===========================================================================
// Helpers
// ===========================================================================

func f1(v float64) string  { return strconv.FormatFloat(v, 'f', 1, 64) }
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

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

// Flow is one AIMD flow's window trace plus its summary metrics.
type Flow struct {
	Cwnd        []float64
	Utilization float64
	Losses      int
}

// Pair is two flows sharing a pipe, with the fairness gap over time.
type Pair struct {
	A, B     []float64
	Gap      []float64
	FinalGap float64
	Rule     int
}

// Saw draws the single-flow sawtooth against the capacity line.
type Saw struct{ Flow Flow }

func (s Saw) Render() Rendered {
	return plotSeries(
		"congestion window vs round trip — the AIMD sawtooth",
		[]series{{s.Flow.Cwnd, "#2a78d6", "cwnd"}},
		true,
	)
}

// Fair draws the two-flow convergence (or not).
type Fair struct{ Pair Pair }

func (f Fair) Render() Rendered {
	title := "two flows sharing the pipe — converging to a fair share"
	if f.Pair.Rule == 1 {
		title = "two flows, ADDITIVE decrease — fairness never converges"
	}
	return plotSeries(
		title,
		[]series{
			{f.Pair.A, "#2a78d6", "flow A"},
			{f.Pair.B, "#0797b8", "flow B"},
		},
		true,
	)
}

type series struct {
	data  []float64
	color string
	label string
}

// plotSeries draws one or more cwnd traces on a shared time/value grid, with the
// capacity line. Shared by both charts so the sawtooth and the fairness view read
// identically. Engine-called (via Render), not a cell — fmt is fine here.
func plotSeries(title string, ss []series, showCap bool) Rendered {
	const w, h, pad = 720.0, 320.0, 42.0
	plotW, plotH := w-2*pad, h-2*pad
	hi := capacity
	for _, s := range ss {
		if m := maxOf(s.data); m > hi {
			hi = m
		}
	}
	hi *= 1.1
	n := 0
	for _, s := range ss {
		if len(s.data) > n {
			n = len(s.data)
		}
	}
	if n < 2 {
		n = 2
	}
	sx := func(t int) float64 { return pad + float64(t)/float64(n-1)*plotW }
	sy := func(v float64) float64 { return h - pad - v/hi*plotH }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, plotW, plotH)

	if showCap {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#5b6472" stroke-dasharray="5 4"/>`,
			pad, sy(capacity), w-pad, sy(capacity))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#5b6472">pipe capacity %.0f</text>`,
			w-pad-104, sy(capacity)-5, capacity)
	}

	for _, s := range ss {
		var d strings.Builder
		for t, v := range s.data {
			verb := " L"
			if t == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(t), sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2.2"/>`, d.String(), s.color)
	}

	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#1b3a6b">%s</text>`, pad, esc(title))
	lx := pad + 6
	for _, s := range ss {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill=%q>%s</text>`,
			lx, h-pad-8, s.color, esc(s.label))
		lx += float64(len(s.label))*7 + 18
	}
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

// Render draws the verdict cards. A composite value with no Render method carries no
// MIME and the client hides the cell — so the cards render themselves. Engine-called,
// not a cell, so fmt here is fine.
func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:14px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e7ebf0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, esc(c.Label))
		fmt.Fprintf(&b, `<div style="font-size:20px;font-weight:700;color:#1b3a6b;margin:2px 0">%s</div>`, esc(c.Value))
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#5b6472">%s</div>`, esc(c.Caption))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
