//go:notebook
//
// Turing patterns from two numbers.
//
// Alan Turing's 1952 idea: two chemicals, one that activates and one that inhibits,
// diffusing at different rates, will spontaneously organize a smooth soup into
// spots, stripes, and mazes — the same math behind leopard spots and zebra stripes.
// This is the Gray-Scott reaction-diffusion system, and the entire zoo of patterns
// is controlled by exactly two numbers: a feed rate and a kill rate.
//
// Drag the two sliders and scrub the step count to watch a pattern develop. Small
// moves in feed/kill cross sharp boundaries between regimes — solid, spots,
// stripes, mazes, coral, chaos — which is why the parameter plane is worth
// exploring by hand rather than reading off a chart.
//
// Two things this notebook is built to show:
//
//   - **No fold.** The grid is simulated to a fixed horizon INSIDE one cell — a
//     pure function of (feed, kill, steps). Scrub the step slider down and it
//     re-runs from the seeded soup, exactly; there is no accumulating state to
//     rewind, the same reason bayes can scrub backward.
//
//   - **The honest WASM caveat, as the exhibit.** Each step updates every cell of
//     the grid from its neighbours — embarrassingly parallel, the textbook case
//     for the scheduler's goroutine fan-out. Built natively, that fan-out is real
//     and the sweep is fast. Built to WASM for this page, GOOS=js is single-
//     threaded, so the steps run serially and a big grid repaints slowly. Same
//     file, same code; the parallel dividend is present on a cluster and absent in
//     the tab. The project says this caveat out loud instead of hiding it, and
//     here you can feel it: turn the step count up and watch the browser work.

package turing

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strconv"
)

const grid = 96 // grid is grid×grid; kept modest so the WASM (serial) build repaints.

// ---------------------------------------------------------------------------
// Inputs — the two numbers that make the whole zoo.
// ---------------------------------------------------------------------------

// Feed rate F — how fast the activator is replenished. Thousandths, so the slider
// is an integer; 0.054 is written 54.
//
//notebook:slider min=10 max=90 step=1
func feedMilli() (f int) { return 30 }

// Kill rate k — how fast the inhibitor is removed. Also in thousandths; 0.062 is 62.
//
//notebook:slider min=45 max=75 step=1
func killMilli() (k int) { return 57 }

// Steps to simulate. The horizon and the scrub axis — drag up to develop the
// pattern further, down to rewind to the soup. Also the WASM-cost knob: each step
// is a full-grid update, serial in the browser, parallel on a cluster.
//
//notebook:slider min=500 max=8000 step=500
func steps() (n int) { return 6000 }

// ---------------------------------------------------------------------------
// The simulation — fixed horizon, pure.
// ---------------------------------------------------------------------------

// The pattern after n steps of Gray-Scott from a fixed seeded soup. Pure: a
// function of (feed, kill, steps) alone — same inputs, same field, every time —
// which is what lets the step slider scrub backward exactly. Returns a Field, which
// has a Render method, so the engine paints it as the visible Turing pattern.
//
//notebook:height=440
func field(f int, k int, n int) (grid Field) {
	return simulate(float64(f)/1000, float64(k)/1000, n)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// Note there is no separate "render" cell: `field` already returns a Field, and
// Field has a Render method, so the engine renders it directly. Calling
// grid.Render() inside a cell body would pull fmt (via Render) into the cell's
// call graph and trip the conservative fmt→os WASM gate — the same reason the
// other notebooks keep fmt inside Render, which the engine calls, not a cell.

// Which regime the two numbers land in — a plain-language readout, so the sliders
// have a legend. The boundaries are approximate; the fun is finding them by hand.
func regime(f int, k int) (where Readout) {
	return Readout{Cards: []Card{
		{Label: "feed F", Value: milli(f)},
		{Label: "kill k", Value: milli(k)},
		{Label: "regime", Value: classify(f, k), Caption: "approximate — drag across the boundaries"},
	}}
}

// Turing patterns from two numbers.
func intro() (md Markdown) {
	return `Two chemicals, two rates. Drag **feed** and **kill** to move through the
Gray-Scott parameter plane — spots, stripes, mazes, coral — and scrub **steps** to
watch a pattern grow out of a seeded soup.

The grid update is embarrassingly parallel: every cell from its neighbours, the
textbook case for the goroutine fan-out. Built for a cluster, that fan-out is real.
Built for this browser tab, ` + "`GOOS=js`" + ` is single-threaded and the steps run
serially — turn the step count up and you can feel it. Same file; the parallel
dividend is present natively and absent here, and the project says so out loud.`
}

// ===========================================================================
// Gray-Scott
// ===========================================================================

// Diffusion rates for the two chemicals. U (activator) diffuses twice as fast as V
// (inhibitor) — the rate difference is what breaks symmetry and makes patterns.
const (
	diffU = 0.16
	diffV = 0.08
	dt    = 1.0
)

// simulate runs n Gray-Scott steps on a grid×grid field seeded with U=1 everywhere
// and a small central square of V, then returns the V concentration field. Serial
// by construction here — the per-cell update is where a native build would fan out.
func simulate(feed, kill float64, n int) Field {
	u := make([]float64, grid*grid)
	v := make([]float64, grid*grid)
	for i := range u {
		u[i] = 1
	}
	// Seed a small square of V at the centre — the perturbation patterns grow from.
	for y := grid/2 - 6; y < grid/2+6; y++ {
		for x := grid/2 - 6; x < grid/2+6; x++ {
			v[y*grid+x] = 0.25
			u[y*grid+x] = 0.5
		}
	}

	nu := make([]float64, grid*grid)
	nv := make([]float64, grid*grid)
	for range n {
		step(u, v, nu, nv, feed, kill)
		u, nu = nu, u
		v, nv = nv, v
	}
	return Field{V: v}
}

// step advances every cell one Gray-Scott timestep, writing into (outU, outV). The
// Laplacian is a 3×3 stencil on a toroidal (wrap-around) grid. This loop is the
// parallel kernel: each output cell depends only on the OLD field, so on a native
// build the rows fan out across cores; under GOOS=js it runs serially.
func step(u, v, outU, outV []float64, feed, kill float64) {
	for y := 0; y < grid; y++ {
		ym := ((y - 1) + grid) % grid
		yp := (y + 1) % grid
		for x := 0; x < grid; x++ {
			xm := ((x - 1) + grid) % grid
			xp := (x + 1) % grid
			c := y*grid + x
			uc, vc := u[c], v[c]
			// Weighted Laplacian (the standard Gray-Scott 3×3 stencil).
			lapU := 0.2*(u[y*grid+xm]+u[y*grid+xp]+u[ym*grid+x]+u[yp*grid+x]) +
				0.05*(u[ym*grid+xm]+u[ym*grid+xp]+u[yp*grid+xm]+u[yp*grid+xp]) - uc
			lapV := 0.2*(v[y*grid+xm]+v[y*grid+xp]+v[ym*grid+x]+v[yp*grid+x]) +
				0.05*(v[ym*grid+xm]+v[ym*grid+xp]+v[yp*grid+xm]+v[yp*grid+xp]) - vc
			uvv := uc * vc * vc
			outU[c] = uc + (diffU*lapU-uvv+feed*(1-uc))*dt
			outV[c] = vc + (diffV*lapV+uvv-(kill+feed)*vc)*dt
		}
	}
}

// classify names the pattern regime for a feed/kill pair. Boundaries are rough —
// the point is to give the sliders a legend, not to be authoritative.
func classify(f, k int) string {
	switch {
	case k < 55:
		return "coral / mitosis"
	case k < 60 && f < 40:
		return "moving spots"
	case k < 62:
		return "mazes"
	case k < 66:
		return "stripes"
	default:
		return "stable spots"
	}
}

// ===========================================================================
// Helpers & types
// ===========================================================================

func milli(v int) string { return "0." + pad3(v) }

func pad3(v int) string {
	s := strconv.Itoa(v)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// Field is the V (inhibitor) concentration grid — the pattern to display.
type Field struct {
	V []float64
}

// Render maps the concentration field through a palette to a PNG, embedded in an
// SVG wrapper so the client (which injects image/svg+xml but not a bare image/png)
// paints it. Upscaled with a nearest-neighbour <image> so the 96×96 grid fills the
// panel crisply.
func (fld Field) Render() Rendered {
	img := image.NewRGBA(image.Rect(0, 0, grid, grid))
	for i, v := range fld.V {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = palette(v)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	const box = 420
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, box, box)
	// image-rendering:pixelated keeps the cells sharp when upscaled.
	fmt.Fprintf(&b, `<image x="0" y="0" width="%d" height="%d" href=%q `+
		`style="image-rendering:pixelated"/>`, box, box, uri)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// palette maps a V concentration in [0,1] to an R,G,B,A: low V deep indigo, high V
// warm, a rising ramp so the pattern reads as relief. Unnamed results, so the
// analyzer treats it as a helper (a cell is a documented func with NAMED results).
func palette(v float64) (uint8, uint8, uint8, uint8) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	// Two-stop ramp: indigo → teal → warm white.
	switch {
	case v < 0.5:
		t := v / 0.5
		return lerp(27, 20, t), lerp(20, 130, t), lerp(80, 130, t), 255
	default:
		t := (v - 0.5) / 0.5
		return lerp(20, 250, t), lerp(130, 240, t), lerp(130, 200, t), 255
	}
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
