package chart

import (
	"math"

	"github.com/scttfrdmn/go-notebook/nb"
)

// Histogram bins a single slice of values and draws the counts as touching bars —
// the shape of a distribution. Build it with [Hist] / [HistWith]. One series, one
// color (slot 1): a histogram shows a distribution, not categories, so it never
// needs the categorical palette. Bars touch (a distribution is continuous) but
// carry the same 2px surface gap so adjacent bins read distinct.
type Histogram struct {
	values []float64
	bins   int
	opts   Opts
}

// Hist bins values into a sensible number of equal-width bins (Sturges' rule) and
// draws the counts.
func Hist(values []float64) Histogram { return Histogram{values: values} }

// HistWith is [Hist] with options and an explicit bin count (0 = auto).
func HistWith(opts Opts, bins int, values []float64) Histogram {
	return Histogram{values: values, bins: bins, opts: opts}
}

// Bins sets the bin count explicitly (0 restores the automatic choice).
func (h Histogram) Bins(n int) Histogram { h.bins = n; return h }

// Render draws the histogram.
func (h Histogram) Render() nb.Rendered {
	const defW, defH = 720, 380
	c := newCanvas(defW, h.opts.height(defH))

	lo, hi, counts := h.binned()
	maxCount := 0
	for _, n := range counts {
		if n > maxCount {
			maxCount = n
		}
	}
	nlo, nhi, yticks := niceScale(0, float64(maxCount), 5)

	// Margins (value axis on the left, numeric bin edges on the x).
	maxYLabel := 0.0
	for _, v := range yticks {
		maxYLabel = math.Max(maxYLabel, textW(fmtNum(v), fontTick))
	}
	left := 18.0 + maxYLabel + 8.0
	if h.opts.YLabel != "" {
		left += fontAxis + 12
	}
	top := padTop
	if h.opts.Title != "" {
		top += fontTitle + 8
	}
	bottom := float64(c.h) - (fontTick + 16)
	if h.opts.XLabel != "" {
		bottom -= fontAxis + 2
	}
	l, r := left, float64(c.w)-padRight

	vy := scale{lo: nlo, hi: nhi, p0: bottom, p1: top}
	vx := scale{lo: lo, hi: hi, p0: l, p1: r}

	// Grid + value-tick labels.
	for _, v := range yticks {
		y := vy.at(v)
		c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--grid)" stroke-width="1"/>`, l, y, r, y)
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="end" dominant-baseline="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			l-6, y, fontTick, esc(fmtNum(v)))
	}
	c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--axis)" stroke-width="1"/>`, l, bottom, r, bottom)

	// Bars — bin 0 is slot 1, all one color. A 2px surface gap between bins.
	const gap = 2.0
	cls := c.use(0)
	binW := (hi - lo) / float64(len(counts))
	for i, n := range counts {
		x0 := vx.at(lo + float64(i)*binW)
		x1 := vx.at(lo + float64(i+1)*binW)
		top := vy.at(float64(n))
		w := x1 - x0 - gap
		hgt := bottom - top
		if hgt <= 0.5 || w <= 0 {
			continue
		}
		// Rounded top, square baseline — same spec as the bar form.
		const rad = 4.0
		if hgt < rad*2 || w < rad*2 {
			c.rawf(`<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="currentColor"/>`,
				cls, x0, top, w, hgt)
		} else {
			c.rawf(`<path class="%s" fill="currentColor" d="M%.1f %.1f v%.1f a%.0f %.0f 0 0 1 %.0f %.0f h%.1f a%.0f %.0f 0 0 1 %.0f %.0f v%.1f z"/>`,
				cls, x0, top+hgt, -(hgt - rad), rad, rad, rad, -rad, w-2*rad, rad, rad, rad, rad, hgt-rad)
		}
	}

	// X-axis edge labels — a handful of nice ticks across the value range, not one
	// per bin (that would crowd).
	_, _, xt := niceScale(lo, hi, 6)
	lastRight := math.Inf(-1)
	for _, v := range xt {
		x := vx.at(v)
		if x < l-0.5 || x > r+0.5 {
			continue
		}
		label := fmtNum(v)
		half := textW(label, fontTick) / 2
		if x-half < lastRight+6 {
			continue
		}
		lastRight = x + half
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			x, bottom+fontTick+8, fontTick, esc(label))
	}

	// Chrome.
	if h.opts.Title != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" font-weight="600" fill="var(--ink)">%s</text>`,
			l, padTop+fontTitle*0.5, fontTitle, esc(h.opts.Title))
	}
	if h.opts.XLabel != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle">%s</text>`,
			(l+r)/2, float64(c.h)-6, fontAxis, esc(h.opts.XLabel))
	}
	if h.opts.YLabel != "" {
		yc := (top + bottom) / 2
		yx := (l - 8 - maxYLabel) * 0.58
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle" transform="rotate(-90 %.1f %.1f)">%s</text>`,
			yx, yc, fontAxis, yx, yc, esc(h.opts.YLabel))
	}

	return nb.SVG(c.finish())
}

// binned computes the histogram bins: the value range and per-bin counts. The bin
// count is the caller's if set, else Sturges' rule (1 + log2 n), a reasonable
// default for roughly-normal data.
func (h Histogram) binned() (lo, hi float64, counts []int) {
	if len(h.values) == 0 {
		return 0, 1, []int{0}
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range h.values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if !(hi > lo) {
		hi = lo + 1 // all values equal — one unit-wide bin
	}
	k := h.bins
	if k <= 0 {
		k = int(math.Ceil(1+math.Log2(float64(len(h.values))))) + 1
	}
	if k < 1 {
		k = 1
	}
	counts = make([]int, k)
	binW := (hi - lo) / float64(k)
	for _, v := range h.values {
		idx := int((v - lo) / binW)
		if idx >= k { // the max value lands in the last bin
			idx = k - 1
		}
		if idx < 0 {
			idx = 0
		}
		counts[idx]++
	}
	return lo, hi, counts
}
