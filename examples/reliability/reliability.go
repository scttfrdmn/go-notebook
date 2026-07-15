//go:notebook
//
// Nines, in series and in parallel.
//
// A request flows through a chain: a load balancer, then an application tier, then a
// database. The system is up only if every stage is up — availabilities in series
// *multiply*, so the whole is always worse than its worst part. That is the first
// thing this shows, and it surprises people: three "three-nines" components (99.9%
// each) compose to 99.7%, nearly a full day of downtime a year, because 0.999³ < 0.999.
//
// The second thing is the fix, and it is nonlinear: put N app servers in parallel and
// the tier fails only if *all* of them fail, so its unavailability is raised to the
// Nth power. One extra replica of a three-nines app server takes that tier to six
// nines — and the system's downtime drops off a cliff. Drag `replicas` from 1 to 2
// and watch the nines jump.
//
// Availability is set in **nines** — the slider reads directly as the SRE unit
// (3.0 nines = 99.9%), because a = 1 − 10^(−nines) is exactly what "number of nines"
// means. Downtime per year is the number people actually feel.
//
// What it puts on stage: **the dependency graph as the subject.** Every notebook here
// has a derived graph; this one's *content* is also a graph — a reliability block
// diagram — so the picture the engine draws at the top (cells feeding cells) and the
// picture in the cell body (components composing into a system) are the same shape,
// one level apart. And it is a real SRE tool: pure arithmetic over availabilities,
// no fold, so every slider scrubs exactly.

package reliability

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs — availabilities in tenths-of-a-nine (30 = 3.0 nines = 99.9%).
// ---------------------------------------------------------------------------

// Load balancer availability, in nines×10 (30 → 3.0 nines → 99.9%). The front door;
// it's in series with everything, so its nines cap the whole system.
//
//notebook:slider min=10 max=50 step=1
func lbNines() (lb int) { return 35 }

// Per-app-server availability, in nines×10. This is the tier you make redundant —
// one replica here is the weak link, but parallel copies fix it fast.
//
//notebook:slider min=10 max=50 step=1
func appNines() (app int) { return 25 }

// Database availability, in nines×10. Usually the hardest to make redundant, so
// often the real floor on the system.
//
//notebook:slider min=10 max=50 step=1
func dbNines() (db int) { return 35 }

// Number of app-server replicas in parallel. The tier is up if ANY replica is up, so
// its unavailability is raised to this power — drag 1 → 2 and watch the nines jump.
//
//notebook:slider min=1 max=5 step=1
func replicas() (n int) { return 1 }

// ---------------------------------------------------------------------------
// Compute (Go) — availability composition, pure.
// ---------------------------------------------------------------------------

// The composed system: the app tier's availability after N-way redundancy, and the
// series product of load balancer × app tier × database. Pure arithmetic over the
// four sliders — no fold — so scrubbing is exact both ways.
func system(lb int, app int, db int, n int) (sys System) {
	aLB := fromNines(lb)
	aApp := fromNines(app)
	aDB := fromNines(db)

	// Redundant app tier: up unless all N replicas are down.
	appTier := 1 - math.Pow(1-aApp, float64(n))
	// Series: the request must clear every stage.
	total := aLB * appTier * aDB

	return System{
		LB:      Component{Name: "load balancer", A: aLB},
		App:     Component{Name: "app tier", A: appTier, Replicas: n, Each: aApp},
		DB:      Component{Name: "database", A: aDB},
		Total:   total,
		AppEach: aApp,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The reliability block diagram: the three stages in series, left to right, with the
// app tier drawn as N stacked parallel boxes. Each block shows its availability; the
// system total sits at the end. This is the notebook's content — a graph — beneath
// the engine's derived graph of the same shape.
//
//notebook:height=320
func diagram(sys System) (block Diagram) {
	return Diagram{Sys: sys}
}

// The numbers people actually feel: the system's nines and its downtime per year,
// and the same for each stage — so you can see which stage is the floor.
func budget(sys System) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "system availability", Value: ninesOf(sys.Total), Caption: downtime(sys.Total) + " down / year"},
		{Label: "load balancer", Value: ninesOf(sys.LB.A), Caption: downtime(sys.LB.A) + " / year"},
		{Label: "app tier", Value: ninesOf(sys.App.A), Caption: replicaNote(sys) + downtime(sys.App.A) + " / year"},
		{Label: "database", Value: ninesOf(sys.DB.A), Caption: downtime(sys.DB.A) + " / year"},
	}}
}

// Nines, in series and in parallel.
func intro() (md Markdown) {
	return `A request passes through a load balancer, an app tier, and a database. The
system is up only if **all three** are — availabilities in series *multiply*, so the
whole is always worse than its worst part. Three three-nines stages (99.9% each)
compose to about 99.7%: nearly a day of downtime a year.

The fix is redundancy, and it's nonlinear. Put N app servers in **parallel** and the
tier fails only if *every* replica does, so its unavailability is raised to the Nth
power. Drag **replicas** from 1 to 2 and watch the app tier — and the whole system —
jump several nines. Set the availabilities in nines (3.0 = 99.9%); the readout shows
the downtime-per-year you'd actually feel. Pure arithmetic, so scrub freely.`
}

// ===========================================================================
// Availability helpers
// ===========================================================================

// fromNines converts a nines×10 slider value to an availability fraction:
// a = 1 − 10^(−nines). 30 → 3.0 nines → 0.999.
func fromNines(ninesTenths int) float64 {
	return 1 - math.Pow(10, -float64(ninesTenths)/10)
}

// ninesOf renders an availability as "N.N nines (99.9%)".
func ninesOf(a float64) string {
	if a >= 1 {
		return "∞ nines (100%)"
	}
	nines := -math.Log10(1 - a)
	return strconv.FormatFloat(nines, 'f', 1, 64) + " nines (" + pctString(a) + ")"
}

// pctString renders an availability as a percentage with enough decimals to show the
// nines (99.9%, 99.99%, …).
func pctString(a float64) string {
	p := a * 100
	// choose decimals so the nines are visible
	dec := 1
	if p > 99.9 {
		dec = 3
	}
	if p > 99.99 {
		dec = 5
	}
	return strconv.FormatFloat(p, 'f', dec, 64) + "%"
}

// downtime renders unavailability as a human duration per year.
func downtime(a float64) string {
	mins := (1 - a) * 365.25 * 24 * 60
	switch {
	case mins >= 24*60:
		return f1(mins/(24*60)) + " days"
	case mins >= 60:
		return f1(mins/60) + " hours"
	case mins >= 1:
		return f1(mins) + " min"
	default:
		return f1(mins*60) + " sec"
	}
}

func replicaNote(sys System) string {
	if sys.App.Replicas > 1 {
		return strconv.Itoa(sys.App.Replicas) + "× redundant · "
	}
	return ""
}

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// ===========================================================================
// Types
// ===========================================================================

// Component is one stage: its name, composed availability, and (for the app tier) the
// replica count and per-replica availability.
type Component struct {
	Name     string
	A        float64
	Replicas int
	Each     float64
}

// System is the composed architecture.
type System struct {
	LB, App, DB Component
	Total       float64
	AppEach     float64
}

// Diagram renders the reliability block diagram.
type Diagram struct{ Sys System }

func (d Diagram) Render() Rendered {
	const w, h = 820.0, 320.0
	s := d.Sys

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	// A helper to draw a labeled block with an availability caption.
	block := func(cx, cy, bw, bh float64, title, sub, fill string) {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="8" fill=%q stroke="#1b3a6b" stroke-width="1.5"/>`,
			cx-bw/2, cy-bh/2, bw, bh, fill)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="13" font-weight="600" fill="#1b3a6b" text-anchor="middle">%s</text>`,
			cx, cy-2, title)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#5b6472" text-anchor="middle">%s</text>`,
			cx, cy+14, sub)
	}
	arrow := func(x0, x1, y float64) {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-width="2"/>`, x0, y, x1, y)
		fmt.Fprintf(&b, `<path d="M%.1f %.1f l-7 -4 l0 8 z" fill="#94a3b8"/>`, x1, y)
	}

	midY := 130.0
	lbX, appX, dbX := 110.0, 400.0, 690.0

	// load balancer
	block(lbX, midY, 150, 54, "load balancer", pctString(s.LB.A), "#eef4ff")
	arrow(lbX+82, appX-95, midY)

	// app tier — N stacked replica boxes, bracketed
	n := s.App.Replicas
	repW, repH, gap := 150.0, 34.0, 10.0
	totalH := float64(n)*repH + float64(n-1)*gap
	top := midY - totalH/2
	for i := 0; i < n; i++ {
		cy := top + float64(i)*(repH+gap) + repH/2
		block(appX, cy, repW, repH, "app server", pctString(s.AppEach), "#eafaf1")
	}
	// tier label under the stack
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#237a2b" text-anchor="middle">app tier — %s (%s)</text>`,
		appX, top+totalH+22, redundancyLabel(n), pctString(s.App.A))
	arrow(appX+82, dbX-77, midY)

	// database
	block(dbX, midY, 130, 54, "database", pctString(s.DB.A), "#eef4ff")

	// system total, prominent, at the bottom
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="15" font-weight="700" fill="#1b3a6b" text-anchor="middle">system: %s</text>`,
		w/2, h-24, ninesOf(s.Total))
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

func redundancyLabel(n int) string {
	if n <= 1 {
		return "single (no redundancy)"
	}
	return strconv.Itoa(n) + "× parallel"
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
