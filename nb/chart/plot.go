package chart

import (
	"math"
)

// plot is the framed drawing area shared by the point-based forms (Line,
// Scatter): it owns the margins, the x/y scales, and the drawing of grid, ticks,
// axis titles, the plot title, and the legend. A form builds a plot from its
// series and options, draws its own marks against plot.sx / plot.sy, then calls
// legend() and direct labels. The categorical forms (Bar, Histogram) use their
// own framing since their x-axis is bands, not a scale.

// layout margins. Left grows to fit the widest y-tick label; the others are
// fixed generous padding so marks breathe.
const (
	padTop    = 16.0 // above the plot (title sits here when present)
	padRight  = 16.0
	padBottom = 34.0 // x ticks + x title
	tickLen   = 0.0  // ticks are label-only; the grid carries the line
	fontTick  = 11.0
	fontAxis  = 12.0
	fontTitle = 14.0
	fontLabel = 12.0 // direct end-labels
)

type plot struct {
	c          *canvas
	opts       Opts
	x, y       scale
	l, r, t, b float64 // plot rect edges in px
	xticks     []float64
	yticks     []float64
	labelEnds  bool // identity via direct end-labels (set by newPlot)
	showLegend bool // identity via a top legend row (the collision fallback)
	dotKey     bool // legend key is a dot (scatter) rather than a line+dot
}

// newPlot computes the data window from the series, expands it to nice ticks,
// reserves the margins (including room for the widest y-label and the legend/title
// bands), and draws the frame: grid, ticks, axis titles, and title. It returns a
// plot the caller draws data into.
//
// canDirectLabel says whether this form's marks have a meaningful "end" to hang a
// name on: true for a line (label the last point), false for a scatter cloud
// (which has no end, so identity must ride the legend). It selects the identity
// channel: direct end-labels vs a top legend row.
func newPlot(c *canvas, opts Opts, series []Series, canDirectLabel bool) *plot {
	// A form that cannot direct-label (scatter) falls back to a legend, whose key
	// should be a dot to match its marks.
	dotKey := !canDirectLabel
	// Data extent across all series.
	xlo, xhi := math.Inf(1), math.Inf(-1)
	ylo, yhi := math.Inf(1), math.Inf(-1)
	for _, s := range series {
		for _, p := range s.XY {
			xlo, xhi = math.Min(xlo, p.X), math.Max(xhi, p.X)
			ylo, yhi = math.Min(ylo, p.Y), math.Max(yhi, p.Y)
		}
	}
	if math.IsInf(xlo, 1) { // no points at all
		xlo, xhi, ylo, yhi = 0, 1, 0, 1
	}

	p := &plot{c: c, opts: opts}

	// Y scale: log or nice-linear. For linear, include zero-ish baselines only if
	// natural — niceScale already rounds outward.
	var nyl, nyh float64
	if opts.YLog && ylo > 0 {
		nyl, nyh, p.yticks = logTicks(ylo, yhi)
	} else {
		nyl, nyh, p.yticks = niceScale(ylo, yhi, 5)
	}
	nxl, nxh, xt := niceScale(xlo, xhi, 6)
	p.xticks = xt

	// Reserve the left margin for the widest y-tick label.
	maxYLabel := 0.0
	for _, v := range p.yticks {
		maxYLabel = math.Max(maxYLabel, textW(fmtNum(v), fontTick))
	}
	left := 12.0 + maxYLabel + 6.0
	if opts.YLabel != "" {
		left += fontAxis + 4 // rotated y-title column
	}

	named := 0
	for _, s := range series {
		if s.Name != "" {
			named++
		}
	}

	top := padTop
	if opts.Title != "" {
		top += fontTitle + 8
	}

	p.l, p.r = left, float64(c.w)-padRight
	p.t, p.b = top, float64(c.h)-padBottom
	if opts.XLabel != "" {
		p.b -= fontAxis + 2
	}

	// Provisional scales, to decide the identity channel: direct end-labels (the
	// Tufte-preferred "name the line, not a key") when 2+ series separate cleanly
	// at the right edge; a top legend only as the collision fallback. This choice
	// drives whether we reserve the right margin (for labels) or the top band
	// (for a legend), so it must happen before the final rect is fixed.
	p.x = scale{lo: nxl, hi: nxh, p0: p.l, p1: p.r}
	p.y = scale{lo: nyl, hi: nyh, p0: p.b, p1: p.t, log: opts.YLog && ylo > 0}
	if named >= 2 && canDirectLabel && !endsCollide(series, p) {
		p.labelEnds = true
		// Reserve room on the right for the widest end-label.
		maxEnd := 0.0
		for _, s := range series {
			if s.Name != "" {
				maxEnd = math.Max(maxEnd, textW(s.Name, fontLabel))
			}
		}
		p.r -= maxEnd + 10
	} else if named >= 2 {
		p.showLegend = true
		p.dotKey = dotKey
		top += fontLabel + 10
		p.t = top
	}
	// Re-fix scales against the adjusted rect.
	p.x = scale{lo: nxl, hi: nxh, p0: p.l, p1: p.r}
	p.y = scale{lo: nyl, hi: nyh, p0: p.b, p1: p.t, log: opts.YLog && ylo > 0}
	p.opts = opts

	p.drawFrame(series)
	return p
}

func (p *plot) sx(v float64) float64 { return p.x.at(v) }
func (p *plot) sy(v float64) float64 { return p.y.at(v) }

// drawFrame paints grid, ticks, axis titles, title, and legend — everything
// behind and around the data.
func (p *plot) drawFrame(series []Series) {
	c := p.c

	// Horizontal gridlines + y-tick labels. Grid is a hairline, one shade off the
	// surface, solid and recessive — never dashed.
	for _, v := range p.yticks {
		y := p.sy(v)
		if y < p.t-0.5 || y > p.b+0.5 {
			continue
		}
		c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--grid)" stroke-width="1"/>`,
			p.l, y, p.r, y)
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="end" dominant-baseline="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			p.l-6, y, fontTick, esc(fmtNum(v)))
	}

	// X-tick labels along the baseline (no vertical gridlines — they'd double the
	// ink; the horizontal grid carries the reading). Skip labels that would collide.
	lastRight := math.Inf(-1)
	for _, v := range p.xticks {
		x := p.sx(v)
		if x < p.l-0.5 || x > p.r+0.5 {
			continue
		}
		label := fmtNum(v)
		half := textW(label, fontTick) / 2
		if x-half < lastRight+6 { // would touch the previous label
			continue
		}
		lastRight = x + half
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			x, p.b+fontTick+6, fontTick, esc(label))
	}

	// Baseline (x-axis rule) — a single hairline a touch darker than the grid.
	c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--axis)" stroke-width="1"/>`,
		p.l, p.b, p.r, p.b)

	// Axis titles.
	if p.opts.XLabel != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle">%s</text>`,
			(p.l+p.r)/2, float64(c.h)-6, fontAxis, esc(p.opts.XLabel))
	}
	if p.opts.YLabel != "" {
		yc := (p.t + p.b) / 2
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle" transform="rotate(-90 %.1f %.1f)">%s</text>`,
			fontAxis+2, yc, fontAxis, fontAxis+2, yc, esc(p.opts.YLabel))
	}

	// Title.
	if p.opts.Title != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" font-weight="600" fill="var(--ink)">%s</text>`,
			p.l, padTop+fontTitle*0.5, fontTitle, esc(p.opts.Title))
	}

	// Legend for 2+ named series: a row of short line-keys + names under the
	// title. Identity is the colored key beside the name; the text stays in the
	// ink token (never the series hue — a light hue is illegible as text).
	p.drawLegend(series)
}

// drawLegend lays out the legend row when there are two or more named series.
func (p *plot) drawLegend(series []Series) {
	if !p.showLegend {
		return
	}
	named := make([]int, 0, len(series))
	for i, s := range series {
		if s.Name != "" {
			named = append(named, i)
		}
	}
	if len(named) < 2 {
		return
	}
	c := p.c
	y := padTop
	if p.opts.Title != "" {
		y += fontTitle + 8
	}
	y += fontLabel*0.5 + 2
	x := p.l
	for _, i := range named {
		cls := c.use(i)
		var tx float64
		if p.dotKey {
			// A single dot — matches a scatter's marks.
			c.rawf(`<circle class="%s" cx="%.1f" cy="%.1f" r="4" fill="currentColor"/>`, cls, x+4, y)
			tx = x + 13
		} else {
			// A short line-key with a dot — matches a line's mark + end-marker.
			c.rawf(`<line class="%s" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"/>`,
				cls, x, y, x+14, y)
			c.rawf(`<circle class="%s" cx="%.1f" cy="%.1f" r="3" fill="currentColor"/>`, cls, x+7, y)
			tx = x + 20
		}
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" dominant-baseline="middle">%s</text>`,
			tx, y, fontLabel, esc(series[i].Name))
		x = tx + textW(series[i].Name, fontLabel) + 18
	}
}

// directLabel places a series' name at its final point, nudged to avoid running
// off the right edge. Callers use it selectively (never a label on every point);
// it supplements the legend for the series whose end is uncrowded.
func (p *plot) directLabel(seriesIdx int, at Pt, name string) {
	if name == "" {
		return
	}
	c := p.c
	x := p.sx(at.X) + 6
	y := p.sy(at.Y)
	w := textW(name, fontLabel)
	if x+w > p.r {
		// No room to the right; place to the left of the point instead.
		x = p.sx(at.X) - 6 - w
	}
	c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" dominant-baseline="middle">%s</text>`,
		x, y, fontLabel, esc(name))
}

// endsCollide reports whether the series' final points are too close vertically
// for direct end-labels — the signal to fall back to the legend alone. It is the
// method's "when end-labels collide, don't stack them" rule.
func endsCollide(series []Series, p *plot) bool {
	var ys []float64
	for _, s := range series {
		if len(s.XY) == 0 {
			continue
		}
		ys = append(ys, p.sy(s.XY[len(s.XY)-1].Y))
	}
	for i := 0; i < len(ys); i++ {
		for j := i + 1; j < len(ys); j++ {
			if math.Abs(ys[i]-ys[j]) < fontLabel+2 {
				return true
			}
		}
	}
	return false
}
