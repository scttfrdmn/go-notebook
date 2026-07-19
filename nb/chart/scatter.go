package chart

import "github.com/scttfrdmn/go-notebook/nb"

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
