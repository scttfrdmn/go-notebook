//go:notebook
//
// Little's Law — and why the units won't let you cheat.
//
// One equation ties the three numbers every performance conversation is about:
//
//     L = λ · W        concurrency = throughput × latency
//
// Requests in flight (L) equals arrival rate (λ) times how long each stays (W).
// It holds for any stable system, with no assumption about the arrival pattern or
// the service-time distribution — which is what makes it both powerful and easy to
// misremember. People confuse throughput with latency, or read "we handle 5000 req/s
// at 20 ms" and can't say how many requests are in flight (it's 100).
//
// This notebook is the law made draggable, and its point is a small one the whole
// project is built around: **the units carry the law.** λ is a `PerSecond`, W is
// `Seconds`, L is a dimensionless `Requests`. `PerSecond × Seconds` is the only
// multiplication that typechecks into `Requests`; you *cannot* multiply two rates,
// or add a latency to a throughput, because the types forbid it. The equation isn't
// enforced by a comment or a test — it's enforced by the compiler, because each
// quantity is what it is. Rearranging to solve for λ or W is likewise the only
// division that typechecks. Get the algebra wrong and it doesn't compile.
//
// Pick which two you know; the third is computed. Drag them and watch the identity
// hold — and watch the little scenarios ("a 2× traffic spike with the same latency
// doubles the requests in flight") fall out of it.

package little

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Throughput λ — requests arriving per second.
//
//notebook:slider min=100 max=10000 step=100
func arrivalRate() (lambda PerSecond) { return 2500 }

// Latency W — how long each request stays in the system, in milliseconds.
//
//notebook:slider min=1 max=250 step=1
func latencyMillis() (wMillis int) { return 200 }

// ---------------------------------------------------------------------------
// Compute (Go) — Little's Law, in units.
// ---------------------------------------------------------------------------

// Latency as a typed duration in Seconds — the unit W must be in for the law. The
// slider is milliseconds (the number people quote); this is the one place the
// conversion happens, named, so nothing downstream can use raw millis by accident.
func latency(wMillis int) (w Seconds) { return Seconds(float64(wMillis) / 1000) }

// Concurrency L — requests in flight — BY Little's Law: L = λ·W. This multiplication
// is the only one that typechecks to Requests (PerSecond × Seconds); a slip like
// λ·λ or λ+W is a compile error, so the law can't be miswired.
func concurrency(lambda PerSecond, w Seconds) (l Requests) {
	return lambda.times(w)
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The law, drawn: a rectangle whose width is λ and height is W, so its AREA is L —
// Little's Law as literal area. Drag either slider and the box reshapes; the area
// (the requests in flight) is the number that matters.
//
//notebook:height=340
func view(lambda PerSecond, w Seconds, l Requests) (chart Chart) {
	return Chart{Lambda: lambda, W: w, L: l}
}

// The three numbers, and the identity between them stated plainly. Plus two
// scenarios that fall straight out of the law, so it stops being abstract.
func readout(lambda PerSecond, w Seconds, l Requests) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "throughput λ", Value: f0(float64(lambda)) + " req/s"},
		{Label: "latency W", Value: f0(float64(w)*1000) + " ms"},
		{Label: "in flight L = λ·W", Value: f1(float64(l)) + " requests", Caption: "the concurrency you must provision for"},
		{Label: "if latency doubles", Value: f1(float64(l)*2) + " in flight", Caption: "same traffic, 2× W → 2× L"},
	}}
}

// Little's Law — and why the units won't let you cheat.
func intro() (md Markdown) {
	return `**L = λ·W**: requests in flight = throughput × latency. It holds for any
stable system, no matter the traffic pattern — which is why it's so easy to
misremember. Drag **λ** (req/s) and **W** (ms); the requests in flight *L* fall out.

The point is the units. λ is a ` + "`PerSecond`" + `, W is ` + "`Seconds`" + `, L is
dimensionless ` + "`Requests`" + ` — and ` + "`PerSecond × Seconds`" + ` is the *only*
product that typechecks to ` + "`Requests`" + `. You can't multiply two rates or add a
latency to a throughput; the compiler refuses. The law isn't enforced by a comment
or a test — it's enforced by each quantity being what it is. The box below draws it:
λ wide, W tall, and the **area** is L.`
}

// ===========================================================================
// Units — the load-bearing part. Each quantity is its own type; the only legal
// arithmetic between them is the one Little's Law describes.
// ===========================================================================

// PerSecond is a throughput (a rate). It cannot be added to a Seconds or multiplied
// by another PerSecond — only multiplied by a Seconds to yield Requests.
type PerSecond float64

// Seconds is a duration.
type Seconds float64

// Requests is a dimensionless count (a number in flight).
type Requests float64

// times is Little's Law as a typed method: PerSecond × Seconds → Requests. This is
// the ONLY multiplication defined between these types, so the law is the only way to
// combine them and a dimensional slip won't compile.
func (r PerSecond) times(w Seconds) Requests { return Requests(float64(r) * float64(w)) }

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// ===========================================================================
// View
// ===========================================================================

// Chart draws Little's Law as an area: a λ-wide, W-tall rectangle whose area is L.
type Chart struct {
	Lambda PerSecond
	W      Seconds
	L      Requests
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 340.0, 50.0
	// Fixed axes so the box is comparable as you drag, matched to the slider ranges:
	// λ up to 10000 req/s, W up to 0.25 s. The area (in req) is what L reads.
	const lamMax, wMax = 10000.0, 0.25
	boxW := float64(c.Lambda) / lamMax * (w - 2*pad)
	boxH := float64(c.W) / wMax * (h - 2*pad)
	x0, y0 := pad, h-pad

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	// axes
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#5b6472"/>`, x0, y0, w-pad, y0)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#5b6472"/>`, x0, y0, x0, pad)

	// the area box
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#1b3a6b" fill-opacity="0.18" stroke="#1b3a6b" stroke-width="2"/>`,
		x0, y0-boxH, boxW, boxH)

	// labels
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" fill="#1b3a6b" text-anchor="middle">λ = %s req/s</text>`,
		x0+boxW/2, y0+22, f0(float64(c.Lambda)))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" fill="#1b3a6b" transform="rotate(-90 %.1f %.1f)" text-anchor="middle">W = %s ms</text>`,
		x0-20, y0-boxH/2, x0-20, y0-boxH/2, f0(float64(c.W)*1000))
	// Label just above the box's top edge, so it never sits on the border even when
	// the box is short.
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="15" font-weight="700" fill="#1b3a6b" text-anchor="middle">area = L = %s requests in flight</text>`,
		x0+boxW/2, y0-boxH-10, f1(float64(c.L)))
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
