//go:notebook
//
// Mandelbrot — a fractal you can zoom until float64 runs out of mantissa.
//
// The Mandelbrot set is the cleanest example of "infinite detail from a trivial rule":
// a point c is in the set if the iteration z ← z² + c never escapes to infinity. Color
// each pixel by how many iterations it takes to escape (points that never escape are
// the black interior), and you get the famous boundary — self-similar, endlessly
// intricate, and computed by nothing more than repeated squaring.
//
// Every pixel is an INDEPENDENT escape-time calculation — a pure function of its
// position, the viewport, and the iteration limit. So the whole image is a pure
// function of (center, zoom, iterations): move the viewport and it recomputes from
// scratch, no state carried. That is exactly the shape that runs live in the browser
// tab, and this notebook does — drag the sliders and watch it recompute.
//
// The interesting limit here is numerical, not computational. Push the zoom slider
// toward 10¹³ and the boundary goes blocky — that is `float64` running out of mantissa
// bits to distinguish neighbouring pixels, not a bug. It's the honest edge of
// double-precision arithmetic, visible on screen: past ~15 significant digits, two
// adjacent pixels map to the same complex number and the detail flattens. Deeper zooms
// in the wild need arbitrary-precision arithmetic for exactly this reason.
//
// A note on what this notebook DEFERS. Panning by absolute (centerX, centerY) sliders
// is stateless — each is an absolute coordinate, so you can scrub any of them backward
// freely and the image recomputes. Drag-a-box-to-zoom would be a RELATIVE gesture (the
// new viewport is a function of the old one), which is path-dependent and needs a
// Prev[Viewport] carry the engine doesn't have yet (tracked separately). Absolute
// controls recompute; relative ones accumulate — so this notebook uses the absolute
// ones, and stays pure and WASM-live.
//
// (Historical note: marimo's gallery ships this as a Cython-vs-Python *speed*
// benchmark — the inner loop is unbearable in pure Python. In Go the escape loop is
// just fast, so there's no language fight to stage; the subject is the fractal and its
// numerical limit, which is the part actually worth looking at.)

package mandelbrot

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

const width, height = 720, 540

// ---------------------------------------------------------------------------
// Controls — all absolute, so every one scrubs backward freely.
// ---------------------------------------------------------------------------

// Center, real axis. The classic seahorse-valley detail lives near −0.7436.
//
//notebook:slider min=-2.5 max=1.0 step=0.0001
func centerRe() (cx float64) { return -0.743643887037 }

// Center, imaginary axis.
//
//notebook:slider min=-1.5 max=1.5 step=0.0001
func centerIm() (cy float64) { return 0.131825904205 }

// Zoom, in powers of ten. Push it toward 13 and the boundary goes blocky — that's
// float64 running out of mantissa, the numerical edge this notebook is really about.
//
//notebook:slider min=0 max=13 step=0.1
func zoom() (mag float64) { return 3.0 }

// Iteration limit. Deep zooms need more to resolve the fine boundary; shallow ones
// waste them. Higher = sharper detail but more compute per frame.
//
//notebook:slider min=64 max=2048 step=64
func maxIter() (limit int) { return 512 }

// ---------------------------------------------------------------------------
// The set — a pure function of the viewport.
// ---------------------------------------------------------------------------

// render computes the escape-time image at the current viewport. Pure in
// (cx, cy, mag, limit): every pixel is an independent z←z²+c iteration, so the whole
// image recomputes from scratch when any slider moves — no state, WASM-portable. The
// span shrinks by 10^mag, which is the zoom.
//
//notebook:height=560
func render(cx float64, cy float64, mag float64, limit int) (picture Image) {
	span := 3.0 / math.Pow(10, mag)
	px := make([]color.RGBA, width*height)
	for y := 0; y < height; y++ {
		im := cy + (float64(y)/height-0.5)*span*height/width
		for x := 0; x < width; x++ {
			re := cx + (float64(x)/width-0.5)*span
			px[y*width+x] = shade(escape(re, im, limit), limit)
		}
	}
	return Image{W: width, H: height, Px: px}
}

// depth reports the escape time at the exact center pixel — the number the crosshair
// would read. A quick scalar readout that proves the viewport is where you think.
func depth(cx float64, cy float64, limit int) (centerDepth float64) {
	return escape(cx, cy, limit)
}

// Mandelbrot — a fractal you can zoom until float64 runs out of mantissa.
func intro() (md Markdown) {
	return `The Mandelbrot set: a point c is in the set if **z ← z² + c** never escapes.
Color each pixel by how fast it escapes and the boundary appears — infinite detail
from repeated squaring. Every pixel is independent, so the image is a **pure function
of (center, zoom, iterations)**, which is exactly why it runs live in the tab.

Drag **zoom** toward 13 and the boundary goes blocky — that's not a bug, it's
` + "`float64`" + ` running out of mantissa bits to tell neighbouring pixels apart. The
numerical edge of double precision, on screen. Panning uses absolute center sliders
(stateless, scrub freely); box-drag zoom would be a relative gesture and is deferred.`
}

// ===========================================================================
// Kernel
// ===========================================================================

// escape returns a smooth (fractional) escape time for c = re + im·i, or 0 for points
// in the set. Cheap interior tests (cardioid + period-2 bulb) skip the iteration for
// the big black regions, which is what keeps deep zooms responsive.
func escape(re, im float64, limit int) float64 {
	q := (re-0.25)*(re-0.25) + im*im
	if q*(q+(re-0.25)) <= 0.25*im*im || (re+1)*(re+1)+im*im <= 0.0625 {
		return 0
	}
	var zr, zi float64
	for i := 0; i < limit; i++ {
		zr2, zi2 := zr*zr, zi*zi
		if zr2+zi2 > 4 {
			// Smooth (continuous) coloring so the bands don't stair-step.
			return float64(i) + 1 - math.Log2(math.Log(math.Sqrt(zr2+zi2)))
		}
		zr, zi = zr2-zi2+re, 2*zr*zi+im
	}
	return 0
}

func shade(n float64, limit int) color.RGBA {
	if n == 0 {
		return color.RGBA{8, 8, 20, 255} // interior
	}
	t := math.Sqrt(n / float64(limit))
	return color.RGBA{
		R: uint8(255 * math.Min(1, 1.4*t*t)),
		G: uint8(255 * math.Min(1, 0.9*t)),
		B: uint8(255 * math.Min(1, 0.35+0.65*math.Sin(3.1*t))),
		A: 255,
	}
}

// ===========================================================================
// Types
// ===========================================================================

// Image is the rendered viewport: a pixel buffer painted as a PNG inside an SVG
// wrapper (the client injects image/svg+xml, not a bare image/png).
type Image struct {
	W, H int
	Px   []color.RGBA
}

func (im Image) Render() Rendered {
	rgba := image.NewRGBA(image.Rect(0, 0, im.W, im.H))
	copy(rgba.Pix, pixelBytes(im.Px))
	var buf bytes.Buffer
	_ = png.Encode(&buf, rgba)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, im.W, im.H)
	fmt.Fprintf(&b, `<image x="0" y="0" width="%d" height="%d" href=%q/>`, im.W, im.H, uri)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

func pixelBytes(px []color.RGBA) []byte {
	out := make([]byte, len(px)*4)
	for i, p := range px {
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = p.R, p.G, p.B, p.A
	}
	return out
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
