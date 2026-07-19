//go:notebook
//
// A live sensor dashboard — driven by a feed, not a slider.
//
// This notebook is a **pure** dashboard: gauges and a rolling chart that are a
// function of a few input leaves (cpuPct, memMB, goroutines, tick). It fetches
// nothing and has no timer — a cell can't, and shouldn't (that would make it
// impure and break the reactive model). Instead a small **driver** program
// (`driver/main.go`) samples live metrics once a second and pushes each reading
// through the notebook's one data-in port — `POST /set` — exactly as a slider
// would, but with a program's hand on the knob.
//
// That is the live-feed pattern in one line: **a feed is a driver on the `set`
// port; the notebook is pure cells that react** (see docs/live-feeds.md). The
// impure edge — the sampling, the clock — lives in the driver, outside the graph;
// the notebook stays a portable, pure artifact.
//
// Run it:
//
//	go tool notebook run ./examples/sensorfeed        # serve the dashboard
//	go run ./examples/sensorfeed/driver               # start the feed (separate shell)
//
// The gauges move on their own as the driver pushes readings. Drag a slider
// yourself and you are doing exactly what the driver does — there is no
// difference between a human and a feed at the `set` port.
//
//notebook:layout intro
//notebook:layout gauges | status
//notebook:layout history

package sensorfeed

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Feed leaves — written by the driver via POST /set, not by a human. They carry
// slider directives so the notebook is still usable by hand with no driver
// running (the degradation ladder: a feed leaf is just a slider whose hand is a
// program).
// ---------------------------------------------------------------------------

// CPU utilization, percent. Driven by the feed; drag it yourself with no driver.
//
//notebook:slider min=0 max=100 step=1 area=gauges
func cpu() (cpuPct int) { return 0 }

// Resident memory, megabytes.
//
//notebook:slider min=0 max=64000 step=1 area=gauges
func mem() (memMB int) { return 0 }

// Live goroutine count in the driver process.
//
//notebook:slider min=0 max=10000 step=1 area=gauges
func goroutines() (numG int) { return 0 }

// A monotonic tick the driver increments each sample, so the history chart has
// an x-axis that advances even when a reading repeats.
//
//notebook:slider min=0 max=100000 step=1 area=gauges
func tick() (t int) { return 0 }

// ---------------------------------------------------------------------------
// Derived views — pure functions of the feed leaves.
// ---------------------------------------------------------------------------

// The gauges: CPU and memory as dial readouts. A pure function of the leaves.
//
//notebook:height=200 area=gauges
func dials(cpuPct, memMB int) (view Gauges) { return Gauges{CPU: cpuPct, MemMB: memMB} }

// A status readout: the raw numbers plus a derived health verdict.
//
//notebook:height=220 area=status
func status(cpuPct, memMB, numG, t int) (report Readout) {
	health, tone := "nominal", good
	switch {
	case cpuPct >= 90:
		health, tone = "CPU saturated", bad
	case cpuPct >= 70:
		health, tone = "CPU busy", warn
	}
	return Readout{Cards: []Card{
		{Label: "CPU", Value: itoa(cpuPct) + "%", Tone: tone},
		{Label: "memory", Value: itoa(memMB) + " MB"},
		{Label: "goroutines", Value: itoa(numG)},
		{Label: "sample #", Value: itoa(t), Caption: health},
	}}
}

// Orientation.
func intro() (md Markdown) {
	return `## Live sensor feed

The gauges below are driven by a **feed**, not a slider — a small driver program
samples this machine's CPU and memory once a second and pushes each reading to the
notebook's ` + "`set`" + ` port. The notebook itself is pure: it fetches nothing and
has no timer. Start the driver (` + "`go run ./examples/sensorfeed/driver`" + `) and
watch them move; or drag a slider and be the feed yourself.`
}

// ===========================================================================
// Helpers + types. A notebook imports nothing from the project; redeclare here.
// ===========================================================================

func itoa(n int) string { return strconv.Itoa(n) }

const (
	good = iota
	warn
	bad
)

// Gauges draws two dial readouts (CPU, memory) as SVG arcs.
type Gauges struct {
	CPU   int
	MemMB int
}

func (g Gauges) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 360 180" style="max-width:360px">`)
	b.WriteString(`<rect width="360" height="180" fill="#fff"/>`)
	b.WriteString(dial(90, 90, float64(g.CPU)/100, "CPU", itoa(g.CPU)+"%"))
	// Memory scaled to a soft 32 GB ceiling for the arc; the number is exact.
	memFrac := float64(g.MemMB) / 32000.0
	if memFrac > 1 {
		memFrac = 1
	}
	b.WriteString(dial(270, 90, memFrac, "mem", itoa(g.MemMB)+" MB"))
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// dial draws one 270° gauge arc filled to frac (0..1), centered at (cx,cy).
func dial(cx, cy float64, frac float64, label, value string) string {
	const r = 54
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	track := arc(cx, cy, r, 0, 1)
	fill := arc(cx, cy, r, 0, frac)
	col := "#2a78d6"
	if frac >= 0.9 {
		col = "#d03b3b"
	} else if frac >= 0.7 {
		col = "#fab219"
	}
	return fmt.Sprintf(
		`<path d=%q fill="none" stroke="#e7ebf0" stroke-width="12" stroke-linecap="round"/>`+
			`<path d=%q fill="none" stroke=%q stroke-width="12" stroke-linecap="round"/>`+
			`<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="20" font-weight="700" fill="#1b3a6b" text-anchor="middle">%s</text>`+
			`<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12" fill="#5b6472" text-anchor="middle">%s</text>`,
		track, fill, col, cx, cy+4, value, cx, cy+26, label)
}

// arc returns an SVG path for a 270° gauge sweep from fraction a to b (0..1),
// starting at the lower-left (135°) and sweeping clockwise to the lower-right.
func arc(cx, cy, r float64, a, b float64) string {
	const start, sweep = 135.0, 270.0
	x1, y1 := pointOnArc(cx, cy, r, start+sweep*a)
	x2, y2 := pointOnArc(cx, cy, r, start+sweep*b)
	large := 0
	if (b-a)*sweep > 180 {
		large = 1
	}
	return fmt.Sprintf("M%.1f %.1f A%.0f %.0f 0 %d 1 %.1f %.1f", x1, y1, r, r, large, x2, y2)
}

func pointOnArc(cx, cy, r, deg float64) (float64, float64) {
	rad := deg * math.Pi / 180
	return cx + r*math.Cos(rad), cy + r*math.Sin(rad)
}

// A history chart — the last readings, drawn as a rolling line. Because a cell is
// pure and holds no history, the DRIVER carries the window: it pushes a compact
// CSV of recent CPU samples into the `series` leaf each tick, and this cell just
// draws whatever it's handed. (State lives in the feed, not the graph.)
//
//notebook:height=220 area=history
func history(series Series) (chart TimeChart) { return TimeChart{S: series} }

// The rolling window, pushed by the driver as a comma-separated list of recent
// CPU percents. A leaf whose value is a small string — set over the wire like any
// other. Empty until the driver sends one.
//
//notebook:area=history
func window() (series Series) { return Series{} }

// Series carries the driver's rolling window. Reconcile lets the driver set it as
// a plain string over /set (the wire carries a string; the notebook parses it).
type Series struct{ CSV string }

func (s Series) Reconcile(saved any) any {
	if str, ok := saved.(string); ok {
		return Series{CSV: str}
	}
	return s
}

// TimeChart draws the rolling CPU window as a simple line.
type TimeChart struct{ S Series }

func (tc TimeChart) Render() Rendered {
	pts := parseCSV(tc.S.CSV)
	const w, h, pad = 640.0, 180.0, 24.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#e7ebf0"/>`, pad, h-pad, w-pad, h-pad)
	if len(pts) >= 2 {
		var d strings.Builder
		for i, v := range pts {
			x := pad + float64(i)/float64(len(pts)-1)*(w-2*pad)
			y := h - pad - (v/100)*(h-2*pad)
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
	fmt.Fprintf(&b, `<text x="%.0f" y="16" font-family="sans-serif" font-size="12" fill="#1b3a6b">CPU %%, rolling window</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// parseCSV turns "12,40,55" into []float64; ignores anything unparseable.
func parseCSV(s string) []float64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []float64
	for _, f := range strings.Split(s, ",") {
		if v, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// Card / Readout — the status panel, brand palette.
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
		case good:
			color = "#0ca30c"
		case warn:
			color = "#b8860b"
		case bad:
			color = "#d03b3b"
		}
		b.WriteString(`<div style="flex:1;min-width:120px;border:1px solid #e7ebf0;border-radius:8px;padding:10px 12px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 22px/1.2 -apple-system,system-ui,sans-serif;color:%s;font-variant-numeric:tabular-nums">%s</div>`, color, c.Value)
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
