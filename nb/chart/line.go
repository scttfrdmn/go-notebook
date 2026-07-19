package chart

import (
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb"
)

// LineChart is a multi-series line plot. Build it with [Line] (common case) or
// [LineWith] (title, axis labels, log scale, height). It renders as an SVG that
// carries its own light/dark styling.
type LineChart struct {
	series []Series
	opts   Opts
}

// Line plots one or more series as connected lines, points taken in order. With
// two or more named series a legend appears; a single series needs none (the
// title says what is plotted).
func Line(series ...Series) LineChart { return LineChart{series: series} }

// LineWith is [Line] with options.
func LineWith(opts Opts, series ...Series) LineChart {
	return LineChart{series: series, opts: opts}
}

// Render draws the line chart.
func (l LineChart) Render() nb.Rendered {
	const defW, defH = 720, 380
	c := newCanvas(defW, l.opts.height(defH))
	p := newPlot(c, l.opts, l.series, true)

	for i, s := range l.series {
		if len(s.XY) == 0 {
			continue
		}
		cls := c.use(i)
		// The path.
		var d strings.Builder
		for j, pt := range s.XY {
			cmd := "L"
			if j == 0 {
				cmd = "M"
			}
			d.WriteString(cmd)
			d.WriteString(ftoa(p.sx(pt.X)))
			d.WriteByte(' ')
			d.WriteString(ftoa(p.sy(pt.Y)))
			d.WriteByte(' ')
		}
		c.rawf(`<path class="%s" d="%s" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`,
			cls, strings.TrimSpace(d.String()))

		// An end-marker with a 2px surface ring, so the line's end reads where it
		// crosses another. r=4 (>=8px mark).
		end := s.XY[len(s.XY)-1]
		c.rawf(`<circle class="%s" cx="%.1f" cy="%.1f" r="4" fill="currentColor" stroke="var(--surface)" stroke-width="2"/>`,
			cls, p.sx(end.X), p.sy(end.Y))

		// Selective direct label at the end, when the series is named and the ends
		// don't collide. Otherwise the legend carries identity.
		if p.labelEnds && s.Name != "" {
			p.directLabel(i, end, s.Name)
		}
	}

	return nb.SVG(c.finish())
}

// ftoa formats a coordinate with one decimal — compact, stable path data.
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
