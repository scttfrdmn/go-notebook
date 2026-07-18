//go:notebook
//
// When is the cluster busy? A day × hour utilization punchcard.
//
// The classic "punchcard" (GitHub's contribution graph, Unix `history` heatmaps):
// a 7 × 24 grid, one cell per hour-of-week, shaded by how loaded the cluster is
// then. It answers a scheduling question no line chart answers as fast — *when is
// there headroom?* — because the eye reads a 2-D grid of intensity at a glance and
// a 168-point time series does not.
//
// The load model is ordinary Go: a weekday business-hours bump, a lighter
// weekend, a nightly batch window, and a tunable noise floor — each a pure cell.
// Drag the knobs and the grid re-shades live.
//
// Design deferred to HTML, and specifically to a CSS GRID. A heatmap is a grid of
// coloured boxes with hover — that is native HTML/CSS, and doing it as one SVG
// would mean hand-placing 168 rects and faking hover. So Render() emits a
// `display:grid` of `<div>`s, each tinted by its load and carrying a `title=` so
// hovering a cell shows the exact number — real browser hover, no JavaScript.
// Go owns the model; CSS owns the picture.
//
//notebook:layout intro
//notebook:layout knobs | card

package punchcard

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs — the shape of the week's load.
// ---------------------------------------------------------------------------

// Peak weekday utilization at the busiest hour (percent).
//
//notebook:slider min=10 max=100 step=1 area=knobs
func weekdayPeak() (peak int) { return 92 }

// Weekend load as a fraction of the weekday peak (percent) — how much quieter
// Saturday and Sunday are.
//
//notebook:slider min=0 max=100 step=1 area=knobs
func weekendFraction() (frac int) { return 35 }

// Nightly batch window intensity (percent) — the 1–4am ETL/backup spike that
// runs every day regardless of the business cycle.
//
//notebook:slider min=0 max=100 step=1 area=knobs
func batchIntensity() (batch int) { return 60 }

// Baseline load floor (percent) — always-on services that never idle to zero.
//
//notebook:slider min=0 max=60 step=1 area=knobs
func baseline() (floor int) { return 12 }

// ---------------------------------------------------------------------------
// The load model — a pure function of the knobs, evaluated at every hour of the
// week. Each cell of the grid is load(day, hour).
// ---------------------------------------------------------------------------

// The full 7×24 grid of utilization, computed from the model. Rows are days
// (Mon..Sun), columns hours (0..23); each value is a percent 0..100.
func grid(peak, frac, batch, floor int) (g Grid) {
	var cells [7][24]int
	for d := 0; d < 7; d++ {
		weekend := d >= 5
		for h := 0; h < 24; h++ {
			v := floor
			// Business-hours bump: a bell around 14:00, weekdays full, weekends scaled.
			business := bell(h, 14, 4) * peak / 100
			if weekend {
				business = business * frac / 100
			}
			v += business
			// Nightly batch window: a bump around 02:00, every day.
			v += bell(h, 2, 1) * batch / 100
			if v > 100 {
				v = 100
			}
			cells[d][h] = v
		}
	}
	return Grid{Cells: cells}
}

// The busiest and quietest hours of the week — the scheduling answer, stated.
func extremes(g Grid) (report Readout) {
	maxV, minV := -1, 101
	var maxD, maxH, minD, minH int
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			v := g.Cells[d][h]
			if v > maxV {
				maxV, maxD, maxH = v, d, h
			}
			if v < minV {
				minV, minD, minH = v, d, h
			}
		}
	}
	return Readout{Cards: []Card{
		{Label: "busiest hour", Value: dayName(maxD) + " " + hourLabel(maxH), Caption: itoa(maxV) + "% utilized", Bad: true},
		{Label: "most headroom", Value: dayName(minD) + " " + hourLabel(minH), Caption: itoa(minV) + "% — schedule here", Good: true},
	}}
}

// The punchcard view: the grid, shaded, as an HTML/CSS heatmap.
//
//notebook:height=340 area=card
func punchcard(g Grid) (card Heatmap) {
	return Heatmap{G: g}
}

// Orientation.
func intro() (md Markdown) {
	return `## Cluster punchcard

A 7 × 24 heatmap of utilization — one box per hour of the week, darker = busier.
It answers *when is there headroom?* faster than any line chart, because a grid of
intensity reads at a glance. Drag the knobs to reshape the week; hover a box for
the exact number. Go computes the load; the picture is a CSS grid.`
}

// ===========================================================================
// Model + formatting helpers (unnamed returns → helpers, not cells).
// ===========================================================================

// bell is a small integer "bump" peaking at center with the given half-width,
// returning 0..100. A cheap triangular kernel — no math import needed, keeps the
// cell graph obviously pure.
func bell(x, center, halfWidth int) int {
	d := x - center
	if d < 0 {
		d = -d
	}
	// wrap around the 24h clock so a bump near midnight is continuous
	if d > 12 {
		d = 24 - d
	}
	span := halfWidth * 3
	if d >= span {
		return 0
	}
	return 100 * (span - d) / span
}

func itoa(n int) string { return strconv.Itoa(n) }

func dayName(d int) string {
	return []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}[d%7]
}

func hourLabel(h int) string {
	switch {
	case h == 0:
		return "12am"
	case h < 12:
		return itoa(h) + "am"
	case h == 12:
		return "12pm"
	default:
		return itoa(h-12) + "pm"
	}
}

// ===========================================================================
// Types.
// ===========================================================================

// Grid is the 7×24 utilization matrix (percent per hour-of-week).
type Grid struct {
	Cells [7][24]int
}

// Card / Readout — the extremes panel.
type Card struct {
	Label, Value, Caption string
	Good, Bad             bool
}
type Readout struct{ Cards []Card }

func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:14px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		color := "#1b3a6b"
		if c.Good {
			color = "#0ca30c"
		} else if c.Bad {
			color = "#d03b3b"
		}
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e7ebf0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 18px/1.2 -apple-system,system-ui,sans-serif;color:%s;margin:2px 0">%s</div>`, color, c.Value)
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#5b6472">%s</div>`, c.Caption)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

// Heatmap renders the grid as a CSS-grid punchcard: 24 columns × 7 rows of tinted
// boxes, hour labels along the top, day labels down the left. Each box carries a
// title= so hovering shows the exact utilization — native browser hover, no JS.
type Heatmap struct {
	G Grid
}

func (hm Heatmap) Render() Rendered {
	var b strings.Builder
	// Outer grid: a label column + 24 hour columns; a header row + 7 day rows.
	b.WriteString(`<div style="display:grid;grid-template-columns:34px repeat(24,1fr);gap:2px;` +
		`font:11px/1 -apple-system,system-ui,sans-serif;max-width:720px">`)
	// header row: blank corner + hour ticks (label every 3h to avoid clutter)
	b.WriteString(`<div></div>`)
	for h := 0; h < 24; h++ {
		lbl := ""
		if h%3 == 0 {
			lbl = itoa(h)
		}
		fmt.Fprintf(&b, `<div style="color:#5b6472;text-align:center">%s</div>`, lbl)
	}
	// day rows
	for d := 0; d < 7; d++ {
		fmt.Fprintf(&b, `<div style="color:#5b6472;display:flex;align-items:center">%s</div>`, dayName(d))
		for h := 0; h < 24; h++ {
			v := hm.G.Cells[d][h]
			fmt.Fprintf(&b,
				`<div title="%s %s — %d%% utilized" style="aspect-ratio:1;border-radius:2px;background:%s"></div>`,
				dayName(d), hourLabel(h), v, shade(v))
		}
	}
	b.WriteString(`</div>`)
	// A tiny legend.
	b.WriteString(`<div style="display:flex;align-items:center;gap:6px;margin-top:.7rem;` +
		`font:11px -apple-system,system-ui,sans-serif;color:#5b6472">idle`)
	for _, v := range []int{5, 30, 55, 80, 100} {
		fmt.Fprintf(&b, `<div style="width:16px;height:16px;border-radius:2px;background:%s"></div>`, shade(v))
	}
	b.WriteString(`busy</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

// shade maps a 0..100 utilization to a brand-blue tint, light (idle) to dark
// (busy) — a single-hue sequential ramp, the correct encoding for magnitude.
func shade(v int) string {
	// Interpolate lightness of the brand blue #2a78d6 against white by load.
	// t in [0,1]; low load → near-white, high load → full brand blue.
	t := float64(v) / 100.0
	r := lerp(247, 42, t)
	g := lerp(250, 120, t)
	bl := lerp(252, 214, t)
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, bl)
}

// lerp is an integer linear interpolation from a to b by t in [0,1].
func lerp(a, b int, t float64) int {
	return a + int(float64(b-a)*t+0.5)
}

// Rendered / Markdown.
type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
