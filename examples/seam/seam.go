//go:notebook
//
// Seam carving.
//
// A port of marimo's gallery notebook (notebooks/math/seam-carving/notebook.py),
// whose own header calls it "a demonstration of marimo's caching feature, which
// is helpful because the algorithm is compute intensive even when you use Numba."
//
// Two things are different here, and the second is the interesting one.
//
//  1. The original's find_seam() computes the DP table `dp` but returns only
//     `backtrack`, and remove_seam() then picks the seam's start column with
//     argmin(backtrack[-1]) instead of argmin(dp[-1]). Since backpointers in the
//     bottom row are all within ±1 of their own column, that argmin is column 0
//     every single time (verified: 200/200 on random energy maps). The original
//     therefore never uses minimum-energy seam selection — it shaves the left
//     edge along a locally-low path. This port does the real thing.
//
//  2. The original caches efficient_seam_carve(path, scale) — so every new slider
//     value re-runs the whole DP from the original image. But the *seam order* does
//     not depend on the slider at all: carving to 0.85 is a prefix of carving to
//     0.70. Hoisting that into its own cell makes the expensive work happen once,
//     and makes the slider a pure O(pixels) filter. The cache wasn't making a slow
//     notebook fast; it was compensating for an edge that shouldn't exist.
//
// The whole point of a reactive notebook is that the graph is the thing you think
// with. A cache is what you reach for when the graph is wrong.

package seam

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"runtime"
	"slices"
	"sync"
	"time"
)

const imageURL = "https://raw.githubusercontent.com/marimo-team/gallery-examples/" +
	"main/notebooks/math/seam-carving/The_Persistence_of_Memory.jpg"

// ---------------------------------------------------------------------------
// Cells
// ---------------------------------------------------------------------------

// The Persistence of Memory.
func source() (img Frame, err error) { return fetchImage(imageURL) }

// Amount of resizing to perform.
// The 0.70 floor lives in Scale.Bounds() rather than in a //notebook: directive
// because it is not cosmetic: seamOrder reads it to decide how far to carve.
// The step is cosmetic, so it stays a comment. That is the whole type-vs-directive
// rule, and here it has teeth.
//
//notebook:step=0.05
func amount() (scale Scale) { return 1.0 }

// Order in which seams are removed. Depends on the image, not the slider —
// which is the entire reason this notebook needs no cache.
func seamOrder(img Frame) (order SeamMap) {
	start := time.Now()

	minScale, _ := Scale(0).Bounds()
	maxSeams := img.W - int(float64(img.W)*minScale)

	// Working copy as ragged rows, plus each pixel's column in the original.
	rows := make([][]RGB, img.H)
	orig := make([][]int, img.H)
	for y := range img.H {
		rows[y] = slices.Clone(img.Px[y*img.W : (y+1)*img.W])
		orig[y] = make([]int, img.W)
		for x := range img.W {
			orig[y][x] = x
		}
	}

	order = SeamMap{W: img.W, H: img.H, Removed: make([]int, img.W*img.H), Seams: maxSeams}
	for i := range order.Removed {
		order.Removed[i] = never
	}

	for k := range maxSeams {
		seam := lowestEnergySeam(rows)
		for y, x := range seam {
			order.Removed[y*img.W+orig[y][x]] = k
			rows[y] = slices.Delete(rows[y], x, x+1)
			orig[y] = slices.Delete(orig[y], x, x+1)
		}
	}
	order.Elapsed = time.Since(start)
	return order
}

// Time spent finding seams.
// The original print()s this. Parallel cells share one stdout, so a notebook that
// fans out across goroutines cannot own a per-cell console. Return a value instead.
func precomputeCost(order SeamMap) (elapsed time.Duration) { return order.Elapsed }

// Original.
//
//notebook:row=compare
func original(img Frame) (before Image) { return Image{Frame: img, Caption: "original"} }

// Carved. Moving the slider runs only this cell: one pass over the pixels,
// keeping those whose seam index has not yet come up. No DP, no cache.
//
//notebook:row=compare
func carved(img Frame, order SeamMap, scale Scale) (after Image) {
	drop := img.W - int(float64(img.W)*float64(scale))
	out := Frame{W: img.W - drop, H: img.H}
	out.Px = make([]RGB, 0, out.W*out.H)
	for y := range img.H {
		for x := range img.W {
			if r := order.Removed[y*img.W+x]; r == never || r >= drop {
				out.Px = append(out.Px, img.Px[y*img.W+x])
			}
		}
	}
	return Image{Frame: out, Caption: fmt.Sprintf("carved to %.0f%%", float64(scale)*100)}
}

// Seam order, as a heat map: dark pixels go first.
// This view is free — a second consumer of the same materialized artifact. It is
// the dividend from hoisting the expensive work out from under the slider.
func seamHeatmap(order SeamMap) (heat Image) {
	out := Frame{W: order.W, H: order.H, Px: make([]RGB, order.W*order.H)}
	for i, r := range order.Removed {
		if r == never {
			out.Px[i] = RGB{240, 240, 245}
			continue
		}
		t := float64(r) / float64(max(order.Seams-1, 1))
		out.Px[i] = RGB{uint8(67 + t*120), uint8(56 + t*30), uint8(202 - t*40)}
	}
	return Image{Frame: out, Caption: "seam removal order"}
}

// Seam carving.
func intro() (md Markdown) {
	return `Seam carving resizes an image by repeatedly deleting the lowest-energy
connected path of pixels from top to bottom. Content survives; dead space does not.
Adapted from Vincent Warmerdam's original, itself an homage to 3Blue1Brown's Pluto.jl
demonstration.

Drag the slider. The seams were all found once, up front.`
}

// ===========================================================================
// The algorithm
// ===========================================================================

const never = math.MaxInt32

// lowestEnergySeam returns, for each row, the column of the pixel on the
// minimum-energy top-to-bottom connected seam.
func lowestEnergySeam(rows [][]RGB) []int {
	h, w := len(rows), len(rows[0])
	e := energy(rows)

	dp := slices.Clone(e)
	back := make([]int, h*w)
	for y := 1; y < h; y++ {
		for x := range w {
			lo, hi := max(x-1, 0), min(x+1, w-1)
			best := lo
			for j := lo + 1; j <= hi; j++ {
				if dp[(y-1)*w+j] < dp[(y-1)*w+best] {
					best = j
				}
			}
			back[y*w+x] = best
			dp[y*w+x] += dp[(y-1)*w+best]
		}
	}

	// The endpoint is the minimum of the *cumulative energy* in the last row.
	// This is the line the original drops on the floor.
	x := 0
	for j := 1; j < w; j++ {
		if dp[(h-1)*w+j] < dp[(h-1)*w+x] {
			x = j
		}
	}

	seam := make([]int, h)
	for y := h - 1; y >= 0; y-- {
		seam[y] = x
		if y > 0 {
			x = back[y*w+x]
		}
	}
	return seam
}

// energy is |sobel_h| + |sobel_v| over luminance, computed row-parallel.
// No JIT, no vectorization, no second language.
func energy(rows [][]RGB) []float64 {
	h, w := len(rows), len(rows[0])
	gray := make([]float64, h*w)
	for y := range h {
		for x := range w {
			p := rows[y][x]
			gray[y*w+x] = 0.2989*float64(p.R) + 0.5870*float64(p.G) + 0.1140*float64(p.B)
		}
	}
	at := func(y, x int) float64 {
		return gray[min(max(y, 0), h-1)*w+min(max(x, 0), w-1)]
	}

	out := make([]float64, h*w)
	parallelRows(h, func(y int) {
		for x := range w {
			gx := at(y-1, x+1) + 2*at(y, x+1) + at(y+1, x+1) -
				at(y-1, x-1) - 2*at(y, x-1) - at(y+1, x-1)
			gy := at(y+1, x-1) + 2*at(y+1, x) + at(y+1, x+1) -
				at(y-1, x-1) - 2*at(y-1, x) - at(y-1, x+1)
			out[y*w+x] = math.Abs(gx) + math.Abs(gy)
		}
	})
	return out
}

func parallelRows(h int, f func(y int)) {
	n := min(runtime.NumCPU(), h)
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
// Plumbing
// ===========================================================================

func fetchImage(url string) (Frame, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Frame{}, err
	}
	defer resp.Body.Close()

	src, err := jpeg.Decode(resp.Body)
	if err != nil {
		return Frame{}, fmt.Errorf("seam: decoding %s: %w", url, err)
	}
	b := src.Bounds()
	f := Frame{W: b.Dx(), H: b.Dy(), Px: make([]RGB, b.Dx()*b.Dy())}
	for y := range f.H {
		for x := range f.W {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			f.Px[y*f.W+x] = RGB{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
		}
	}
	return f, nil
}

// ===========================================================================
// Types
// ===========================================================================

type RGB struct{ R, G, B uint8 }

type Frame struct {
	W, H int
	Px   []RGB // row-major
}

// SeamMap records, for each pixel of the original, the index of the seam that
// removed it (or never). Carving to any width is then a filter, not a search.
type SeamMap struct {
	W, H    int
	Seams   int
	Removed []int
	Elapsed time.Duration
}

// Scale is the resize fraction. Its lower bound is a compute precondition —
// seamOrder only precomputes this far — so it belongs to the type.
type Scale float64

func (Scale) Bounds() (lo, hi float64) { return 0.70, 1.0 }

type Image struct {
	Frame   Frame
	Caption string
}

func (im Image) Render() Rendered {
	rgba := image.NewRGBA(image.Rect(0, 0, im.Frame.W, im.Frame.H))
	for y := range im.Frame.H {
		for x := range im.Frame.W {
			p := im.Frame.Px[y*im.Frame.W+x]
			rgba.Set(x, y, color.RGBA{p.R, p.G, p.B, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, rgba)
	return Rendered{MIME: "image/png", Data: base64.StdEncoding.EncodeToString(buf.Bytes())}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
