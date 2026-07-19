package chart

import (
	"math"
	"sort"
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
	labelEnds  bool    // identity via direct end-labels (set by newPlot)
	showLegend bool    // identity via a top legend row (the collision fallback)
	dotKey     bool    // legend key is a dot (scatter) rather than a line+dot
	yLabelW    float64 // widest y-tick label width in px (for y-title centering)
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
	p.yLabelW = maxYLabel
	left := 18.0 + maxYLabel + 8.0
	if opts.YLabel != "" {
		left += fontAxis + 12 // rotated y-title column, held off the edge
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
	if named >= 2 && canDirectLabel {
		p.labelEnds = true
		// Reserve the right gutter for the widest label plus the leader offset
		// (8 gutter + 2 pad, matching directLabels).
		maxEnd := 0.0
		for _, s := range series {
			if s.Name != "" {
				maxEnd = math.Max(maxEnd, textW(s.Name, fontLabel))
			}
		}
		p.r -= maxEnd + 14
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
		// Center the rotated title in the gutter between the left edge and the
		// tick numbers (which end at p.l-8 and run yLabelW leftward), then bias it
		// ~15% toward the chart so it doesn't hug the edge.
		gutterR := p.l - 8 - p.yLabelW
		yx := gutterR * 0.58
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle" transform="rotate(-90 %.1f %.1f)">%s</text>`,
			yx, yc, fontAxis, yx, yc, esc(p.opts.YLabel))
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

// endLabel is one series' direct label, pinned to its line-end.
type endLabel struct {
	seriesIdx int
	name      string
	anchorY   float64 // the line-end pixel y (where the leader starts)
	labelY    float64 // the label's pixel y after de-collision (leader ends here)
}

// directLabels places every series' name in the reserved right-hand gutter,
// spread vertically so no two collide, with a thin leader line from each
// line-end dot to its label. This is the method's leader-line rule: labels never
// sit on top of the lines (so convergence stops mattering), and a nudged label
// stays attached to its series by the connector. Marks carry the series color;
// the label text stays in the secondary-ink token (a light hue is illegible as
// text), with identity coming from the leader + end-dot beside it.
//
// ends is the final point of each series, indexed alongside series; a series with
// no points or no name is skipped.
func (p *plot) directLabels(series []Series, ends []Pt) {
	labels := make([]endLabel, 0, len(series))
	for i, s := range series {
		if s.Name == "" || len(s.XY) == 0 {
			continue
		}
		y := p.sy(ends[i].Y)
		labels = append(labels, endLabel{seriesIdx: i, name: s.Name, anchorY: y, labelY: y})
	}
	if len(labels) == 0 {
		return
	}
	spreadLabels(labels, fontLabel+3, p.t, p.b)

	c := p.c
	gx := p.r + 8 // gutter x: just right of the plot rect
	for _, lb := range labels {
		cls := c.use(lb.seriesIdx)
		// Leader: a thin connector from the line-end (at p.r) to the label, in the
		// series color at low opacity so it recedes. Only drawn when the label was
		// nudged enough to need it; a near-aligned label reads fine without one.
		if math.Abs(lb.labelY-lb.anchorY) > 1.5 {
			c.rawf(`<path class="%s" d="M%.1f %.1f L%.1f %.1f L%.1f %.1f" fill="none" stroke="currentColor" stroke-width="1" stroke-opacity="0.45"/>`,
				cls, p.r, lb.anchorY, gx-2, lb.labelY, gx, lb.labelY)
		}
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" dominant-baseline="middle">%s</text>`,
			gx+2, lb.labelY, fontLabel, esc(lb.name))
	}
}

// spreadLabels pushes a set of labels apart so consecutive ones are at least gap
// px apart, staying within [lo, hi]. It sorts by anchor, then does a two-pass
// relaxation (down then up) — the standard 1-D label-declutter — so labels end up
// in anchor order with minimal total displacement from their line-ends.
func spreadLabels(labels []endLabel, gap, lo, hi float64) {
	sort.Slice(labels, func(i, j int) bool { return labels[i].anchorY < labels[j].anchorY })
	// Downward pass: enforce spacing top-to-bottom.
	for i := 1; i < len(labels); i++ {
		if labels[i].labelY < labels[i-1].labelY+gap {
			labels[i].labelY = labels[i-1].labelY + gap
		}
	}
	// If we ran past the bottom, push the tail back up.
	if n := len(labels); n > 0 && labels[n-1].labelY > hi {
		labels[n-1].labelY = hi
		for i := n - 2; i >= 0; i-- {
			if labels[i].labelY > labels[i+1].labelY-gap {
				labels[i].labelY = labels[i+1].labelY - gap
			}
		}
	}
	// Clamp the top.
	if len(labels) > 0 && labels[0].labelY < lo {
		labels[0].labelY = lo
		for i := 1; i < len(labels); i++ {
			if labels[i].labelY < labels[i-1].labelY+gap {
				labels[i].labelY = labels[i-1].labelY + gap
			}
		}
	}
}
