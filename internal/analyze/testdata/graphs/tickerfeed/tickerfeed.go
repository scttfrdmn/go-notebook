//go:notebook
//
// A live price ticker — a streaming feed into a pure notebook.
//
// A driver connects to a price stream (a public exchange WebSocket) and pushes
// each tick through the notebook's `set` port; the notebook computes, purely, the
// last price, a moving average, and the spread from the running window. The
// notebook holds no socket and no timer — the stream lives in the driver, the
// analysis lives in the cells (see docs/live-feeds.md).
//
// Because a cell is pure and stateless, the ROLLING WINDOW lives in the driver:
// it pushes a compact CSV of recent prices into the `series` leaf each tick, and
// the notebook derives the moving average and spread from that. State lives in
// the feed; math lives in the graph.
//
// Run it:
//
//	go tool notebook run ./examples/tickerfeed        # the ticker UI
//	go run ./examples/tickerfeed/driver               # the price feed (separate shell)
//
// The driver ships a SIMULATED random-walk price so it runs offline with no keys;
// one function marks where a real exchange WebSocket slots in.
//
//notebook:layout intro
//notebook:layout quote | stats
//notebook:layout tape

package tickerfeed

import (
	"fmt"
	"strconv"
	"strings"
)

// The latest trade price, in cents (integer-clean over the wire). Driven by the
// feed; the default is a plausible starting price so the notebook reads sensibly
// with no driver.
//
//notebook:slider min=1 max=20000000 step=1 area=quote
func price() (cents int) { return 6500000 } // $65,000.00

// The rolling window of recent prices (cents), pushed by the driver as a CSV
// string leaf. The notebook derives everything else from it.
//
//notebook:area=quote
func window() (series Series) { return Series{} }

// ---------------------------------------------------------------------------
// Derived — pure functions of the window.
// ---------------------------------------------------------------------------

// The moving average over the window, in cents.
func movingAvg(series Series) (avgCents int) {
	xs := series.vals()
	if len(xs) == 0 {
		return 0
	}
	sum := 0
	for _, v := range xs {
		sum += v
	}
	return sum / len(xs)
}

// The spread over the window: high − low, in cents — a cheap volatility readout.
func spread(series Series) (spreadCents int) {
	xs := series.vals()
	if len(xs) == 0 {
		return 0
	}
	lo, hi := xs[0], xs[0]
	for _, v := range xs {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return hi - lo
}

// The quote panel: last price, and its position vs the moving average.
//
//notebook:height=180 area=quote
func quote(cents, avgCents int) (view Quote) { return Quote{Cents: cents, AvgCents: avgCents} }

// A stats readout.
//
//notebook:height=180 area=stats
func stats(cents, avgCents, spreadCents int) (report Readout) {
	trend, tone := "at average", muted
	switch {
	case cents > avgCents:
		trend, tone = "above average", up
	case cents < avgCents:
		trend, tone = "below average", down
	}
	return Readout{Cards: []Card{
		{Label: "last", Value: dollars(cents)},
		{Label: "moving avg", Value: dollars(avgCents), Caption: trend, Tone: tone},
		{Label: "spread (window)", Value: dollars(spreadCents)},
	}}
}

// The tape: recent prices as a rolling line.
//
//notebook:height=200 area=tape
func tape(series Series) (chart Chart) { return Chart{S: series} }

// Orientation.
func intro() (md Markdown) {
	return `## Live price ticker

A driver streams trade prices from an exchange and pushes each tick to the
notebook's ` + "`set`" + ` port; the notebook derives the last price, a moving average,
and the spread — all pure, no socket, no timer. Run the driver
(` + "`go run ./examples/tickerfeed/driver`" + `, a simulated stream) and watch the tape
move, or set the price slider yourself.`
}

// ===========================================================================
// Types + helpers.
// ===========================================================================

const (
	muted = iota
	up
	down
)

func dollars(cents int) string {
	d := float64(cents) / 100
	s := strconv.FormatFloat(d, 'f', 2, 64)
	// thousands separators on the integer part
	dot := strings.IndexByte(s, '.')
	intp, frac := s[:dot], s[dot:]
	var g strings.Builder
	for i, c := range intp {
		if i > 0 && (len(intp)-i)%3 == 0 {
			g.WriteByte(',')
		}
		g.WriteRune(c)
	}
	return "$" + g.String() + frac
}

// Series carries the driver's rolling price window as a CSV string over the wire.
type Series struct{ CSV string }

func (s Series) Reconcile(saved any) any {
	if str, ok := saved.(string); ok {
		return Series{CSV: str}
	}
	return s
}
func (s Series) vals() []int {
	if strings.TrimSpace(s.CSV) == "" {
		return nil
	}
	var out []int
	for _, f := range strings.Split(s.CSV, ",") {
		if v, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// Quote draws the last price large with its avg beneath.
type Quote struct{ Cents, AvgCents int }

func (q Quote) Render() Rendered {
	col := "#1b3a6b"
	if q.Cents > q.AvgCents {
		col = "#0ca30c"
	} else if q.Cents < q.AvgCents {
		col = "#d03b3b"
	}
	return Rendered{MIME: "text/html", Data: fmt.Sprintf(
		`<div style="font:-apple-system,system-ui,sans-serif">`+
			`<div style="font:700 34px/1.1 system-ui;color:%s;font-variant-numeric:tabular-nums">%s</div>`+
			`<div style="color:#5b6472;font-size:13px;margin-top:.2rem">avg %s</div></div>`,
		col, dollars(q.Cents), dollars(q.AvgCents))}
}

// Chart draws the price window as a line.
type Chart struct{ S Series }

func (c Chart) Render() Rendered {
	xs := c.S.vals()
	const w, h, pad = 640.0, 180.0, 28.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f"><rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h, w, h)
	if len(xs) >= 2 {
		lo, hi := xs[0], xs[0]
		for _, v := range xs {
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
		for i, v := range xs {
			x := pad + float64(i)/float64(len(xs)-1)*(w-2*pad)
			y := h - pad - float64(v-lo)/float64(hi-lo)*(h-2*pad)
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, x, y)
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#2a78d6" stroke-width="2"/>`, d.String())
	} else {
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="13" fill="#5b6472">waiting for the feed…</text>`, pad, h/2)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="16" font-family="sans-serif" font-size="12" fill="#1b3a6b">price, rolling window</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct {
	Label, Value, Caption string
	Tone                  int
}
type Readout struct{ Cards []Card }

func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:12px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		color := "#1b3a6b"
		switch c.Tone {
		case up:
			color = "#0ca30c"
		case down:
			color = "#d03b3b"
		}
		b.WriteString(`<div style="flex:1;min-width:130px;border:1px solid #e7ebf0;border-radius:8px;padding:10px 12px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 18px/1.2 system-ui;color:%s;font-variant-numeric:tabular-nums">%s</div>`, color, c.Value)
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#5b6472">%s</div>`, c.Caption)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
