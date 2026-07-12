//go:notebook
//
// Mandelbrot.
//
// marimo's gallery has two of these: a Cython-vs-Python benchmark, and a version using
// the marimo-cython plugin "for speedup." Both notebooks exist because the inner loop
// is unbearable in the host language. Porting them deletes the subject, which makes it
// a rigged fight and not worth staging.
//
// So here is the honest version of the same fight. The benchmark at the bottom is not
// Go-versus-Python; it is a STRONG SCALING CURVE — wall time against thread count, with
// the ideal line for reference. That is a real measurement, it is the one you actually
// want when you are sizing a machine, and it is the one a Python notebook cannot make
// in-process at all. Not "slower." Cannot.
//
// Two things surfaced building it that had nothing to do with speed.
//
//   BENCHMARKS ARE HOSTILE TO MEMOIZATION. Our engine keys the cache on input versions.
//   A timing cell with unchanged inputs returns the FIRST run's number, forever — so
//   re-running to check variance is impossible, and the notebook reports a stale
//   measurement with total confidence. This is worse than being slow: memoization here
//   produces silently wrong numbers. A benchmark's output depends on EXECUTION, not on
//   inputs, which is the exact definition of uncacheable and is invisible in the
//   signature. Hence //notebook:nocache, and it is a real hole in "purity is derivable."
//
//   A BUTTON IS A MANUAL CLOCK. To re-run a benchmark you need an input that changes
//   when nothing else has. That is a Tick — the same type the queue simulator's timer
//   writes. Runtime writes it on a schedule: timer. User writes it by clicking: button.
//   One leaf kind, two writers. Nothing new was needed.
//
// And one design note in passing. `center` and `zoom` are ABSOLUTE controls, so they are
// stateless leaves and you can scrub them backward freely. Brush-to-zoom — drag a
// rectangle, zoom into it — is a RELATIVE gesture: the new viewport is a function of the
// old one. That is path-dependent, so it needs Prev[Viewport] and a gesture clock. Which
// is precisely why every zoom UI in existence has a back button, and no slider does.
// Same rule as the queue and the posterior, third time: relative accumulates, absolute
// recomputes.

package mandelbrot

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
)

const width, height = 800, 600

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

// Center. Drag it on the image.
func center() (at Draggable[Complex]) {
	return Draggable[Complex]{Value: []Complex{{-0.743643887037, 0.131825904205}}}
}

// Zoom, in powers of ten.
//
//notebook:slider min=0 max=13 step=0.1
func zoom() (mag float64) { return 0.5 }

// Iteration limit. Deep zooms need more; shallow ones waste them.
//
//notebook:slider min=64 max=4096 step=64
func maxIter() (limit int) { return 512 }

// ---------------------------------------------------------------------------
// The set
// ---------------------------------------------------------------------------

// The set, at the current viewport.
//
//notebook:height=600
func view(at Draggable[Complex], mag float64, limit int) (picture Image) {
	c := at.Value[0]
	span := 3.0 / math.Pow(10, mag)

	px := make([]color.RGBA, width*height)
	parallelRows(height, runtime.NumCPU(), func(y int) {
		for x := range width {
			z := Complex{
				Re: c.Re + (float64(x)/width-0.5)*span,
				Im: c.Im + (float64(y)/height-0.5)*span*height/width,
			}
			px[y*width+x] = shade(escape(z, limit), limit)
		}
	})
	return Image{W: width, H: height, Px: px, Grip: at.Grip(0)}
}

// Depth at which the pixel under the crosshair escapes.
func depth(at Draggable[Complex], limit int) (iterations float64) {
	return escape(at.Value[0], limit)
}

// ---------------------------------------------------------------------------
// The benchmark
// ---------------------------------------------------------------------------

// Run the scaling test.
//
//notebook:button
func runBench() (trigger Tick) { return 0 }

// Wall time against thread count.
//
// //notebook:nocache is load-bearing. Without it, the memo table would hand back the first
// run's timings on every subsequent click, and the notebook would report a stale number
// as confidently as a fresh one.
//
//notebook:nocache
func scaling(trigger Tick, at Draggable[Complex], mag float64, limit int) (curve Scaling) {
	if trigger == 0 {
		return curve // nothing measured yet
	}
	c, span := at.Value[0], 3.0/math.Pow(10, mag)

	for n := 1; n <= runtime.NumCPU(); n *= 2 {
		start := time.Now()
		sink := make([]float64, width*height)
		parallelRows(height, n, func(y int) {
			for x := range width {
				sink[y*width+x] = escape(Complex{
					Re: c.Re + (float64(x)/width-0.5)*span,
					Im: c.Im + (float64(y)/height-0.5)*span*height/width,
				}, limit)
			}
		})
		curve.Points = append(curve.Points, Timing{Threads: n, Wall: time.Since(start)})
	}
	return curve
}

// Speedup.
//
//notebook:height=300
func speedup(curve Scaling) (plot Chart) {
	if len(curve.Points) == 0 {
		return Chart{Title: "press Run to measure"}
	}
	base := curve.Points[0].Wall.Seconds()
	plot = Chart{Title: "speedup vs. threads (dashed = ideal)"}
	for _, p := range curve.Points {
		plot.X = append(plot.X, float64(p.Threads))
		plot.Actual = append(plot.Actual, base/p.Wall.Seconds())
		plot.Ideal = append(plot.Ideal, float64(p.Threads))
	}
	return plot
}

// Mandelbrot.
func intro() (md Markdown) {
	return `Drag the crosshair to pan; the zoom slider goes to 10¹³, which is where float64
runs out of mantissa and the boundary goes blocky. That limit is the interesting part of
this notebook, and it is a numerical fact, not a language one.

The benchmark below measures strong scaling across cores — a curve that cannot be drawn
from inside a CPython process at all.`
}

// ===========================================================================
// Kernel
// ===========================================================================

// escape returns a smooth (fractional) escape time, or 0 for points in the set.
func escape(c Complex, limit int) float64 {
	// Cardioid and period-2 bulb: the cheap wins that make deep zooms tolerable.
	q := (c.Re-0.25)*(c.Re-0.25) + c.Im*c.Im
	if q*(q+(c.Re-0.25)) <= 0.25*c.Im*c.Im ||
		(c.Re+1)*(c.Re+1)+c.Im*c.Im <= 0.0625 {
		return 0
	}
	var zr, zi float64
	for i := range limit {
		zr2, zi2 := zr*zr, zi*zi
		if zr2+zi2 > 4 {
			// Smooth coloring: continuous, so the bands don't stair-step.
			return float64(i) + 1 - math.Log2(math.Log(math.Sqrt(zr2+zi2)))
		}
		zr, zi = zr2-zi2+c.Re, 2*zr*zi+c.Im
	}
	return 0
}

func shade(n float64, limit int) color.RGBA {
	if n == 0 {
		return color.RGBA{8, 8, 20, 255}
	}
	t := math.Sqrt(n / float64(limit))
	return color.RGBA{
		R: uint8(255 * math.Min(1, 1.4*t*t)),
		G: uint8(255 * math.Min(1, 0.9*t)),
		B: uint8(255 * math.Min(1, 0.35+0.65*math.Sin(3.1*t))),
		A: 255,
	}
}

// parallelRows is the whole parallelism story. There is no GIL to route around, no
// second language, and no JIT to warm up — which is also why the timings below mean
// something on the first run.
func parallelRows(h, n int, f func(y int)) {
	n = max(min(n, h), 1)
	var wg sync.WaitGroup
	for w := range n {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for y := start; y < h; y += n {
				f(y)
			}
		}(w)
	}
	wg.Wait()
}

// ===========================================================================
// Types
// ===========================================================================

type Complex struct{ Re, Im float64 }

type Tick uint64

type Draggable[T any] struct {
	Value []T
	at    leafToken
}

func (d Draggable[T]) Grip(i int) Ref { return Ref{leaf: d.at, index: i} }

type leafToken uint64
type Ref struct {
	leaf  leafToken
	index int
}

type Timing struct {
	Threads int
	Wall    time.Duration
}

type Scaling struct{ Points []Timing }

type Image struct {
	W, H int
	Px   []color.RGBA
	Grip Ref
}

func (im Image) Render() Rendered {
	rgba := image.NewRGBA(image.Rect(0, 0, im.W, im.H))
	copy(rgba.Pix, pixelBytes(im.Px))
	var buf bytes.Buffer
	_ = png.Encode(&buf, rgba)
	return Rendered{
		MIME: "image/png",
		Data: base64.StdEncoding.EncodeToString(buf.Bytes()),
		// The crosshair is a grip at the image center: dragging it writes `center`.
		Grips: []Handle{{X: 0.5, Y: 0.5, Ref: im.Grip}},
	}
}

func pixelBytes(px []color.RGBA) []byte {
	out := make([]byte, len(px)*4)
	for i, p := range px {
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = p.R, p.G, p.B, p.A
	}
	return out
}

type Handle struct {
	X, Y float64 // fractional position within the output
	Ref  Ref
}

type Chart struct {
	Title         string
	X             []float64
	Actual, Ideal []float64
}

func (c Chart) Render() Rendered {
	const w, h, pad = 640.0, 300.0, 44.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	if len(c.X) > 0 {
		hi := c.Ideal[len(c.Ideal)-1]
		sx := func(v float64) float64 { return pad + math.Log2(v)/math.Log2(hi)*(w-2*pad) }
		sy := func(v float64) float64 { return h - pad - (v-1)/(hi-1)*(h-2*pad) }
		line := func(ys []float64, color, dash string) {
			var d strings.Builder
			for i, v := range ys {
				verb := " L"
				if i == 0 {
					verb = "M"
				}
				fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(c.X[i]), sy(v))
			}
			fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2" `+
				`stroke-dasharray=%q/>`, d.String(), color, dash)
		}
		line(c.Ideal, "#94a3b8", "5 4")
		line(c.Actual, "#4338ca", "")
		for i, v := range c.Actual {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="4" fill="#4338ca"/>`+
				`<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10">%.0f×</text>`,
				sx(c.X[i]), sy(v), sx(c.X[i])+7, sy(v)-6, v)
		}
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12">%s</text>`,
		pad, c.Title)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Rendered struct {
	MIME  string
	Data  string
	Grips []Handle
}

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
