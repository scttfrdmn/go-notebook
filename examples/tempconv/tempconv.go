//go:notebook
//
// A live thermometer — the notebook built from scratch in docs/authoring.md.
//
// Two input-and-derived cells (Celsius in, Fahrenheit out) plus a gauge view.
// The edge celsius → fahrenheit exists because celsius produces a result named
// `c` and fahrenheit takes a parameter named `c`: a cell's named result feeds any
// cell that takes a parameter of the same name and type. Nothing wires it by
// hand — the graph is derived from the signatures.
//
//	go tool notebook run ./examples/tempconv      # drag the slider in a browser
//
//notebook:layout celsius | gauge

package tempconv

import (
	"fmt"
	"strconv"
	"strings"
)

// Temperature in Celsius.
//
//notebook:slider min=-40 max=120 step=1 area=celsius
func celsius() (c int) { return 20 }

// Converted to Fahrenheit — wired in by the parameter name `c`.
func fahrenheit(c int) (f int) { return c*9/5 + 32 }

// A thermometer drawn from the value.
//
//notebook:height=220 area=gauge
func gauge(c, f int) (view Thermo) { return Thermo{C: c, F: f} }

// The reading as plain text, beside the gauge.
//
//notebook:area=celsius
func reading(c, f int) (label Text) {
	return Text(strconv.Itoa(c) + " °C  =  " + strconv.Itoa(f) + " °F")
}

// ===========================================================================
// Display types. A notebook imports nothing from this project; the tiny display
// types are declared here. fmt lives in Render (the engine calls it), never in a
// cell body — that keeps the WASM portability gate clear.
// ===========================================================================

// Thermo renders a simple SVG thermometer.
type Thermo struct{ C, F int }

func (t Thermo) Render() Rendered {
	frac := float64(t.C+40) / 160.0
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fill := frac * 150 // pixels of mercury within the 150px column
	col := "#2a78d6"
	if t.C >= 30 {
		col = "#d0433b"
	} else if t.C <= 0 {
		col = "#00add8"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 260 220">`)
	fmt.Fprintf(&b, `<rect width="260" height="220" fill="#fff"/>`)
	// track + mercury (drawn from the bottom up)
	fmt.Fprintf(&b, `<rect x="24" y="20" width="22" height="150" rx="11" fill="#e7ebf0"/>`)
	fmt.Fprintf(&b, `<rect x="24" y="%.0f" width="22" height="%.0f" rx="11" fill="%s"/>`, 20+150-fill, fill, col)
	fmt.Fprintf(&b, `<circle cx="35" cy="185" r="20" fill="%s"/>`, col)
	// readouts
	fmt.Fprintf(&b, `<text x="72" y="90" font-family="sans-serif" font-size="34" font-weight="700" fill="#1b3a6b">%d°C</text>`, t.C)
	fmt.Fprintf(&b, `<text x="72" y="128" font-family="sans-serif" font-size="24" fill="#5b6472">%d°F</text>`, t.F)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// Text is a plain string readout.
type Text string

func (t Text) Render() Rendered { return Rendered{MIME: "text/plain", Data: string(t)} }

// Rendered is the tiny display envelope every notebook redeclares.
type Rendered struct{ MIME, Data string }
