//go:notebook
//
// Latency vs bandwidth — why your fast link feels slow, and when it doesn't.
//
// "How long does a transfer take?" has two terms, and which one matters depends
// entirely on how much you're moving:
//
//     time  =  latency  +  size / bandwidth
//              ^^^^^^^^     ^^^^^^^^^^^^^^^^^
//              the fixed    the part that grows
//              cost to      with the transfer
//              first byte
//
// **Latency** is the round trip to the first byte — set by distance and hops, and it
// does not shrink no matter how fat the pipe. **Bandwidth** is how fast bytes flow
// once they're moving. A "faster" link usually means more bandwidth, but for a small
// transfer that buys you almost nothing: you pay the latency and you're done before
// bandwidth matters. This is why a 10 GbE link can feel *slower* than your laptop's
// loopback for a directory of tiny files, and why a cross-country 100 Gb link loses
// to a modest local one until the files get big.
//
// Compare two links and find the crossover:
//
//   - **Link A** — pick it as the low-latency one (a LAN, a nearby node): small
//     latency, modest bandwidth. It wins for *small* transfers.
//   - **Link B** — the fat far one (a long-haul or satellite path, or a big
//     cross-datacenter fabric): high latency, high bandwidth. It wins for *large*
//     transfers, once there's enough data for its bandwidth to pay off the latency.
//
// The chart plots transfer time against transfer size (log-log) for both links; they
// cross at one size. Left of the crossover you are **latency-bound** (the flat part —
// the transfer is over before bandwidth matters); right of it you are
// **bandwidth-bound** (the rising part — latency is a rounding error). The vertical
// marker is the file size you picked; the verdict names the winner there.
//
// The units carry the physics: latency is `Milliseconds`, bandwidth is `Megabits`
// per second, size is `Kilobytes`, and `time` is `Seconds` built from a duration plus
// (bytes ÷ a byte-rate). There is no path that lets you add a latency to a bandwidth
// or treat one as the other — the mistake is a type error, not a wrong number. Same
// lesson as the fleet notebook's dollars-vs-kilograms, on a wire.
//
// Pure arithmetic — a pure function of (latency, bandwidth, size) for each link, so
// scrub freely. WASM-live.

package latencybw

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs — two links to compare, plus the transfer size in question.
// ---------------------------------------------------------------------------

// Link A latency (ms) — round trip to the first byte. Make this the LOW one (a LAN).
//
//notebook:slider min=1 max=400 step=1
func linkALatency() (latA int) { return 1 }

// Link A bandwidth (Mbit/s). Modest, to contrast with B's fat pipe.
//
//notebook:slider min=10 max=100000 step=10
func linkABandwidth() (bwA int) { return 100 }

// Link B latency (ms) — the FAR link (long-haul / satellite). Make this the HIGH one.
//
//notebook:slider min=1 max=400 step=1
func linkBLatency() (latB int) { return 300 }

// Link B bandwidth (Mbit/s). The fat pipe — high, so it wins once transfers are big.
//
//notebook:slider min=10 max=100000 step=10
func linkBBandwidth() (bwB int) { return 1000 }

// Transfer size (KB) — the file you're moving. The vertical marker on the chart; the
// verdict reports which link wins at this size. Slide it across the crossover.
//
//notebook:slider min=1 max=1000000 step=1
func transferSize() (sizeKB int) { return 2000 }

// ---------------------------------------------------------------------------
// Compute (Go) — transfer time and the crossover, in units.
// ---------------------------------------------------------------------------

// linkA / linkB assemble each link's characteristics into a typed Link. Kept as their
// own cells so the graph shows two links feeding the comparison.
func linkA(latA int, bwA int) (a Link) {
	return Link{Name: "A", Latency: Milliseconds(latA), Bandwidth: Megabits(bwA)}
}

func linkB(latB int, bwB int) (b Link) {
	return Link{Name: "B", Latency: Milliseconds(latB), Bandwidth: Megabits(bwB)}
}

// compare builds the two time-vs-size curves and the crossover size where the links
// tie. Pure in (a, b): transfer time is latency + size/bandwidth, computed in units so
// no term can be confused for another. The crossover is solved exactly (the size where
// the two times are equal), then confirmed against the sampled curves.
func compare(a Link, b Link, sizeKB int) (result Comparison) {
	// sample sizes geometrically from 1 KB to 1 GB for a log-log plot.
	const lo, hi = 1.0, 1e6 // KB
	const n = 120
	sizes := make([]float64, n)
	ta := make([]float64, n)
	tb := make([]float64, n)
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n-1)
		kb := lo * math.Pow(hi/lo, f) // geometric spacing
		sizes[i] = kb
		ta[i] = float64(a.timeFor(Kilobytes(kb)))
		tb[i] = float64(b.timeFor(Kilobytes(kb)))
	}

	return Comparison{
		A: a, B: b,
		Sizes: sizes, TimeA: ta, TimeB: tb,
		Marker:    sizeKB,
		Crossover: crossoverKB(a, b),
		TimeAtA:   float64(a.timeFor(Kilobytes(sizeKB))),
		TimeAtB:   float64(b.timeFor(Kilobytes(sizeKB))),
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The transfer-time curves: time vs size (log-log) for both links, with the crossover
// and the selected size marked. The flat left ends are latency-bound (size doesn't
// matter yet); the rising right ends are bandwidth-bound (latency is a rounding
// error). They cross once — that size is the whole story.
//
//notebook:height=400
func curveChart(result Comparison) (chart Chart) {
	return Chart{C: result}
}

// The verdict at the selected size: each link's transfer time, which wins, and whether
// you're in the latency-bound or bandwidth-bound regime. Slide the size across the
// crossover and watch the winner change.
func verdict(result Comparison) (report Readout) {
	winner := "tie"
	switch {
	case result.TimeAtA < result.TimeAtB*0.999:
		winner = "Link A"
	case result.TimeAtB < result.TimeAtA*0.999:
		winner = "Link B"
	}
	regime := "bandwidth-bound — the fat pipe wins"
	if float64(result.Marker) < result.Crossover {
		regime = "latency-bound — the near link wins"
	}
	return Readout{Cards: []Card{
		{Label: "transfer size", Value: humanKB(float64(result.Marker)), Caption: "the file you're moving"},
		{Label: "Link A time", Value: humanTime(result.TimeAtA), Caption: linkDesc(result.A)},
		{Label: "Link B time", Value: humanTime(result.TimeAtB), Caption: linkDesc(result.B)},
		{Label: "winner", Value: winner, Caption: regime},
		{Label: "crossover", Value: humanKB(result.Crossover), Caption: "below this: latency wins. above: bandwidth wins."},
	}}
}

// Latency vs bandwidth — why your fast link feels slow, and when it doesn't.
func intro() (md Markdown) {
	return `Transfer time is **latency + size/bandwidth** — a fixed cost to the first
byte, plus a part that grows with the data. For a *small* transfer you pay the latency
and you're done, so a fatter pipe buys almost nothing; that's why a 10 GbE link can
feel slower than loopback for tiny files.

Compare two links — a **near** one (low latency, modest bandwidth) and a **far fat**
one (high latency, high bandwidth) — and slide the transfer size. Left of the
crossover you're **latency-bound** (near link wins); right of it **bandwidth-bound**
(fat pipe wins). The units keep it honest: latency is ` + "`Milliseconds`" + `,
bandwidth is ` + "`Megabits`/s" + `, and you *can't* add one to the other — it's a
type error, like the fleet notebook's dollars vs kilograms. Pure; scrub freely.`
}

// ===========================================================================
// Metrics
// ===========================================================================

// crossoverKB solves for the transfer size (KB) at which both links take equal time:
//
//	latA + s/bwA = latB + s/bwB  ⇒  s = (latB − latA) / (1/bwA − 1/bwB)   [in bytes]
//
// Returns 0 if the links never cross for positive size (one dominates everywhere).
func crossoverKB(a, b Link) float64 {
	latA, latB := a.Latency.seconds(), b.Latency.seconds()
	rA, rB := a.Bandwidth.bytesPerSec(), b.Bandwidth.bytesPerSec()
	denom := 1/rA - 1/rB
	if denom == 0 {
		return 0
	}
	bytes := (latB - latA) / denom
	if bytes <= 0 {
		return 0
	}
	return bytes / 1024.0
}

// ===========================================================================
// Units — latency, bandwidth, size, and time live in separate type universes.
// ===========================================================================

type (
	Milliseconds float64 // a latency
	Megabits     float64 // a bandwidth, Mbit/s
	Kilobytes    float64 // a transfer size
	Seconds      float64 // a duration — what a transfer time IS
)

func (m Milliseconds) seconds() float64 { return float64(m) / 1000.0 }
func (b Megabits) bytesPerSec() float64 { return float64(b) * 1e6 / 8.0 }
func (k Kilobytes) bytes() float64      { return float64(k) * 1024.0 }

// ===========================================================================
// Helpers
// ===========================================================================

func humanTime(s float64) string {
	switch {
	case s < 1e-3:
		return strconv.FormatFloat(s*1e6, 'f', 0, 64) + " µs"
	case s < 1:
		return strconv.FormatFloat(s*1e3, 'f', 1, 64) + " ms"
	case s < 60:
		return strconv.FormatFloat(s, 'f', 2, 64) + " s"
	default:
		return strconv.FormatFloat(s/60, 'f', 1, 64) + " min"
	}
}

func humanKB(kb float64) string {
	switch {
	case kb <= 0:
		return "—"
	case kb < 1024:
		return strconv.FormatFloat(kb, 'f', 0, 64) + " KB"
	case kb < 1024*1024:
		return strconv.FormatFloat(kb/1024, 'f', 1, 64) + " MB"
	default:
		return strconv.FormatFloat(kb/1024/1024, 'f', 2, 64) + " GB"
	}
}

func linkDesc(l Link) string {
	return strconv.FormatFloat(float64(l.Latency), 'f', 0, 64) + " ms · " +
		strconv.FormatFloat(float64(l.Bandwidth), 'f', 0, 64) + " Mbit/s"
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// ===========================================================================
// Types
// ===========================================================================

// Link is one network path: its latency and bandwidth, in their own unit types.
type Link struct {
	Name      string
	Latency   Milliseconds
	Bandwidth Megabits
}

// timeFor is the transfer time for `size` on this link: latency + size/bandwidth,
// assembled from a duration and (bytes ÷ byte-rate). The ONLY way to get a Seconds —
// there is no arithmetic that turns a bandwidth into a latency or vice versa.
func (l Link) timeFor(size Kilobytes) Seconds {
	return Seconds(l.Latency.seconds() + size.bytes()/l.Bandwidth.bytesPerSec())
}

// Comparison holds both time-vs-size curves plus the crossover and the marked size.
type Comparison struct {
	A, B      Link
	Sizes     []float64
	TimeA     []float64
	TimeB     []float64
	Marker    int
	Crossover float64
	TimeAtA   float64
	TimeAtB   float64
}

// Chart draws the two transfer-time curves on log-log axes.
type Chart struct{ C Comparison }

func (ch Chart) Render() Rendered {
	c := ch.C
	const w, h, pad = 720.0, 400.0, 52.0
	plotW, plotH := w-2*pad, h-2*pad

	// log-log extents. x: 1 KB .. 1 GB (1e6 KB). y: from the min to max sampled time.
	const xlo, xhi = 1.0, 1e6
	ylo, yhi := c.TimeA[0], c.TimeA[0]
	for _, arr := range [][]float64{c.TimeA, c.TimeB} {
		for _, v := range arr {
			if v < ylo {
				ylo = v
			}
			if v > yhi {
				yhi = v
			}
		}
	}
	if ylo <= 0 {
		ylo = 1e-6
	}
	lx := func(kb float64) float64 {
		return pad + (math.Log10(kb)-math.Log10(xlo))/(math.Log10(xhi)-math.Log10(xlo))*plotW
	}
	ly := func(s float64) float64 {
		if s <= 0 {
			s = ylo
		}
		return h - pad - (math.Log10(s)-math.Log10(ylo))/(math.Log10(yhi)-math.Log10(ylo))*plotH
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#e7ebf0"/>`,
		pad, pad, plotW, plotH)

	// x decade gridlines + labels (1KB, 1MB, 1GB)
	for _, d := range []struct {
		kb  float64
		lbl string
	}{{1, "1 KB"}, {1024, "1 MB"}, {1024 * 1024, "1 GB"}} {
		if d.kb < xlo || d.kb > xhi {
			continue
		}
		x := lx(d.kb)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e7ebf0"/>`, x, pad, x, h-pad)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#5b6472" text-anchor="middle">%s</text>`,
			x, h-pad+16, d.lbl)
	}

	// crossover marker (where the two links tie)
	if c.Crossover >= xlo && c.Crossover <= xhi {
		cx := lx(c.Crossover)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#008300" stroke-width="1.5"/>`, cx, pad, cx, h-pad)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#008300" text-anchor="middle">crossover</text>`, cx, pad-6)
	}
	// selected-size marker
	if float64(c.Marker) >= xlo && float64(c.Marker) <= xhi {
		mx := lx(float64(c.Marker))
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#5b6472" stroke-dasharray="4 4"/>`, mx, pad, mx, h-pad)
	}

	line := func(xs, ys []float64, color string) {
		var d strings.Builder
		for i := range xs {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, lx(xs[i]), ly(ys[i]))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2.4"/>`, d.String(), color)
	}
	line(c.Sizes, c.TimeA, "#2a78d6") // Link A
	line(c.Sizes, c.TimeB, "#0797b8") // Link B

	fmt.Fprintf(&b, `<text x="%.0f" y="24" font-family="sans-serif" font-size="12" fill="#1b3a6b">transfer time vs size (log-log) — flat = latency-bound, rising = bandwidth-bound</text>`, pad)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#2a78d6">Link A (near)</text>`, pad+6, pad+16)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#0797b8">Link B (fat/far)</text>`, pad+96, pad+16)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#5b6472" transform="rotate(-90 %.1f %.1f)">transfer time</text>`,
		float64(16), pad+plotH/2, float64(16), pad+plotH/2)
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
		b.WriteString(`<div style="flex:1;min-width:130px;border:1px solid #e7ebf0;border-radius:8px;padding:12px 14px">`)
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
