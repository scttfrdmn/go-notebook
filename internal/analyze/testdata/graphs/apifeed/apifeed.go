//go:notebook
//
// Where is the ISS right now? A polled public API as a live feed.
//
// A driver polls a keyless public JSON API (the International Space Station's
// live position) on an interval and pushes the latest reading to the notebook's
// `set` port; the notebook plots the position on a world map and reports it —
// purely. The notebook makes no HTTP call and has no timer; the poll lives in the
// driver (see docs/live-feeds.md). This is the "poll a REST feed" shape, the most
// common live-data pattern of all.
//
// Run it:
//
//	go tool notebook run ./examples/apifeed            # the tracker UI
//	go run ./examples/apifeed/driver                   # the poller (separate shell)
//
// The driver hits a real public endpoint (no key); if it's unreachable it falls
// back to a simulated orbit so the example still runs offline. One function marks
// the API seam.
//
//notebook:layout intro
//notebook:layout map | readout

package apifeed

import (
	"fmt"
	"strconv"
	"strings"
)

// Latitude ×1000 (integer-clean over the wire; 37421 = 37.421°). Driven by the poll.
//
//notebook:slider min=-90000 max=90000 step=1 area=readout
func lat() (latMilli int) { return 0 }

// Longitude ×1000.
//
//notebook:slider min=-180000 max=180000 step=1 area=readout
func lon() (lonMilli int) { return 0 }

// Poll count — the driver increments it each fetch, so the readout shows liveness.
//
//notebook:slider min=0 max=1000000 step=1 area=readout
func polls() (n int) { return 0 }

// ---------------------------------------------------------------------------
// Derived — pure functions of the position.
// ---------------------------------------------------------------------------

// The world map with the ISS plotted. A pure function of lat/lon.
//
//notebook:height=320 area=map
func worldmap(latMilli, lonMilli int) (view Map) {
	return Map{Lat: float64(latMilli) / 1000, Lon: float64(lonMilli) / 1000}
}

// A readout of the current position + a rough "over ocean or land" hint derived
// from the coordinates (a coarse heuristic — the point is the live feed, not
// cartography).
//
//notebook:height=200 area=readout
func readout(latMilli, lonMilli, n int) (report Readout) {
	la, lo := float64(latMilli)/1000, float64(lonMilli)/1000
	return Readout{Cards: []Card{
		{Label: "latitude", Value: coord(la, "N", "S")},
		{Label: "longitude", Value: coord(lo, "E", "W")},
		{Label: "hemisphere", Value: hemi(la, lo)},
		{Label: "polls", Value: itoa(n), Caption: "live"},
	}}
}

// Orientation.
func intro() (md Markdown) {
	return `## ISS tracker

A driver polls a public API for the International Space Station's live position
every few seconds and pushes it to the notebook's ` + "`set`" + ` port; the notebook
plots it on a world map — purely, no HTTP, no timer. Run the poller
(` + "`go run ./examples/apifeed/driver`" + `) and watch it move (~7.7 km/s, so it
travels visibly between polls), or set the coordinate sliders yourself.`
}

// ===========================================================================
// Types + helpers.
// ===========================================================================

func itoa(n int) string { return strconv.Itoa(n) }

func coord(v float64, pos, neg string) string {
	d := pos
	if v < 0 {
		d, v = neg, -v
	}
	return strconv.FormatFloat(v, 'f', 3, 64) + "° " + d
}

func hemi(la, lo float64) string {
	ns, ew := "N", "E"
	if la < 0 {
		ns = "S"
	}
	if lo < 0 {
		ew = "W"
	}
	return ns + ew
}

// Map draws an equirectangular world outline with the ISS marked. The outline is
// a coarse graticule (a real map would embed a GeoJSON path); the marker is the
// live part.
type Map struct{ Lat, Lon float64 }

func (m Map) Render() Rendered {
	const w, h = 640.0, 320.0
	// Equirectangular projection: lon -180..180 → 0..w, lat 90..-90 → 0..h.
	x := (m.Lon + 180) / 360 * w
	y := (90 - m.Lat) / 180 * h
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" style="max-width:%.0fpx">`, w, h, w)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#eef4fb"/>`, w, h)
	// graticule every 30°
	for lonDeg := -180.0; lonDeg <= 180; lonDeg += 30 {
		gx := (lonDeg + 180) / 360 * w
		fmt.Fprintf(&b, `<line x1="%.1f" y1="0" x2="%.1f" y2="%.0f" stroke="#d7e6fb" stroke-width="1"/>`, gx, gx, h)
	}
	for latDeg := -60.0; latDeg <= 60; latDeg += 30 {
		gy := (90 - latDeg) / 180 * h
		fmt.Fprintf(&b, `<line x1="0" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#d7e6fb" stroke-width="1"/>`, gy, w, gy)
	}
	// equator + prime meridian, stronger
	fmt.Fprintf(&b, `<line x1="0" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#b7cdEb" stroke-width="1.5"/>`, h/2, w, h/2)
	fmt.Fprintf(&b, `<line x1="%.0f" y1="0" x2="%.0f" y2="%.0f" stroke="#b7cdeb" stroke-width="1.5"/>`, w/2, w/2, h)
	// the ISS marker
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#d03b3b" stroke="#fff" stroke-width="2"/>`, x, y)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" font-weight="700" fill="#1b3a6b">ISS</text>`, x+10, y+4)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct {
	Label, Value, Caption string
}
type Readout struct{ Cards []Card }

func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:12px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		b.WriteString(`<div style="flex:1;min-width:120px;border:1px solid #e7ebf0;border-radius:8px;padding:10px 12px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 18px/1.2 -apple-system,system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">%s</div>`, c.Value)
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#0ca30c">%s</div>`, c.Caption)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
