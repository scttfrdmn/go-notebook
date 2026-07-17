//go:notebook
//
// Bandwidth-delay product — why a 10-gigabit link can run at dial-up speed.
//
// You have a fast link and a big file, and the transfer crawls. The bandwidth is
// there, the link is idle, and still you get a fraction of a percent of it. The
// culprit is almost never bandwidth — it's the **window**, and the number that ties
// them together is the **bandwidth-delay product**.
//
// TCP keeps a *window* of un-acknowledged data in flight: it sends a window's worth,
// then waits a round trip for the acknowledgements before it can send more. So the
// most it can ever push is one window per round trip:
//
//     throughput  =  min( bandwidth ,  window / RTT )
//                                       ^^^^^^^^^^^^
//                                       the ceiling the window imposes
//
// To keep a link *full*, the window has to cover everything in flight during one round
// trip — and that quantity is the **bandwidth-delay product**:
//
//     BDP  =  bandwidth × RTT
//
// If your window is smaller than the BDP you are **window-limited**: you send a
// burst, then sit idle waiting for ACKs, and the link runs far below its rate no
// matter how fat it is. Only once the window reaches the BDP do you become
// **bandwidth-limited** and actually use the link. This is why the classic 64 KB
// default window is fine on a LAN and catastrophic on a "fat long pipe" — a
// high-bandwidth, high-latency path (transcontinental, satellite, a busy WAN). The
// BDP of a 10 Gb / 150 ms link is ~180 **megabytes**; a 64 KB window fills 0.03% of it.
//
// Drag the three sliders and watch the two regimes:
//
//   - **bandwidth** and **RTT** set the BDP (the window you'd *need*).
//   - **window** is what you *have*. The chart sweeps throughput against window size:
//     a straight climb (window-limited) that knees over into a flat plateau at line
//     rate (bandwidth-limited). The knee sits exactly at the BDP. Left of it you're
//     starving the link; right of it a bigger window buys nothing.
//
// The units carry the arithmetic so the mistake can't be written: bandwidth is
// `Megabits` per second, RTT is `Milliseconds`, window and BDP are `Kilobytes`, and
// throughput is a `Megabits` rate built from bytes ÷ a duration. You cannot add an
// RTT to a bandwidth or read a window as a rate — each is its own type, the fleet
// dollars-vs-kilograms lesson applied to a link budget.
//
// Pure arithmetic — a pure function of (bandwidth, RTT, window), so scrub freely.
// WASM-live.

package bdp

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Link bandwidth (Mbit/s). The rate the link *could* deliver — the plateau the
// throughput curve tops out at once the window is big enough.
//
//notebook:slider min=10 max=10000 step=10
func bandwidth() (bw int) { return 10000 }

// Round-trip time (ms). Distance and hops to the far end and back. Long RTT is what
// makes a fat pipe hard to fill — it multiplies into the BDP.
//
//notebook:slider min=1 max=600 step=1
func roundTrip() (rtt int) { return 150 }

// TCP window (KB) — how much unacknowledged data is allowed in flight at once. The
// classic default is 64. This is the knob you actually control; compare it to the BDP.
//
//notebook:slider min=16 max=262144 step=16
func windowKB() (window int) { return 64 }

// ---------------------------------------------------------------------------
// Compute (Go) — the link, its BDP, and the throughput-vs-window curve.
// ---------------------------------------------------------------------------

// link assembles the path characteristics into a typed Link. Its own cell so the graph
// shows bandwidth and RTT feeding one link description.
func link(bw int, rtt int) (path Link) {
	return Link{Bandwidth: Megabits(bw), RTT: Milliseconds(rtt)}
}

// analyze computes the bandwidth-delay product, the throughput the chosen window
// actually achieves, and the full throughput-vs-window curve. Pure in (path, window):
// throughput is min(bandwidth, window/RTT), computed in units so a window can't be
// mistaken for a rate. The curve is sampled geometrically so the window-limited climb
// and the bandwidth-limited plateau are both visible on a log x-axis.
func analyze(path Link, window int) (result Result) {
	bdp := path.bdp() // Kilobytes needed to fill the link

	const lo, hi = 16.0, 262144.0 // KB, matches the window slider range
	const n = 120
	sizes := make([]float64, n)
	tput := make([]float64, n)
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n-1)
		kb := lo * math.Pow(hi/lo, f)
		sizes[i] = kb
		tput[i] = float64(path.throughput(Kilobytes(kb)))
	}

	achieved := path.throughput(Kilobytes(window))
	return Result{
		Path:      path,
		BDP:       float64(bdp),
		Window:    window,
		Achieved:  float64(achieved),
		Sizes:     sizes,
		Tput:      tput,
		WindowLtd: float64(window) < float64(bdp),
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The throughput curve: achieved rate vs window size (log x). A straight climb while
// window-limited, kneeing into a flat plateau at line rate once the window reaches the
// BDP. The green marker is the BDP (the window you'd need); the dashed line is the
// window you picked. If your marker is left of the BDP, you're starving the link.
//
//notebook:height=400
func curveChart(result Result) (chart Chart) {
	return Chart{R: result}
}

// The verdict: the BDP you need, the window you have, the throughput you get, and
// which regime you're in. Slide the window past the BDP and watch the link fill.
func verdict(result Result) (report Readout) {
	regime := "bandwidth-limited — the link is full"
	if result.WindowLtd {
		regime = "WINDOW-LIMITED — starving the link"
	}
	return Readout{Cards: []Card{
		{Label: "bandwidth-delay product", Value: humanKB(result.BDP), Caption: "the window needed to fill this link"},
		{Label: "your window", Value: humanKB(float64(result.Window)), Caption: "what you have in flight"},
		{Label: "throughput", Value: humanRate(result.Achieved), Caption: "of " + humanRate(float64(result.Path.Bandwidth)) + " possible"},
		{Label: "link utilization", Value: pct(result.Achieved / float64(result.Path.Bandwidth)), Caption: regime},
	}}
}

// Bandwidth-delay product — why a 10-gigabit link can run at dial-up speed.
func intro() (md Markdown) {
	return `TCP sends one **window** of data, then waits a round trip for the ACKs — so
throughput is capped at **window / RTT**, and to fill a link the window must cover the
**bandwidth × RTT** in flight during that round trip. That product is the
**bandwidth-delay product**.

A window smaller than the BDP leaves you **window-limited**: burst, idle, burst, and
the link runs far below its rate however fat it is. This is why a 64 KB default is fine
on a LAN and catastrophic on a *fat long pipe* — the BDP of a 10 Gb / 150 ms link is
~180 **megabytes**, and 64 KB fills 0.03% of it.

Drag bandwidth and RTT (they set the BDP you *need*) and the window (what you *have*).
The curve climbs while window-limited, then knees flat at line rate once the window
reaches the BDP — the green marker. Units keep megabits, milliseconds, and kilobytes
from ever crossing. Pure; scrub freely.`
}

// ===========================================================================
// Units — bandwidth, latency, size, and rate live in separate type universes.
// ===========================================================================

type (
	Megabits     float64 // a bandwidth OR a throughput, Mbit/s
	Milliseconds float64 // a round-trip time
	Kilobytes    float64 // a window or a BDP (a quantity of data)
)

func (m Milliseconds) seconds() float64 { return float64(m) / 1000.0 }
func (b Megabits) bitsPerSec() float64  { return float64(b) * 1e6 }
func (k Kilobytes) bits() float64       { return float64(k) * 1024.0 * 8.0 }

// ===========================================================================
// Types
// ===========================================================================

// Link is one network path: its bandwidth and round-trip time, in unit types.
type Link struct {
	Bandwidth Megabits
	RTT       Milliseconds
}

// bdp is the bandwidth-delay product in Kilobytes — the amount of data in flight over
// one round trip at full rate, i.e. the window needed to keep the link full.
func (l Link) bdp() Kilobytes {
	bits := l.Bandwidth.bitsPerSec() * l.RTT.seconds()
	return Kilobytes(bits / 8.0 / 1024.0)
}

// throughput is the achievable rate for a given window: min(bandwidth, window/RTT).
// The window/RTT term is the ceiling a finite window imposes — one window per round
// trip. Returns a Megabits rate; there is no way to get one except through this.
func (l Link) throughput(window Kilobytes) Megabits {
	windowLimited := window.bits() / l.RTT.seconds() / 1e6 // Mbit/s
	if windowLimited < float64(l.Bandwidth) {
		return Megabits(windowLimited)
	}
	return l.Bandwidth
}

// Result holds the BDP, the achieved throughput, and the throughput-vs-window curve.
type Result struct {
	Path      Link
	BDP       float64
	Window    int
	Achieved  float64
	Sizes     []float64
	Tput      []float64
	WindowLtd bool
}

// ===========================================================================
// Helpers
// ===========================================================================

func pct(v float64) string {
	if v < 0.01 && v > 0 {
		return strconv.FormatFloat(v*100, 'f', 2, 64) + "%"
	}
	return strconv.FormatFloat(v*100, 'f', 0, 64) + "%"
}

func humanKB(kb float64) string {
	switch {
	case kb < 1024:
		return strconv.FormatFloat(kb, 'f', 0, 64) + " KB"
	case kb < 1024*1024:
		return strconv.FormatFloat(kb/1024, 'f', 1, 64) + " MB"
	default:
		return strconv.FormatFloat(kb/1024/1024, 'f', 2, 64) + " GB"
	}
}

func humanRate(mbit float64) string {
	switch {
	case mbit < 1:
		return strconv.FormatFloat(mbit*1000, 'f', 0, 64) + " Kb/s"
	case mbit < 1000:
		return strconv.FormatFloat(mbit, 'f', 1, 64) + " Mb/s"
	default:
		return strconv.FormatFloat(mbit/1000, 'f', 1, 64) + " Gb/s"
	}
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// ===========================================================================
// Chart
// ===========================================================================

// Chart draws throughput vs window size on a log x-axis, with the BDP and the selected
// window marked.
type Chart struct{ R Result }

func (c Chart) Render() Rendered {
	r := c.R
	const w, h, pad = 720.0, 400.0, 54.0
	plotW, plotH := w-2*pad, h-2*pad
	const xlo, xhi = 16.0, 262144.0 // KB
	yhi := float64(r.Path.Bandwidth) * 1.1
	if yhi <= 0 {
		yhi = 1
	}
	lx := func(kb float64) float64 {
		return pad + (math.Log10(kb)-math.Log10(xlo))/(math.Log10(xhi)-math.Log10(xlo))*plotW
	}
	ly := func(mbit float64) float64 { return h - pad - mbit/yhi*plotH }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e2e8f0"/>`,
		pad, pad, plotW, plotH)

	// line-rate ceiling
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#cbd5e1" stroke-dasharray="5 4"/>`,
		pad, ly(float64(r.Path.Bandwidth)), w-pad, ly(float64(r.Path.Bandwidth)))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#64748b">line rate %s</text>`,
		pad+6, ly(float64(r.Path.Bandwidth))-5, humanRate(float64(r.Path.Bandwidth)))

	// x decade labels
	for _, d := range []struct {
		kb  float64
		lbl string
	}{{16, "16 KB"}, {1024, "1 MB"}, {1024 * 64, "64 MB"}, {262144, "256 MB"}} {
		if d.kb < xlo || d.kb > xhi {
			continue
		}
		x := lx(d.kb)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#f1f5f9"/>`, x, pad, x, h-pad)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#94a3b8" text-anchor="middle">%s</text>`,
			x, h-pad+16, d.lbl)
	}

	// BDP marker (the window you'd need to fill the link)
	if r.BDP >= xlo && r.BDP <= xhi {
		bx := lx(r.BDP)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#10b981" stroke-width="1.5"/>`, bx, pad, bx, h-pad)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#059669" text-anchor="middle">BDP</text>`, bx, pad-6)
	}
	// selected window marker
	if wv := float64(r.Window); wv >= xlo && wv <= xhi {
		wx := lx(wv)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-dasharray="4 4"/>`, wx, pad, wx, h-pad)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#64748b" text-anchor="middle">your window</text>`, wx, h-pad-6)
	}

	// throughput curve
	var d strings.Builder
	for i := range r.Sizes {
		verb := " L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&d, "%s%.1f %.1f", verb, lx(r.Sizes[i]), ly(r.Tput[i]))
	}
	fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#2563eb" stroke-width="2.6"/>`, d.String())

	fmt.Fprintf(&b, `<text x="%.0f" y="24" font-family="sans-serif" font-size="12" fill="#334155">throughput vs window (log) — climb is window-limited, plateau is bandwidth-limited</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#94a3b8" transform="rotate(-90 %.1f %.1f)">throughput</text>`,
		float64(16), pad+plotH/2, float64(16), pad+plotH/2)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#94a3b8">window size →</text>`, w-pad-90, h-pad+30)
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
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e2e8f0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#64748b">%s</div>`, esc(c.Label))
		fmt.Fprintf(&b, `<div style="font-size:20px;font-weight:700;color:#1e293b;margin:2px 0">%s</div>`, esc(c.Value))
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
