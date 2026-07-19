package chart

import (
	"math"

	"github.com/scttfrdmn/go-notebook/nb"
)

// ScatterChart is a multi-series point cloud. Build it with [Scatter] or
// [ScatterWith]. Each series is drawn as dots in its slot color, with a 2px
// surface ring so overlapping points stay legible.
type ScatterChart struct {
	series []Series
	opts   Opts
}

// Scatter plots one or more series as unconnected points.
func Scatter(series ...Series) ScatterChart { return ScatterChart{series: series} }

// ScatterWith is [Scatter] with options.
func ScatterWith(opts Opts, series ...Series) ScatterChart {
	return ScatterChart{series: series, opts: opts}
}

// Render draws the scatter chart.
func (s ScatterChart) Render() nb.Rendered {
	const defW, defH = 720, 420
	c := newCanvas(defW, s.opts.height(defH))
	p := newPlot(c, s.opts, s.series, false)

	for i, ser := range s.series {
		cls := c.use(i)
		// The fit line goes UNDER the points, so the cloud reads on top of it.
		if s.opts.Fit {
			s.drawFit(p, i, ser)
		}
		for _, pt := range ser.XY {
			// r=4 (>=8px mark), filled with the series color, 2px surface ring —
			// the ring is the overlap-legibility mechanism (never a border to
			// separate). Slight fill-opacity lets dense clouds show density.
			c.rawf(`<circle class="%s" cx="%.1f" cy="%.1f" r="4" fill="currentColor" fill-opacity="0.8" stroke="var(--surface)" stroke-width="2"/>`,
				cls, p.sx(pt.X), p.sy(pt.Y))
		}
	}

	return nb.SVG(c.finish())
}

// drawFit overlays the least-squares line for one series across the plot's
// x-range. It is the sanctioned exception to "forms never compose": the line is
// LinFit of the SAME points on the SAME axes — an annotation, not a second form.
// Drawn dashed and half-opacity so it reads as a derived model line, never
// mistaken for connected data. For a lone series the equation rides its end as a
// direct label (there's a free right gutter and no legend to compete with).
func (s ScatterChart) drawFit(p *plot, i int, ser Series) {
	if len(ser.XY) < 2 {
		return
	}
	xs := make([]float64, len(ser.XY))
	ys := make([]float64, len(ser.XY))
	loX, hiX := math.Inf(1), math.Inf(-1)
	for k, pt := range ser.XY {
		xs[k], ys[k] = pt.X, pt.Y
		loX, hiX = math.Min(loX, pt.X), math.Max(hiX, pt.X)
	}
	slope, intercept := LinFit(xs, ys)
	if slope == 0 && intercept == 0 {
		return // undefined fit (no x-variation)
	}
	cls := p.c.use(i)
	x1, y1 := loX, slope*loX+intercept
	x2, y2 := hiX, slope*hiX+intercept
	p.c.rawf(`<line class="%s" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="currentColor" stroke-width="2" stroke-opacity="0.5" stroke-dasharray="5 4" stroke-linecap="round"/>`,
		cls, p.sx(x1), p.sy(y1), p.sx(x2), p.sy(y2))

	// Single series: label the line with its equation at the right end.
	if len(s.series) == 1 {
		eq := "y = " + fmtNum(slope) + "x " + signed(intercept)
		lx := p.sx(x2) + 6
		ly := p.sy(y2)
		if lx+textW(eq, fontLabel) > float64(p.c.w)-4 {
			lx = p.sx(x2) - 6 - textW(eq, fontLabel)
		}
		p.c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" dominant-baseline="middle">%s</text>`,
			lx, ly, fontLabel, esc(eq))
	}
}

// signed formats an intercept as "+ 3.2" or "- 3.2" for the fit equation.
func signed(v float64) string {
	if v < 0 {
		return "- " + fmtNum(-v)
	}
	return "+ " + fmtNum(v)
}
