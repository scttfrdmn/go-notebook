package chart

import "math"

// This file draws the bar chart's two orientations. Both share the value-axis
// niceScale + recessive grid with the point-based forms, but lay the category
// axis out as bands rather than a continuous scale — a bar chart's x is discrete,
// so it gets its own framing. The mark specs are the method's: bars capped at
// maxBarThick (never fill the slot — leftover band is air), a 4px rounded
// data-end with a square baseline, and a 2px surface gap between touching bars
// (grouped neighbors and stacked segments alike), the gap doing the separating so
// no border is drawn around a fill.

// renderVertical draws columns growing up from a bottom baseline.
func (b BarChart) renderVertical(c *canvas) {
	_, hi := b.valueExtent()
	nlo, nhi, ticks := niceScale(0, hi, 5)

	// Margins. Reserve the left for the widest value-tick label, the bottom for
	// category labels + optional axis title, the top for title + legend.
	maxYLabel := 0.0
	for _, v := range ticks {
		maxYLabel = math.Max(maxYLabel, textW(fmtNum(v), fontTick))
	}
	left := 18.0 + maxYLabel + 8.0
	if b.opts.YLabel != "" {
		left += fontAxis + 12
	}
	top := padTop
	if b.opts.Title != "" {
		top += fontTitle + 8
	}
	if b.namedGroups() >= 2 {
		top += fontLabel + 10
	}
	bottom := float64(c.h) - (fontTick + 16)
	if b.opts.XLabel != "" {
		bottom -= fontAxis + 2
	}
	l, r := left, float64(c.w)-padRight

	vy := scale{lo: nlo, hi: nhi, p0: bottom, p1: top}

	// Grid + value-tick labels.
	for _, v := range ticks {
		y := vy.at(v)
		c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--grid)" stroke-width="1"/>`, l, y, r, y)
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="end" dominant-baseline="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			l-6, y, fontTick, esc(fmtNum(v)))
	}
	c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--axis)" stroke-width="1"/>`, l, bottom, r, bottom)

	// Category bands.
	n := len(b.cats)
	if n == 0 {
		n = 1
	}
	bandW := (r - l) / float64(n)
	for ci, cat := range b.cats {
		cx := l + (float64(ci)+0.5)*bandW
		b.drawColumn(c, ci, cx, bandW, bottom, vy)
		// Category label, centered under the band.
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="middle">%s</text>`,
			cx, bottom+fontTick+8, fontTick, esc(cat))
	}

	b.drawChrome(c, l, r, top, bottom, maxYLabel)
}

// drawColumn draws all series' bars for one category at center cx.
func (b BarChart) drawColumn(c *canvas, ci int, cx, bandW, baseline float64, vy scale) {
	const gap = 2.0 // surface gap between touching bars/segments
	if b.stack {
		// Stack segments bottom-up, each separated by a surface gap.
		y := baseline
		for si, s := range b.groups {
			v := valAt(s, ci)
			if v <= 0 {
				continue
			}
			h := baseline - vy.at(v) // pixel height of this segment
			top := y - h
			thick := math.Min(maxBarThick, bandW*0.7)
			b.drawBar(c, si, cx-thick/2, top, thick, y-top-gap, si == len(b.groups)-1)
			y = top
		}
		return
	}
	// Grouped: k side-by-side bars sharing the band, 2px gaps between them.
	k := len(b.groups)
	if k == 0 {
		return
	}
	slot := math.Min(maxBarThick, (bandW*0.8)/float64(k))
	groupW := slot*float64(k) + gap*float64(k-1)
	x0 := cx - groupW/2
	for si, s := range b.groups {
		v := valAt(s, ci)
		x := x0 + float64(si)*(slot+gap)
		top := vy.at(v)
		b.drawBar(c, si, x, top, slot, baseline-top, true)
	}
}

// drawBar emits one bar rect. roundTop gives the data-end a 4px radius while the
// baseline stays square (the method's "4px rounded data-end, square at the
// baseline"); a stacked interior segment passes roundTop=false so only the top
// cap of the stack is rounded. h<=0 draws nothing.
func (b BarChart) drawBar(c *canvas, si int, x, y, w, h float64, roundTop bool) {
	if h <= 0.5 || w <= 0 {
		return
	}
	cls := c.use(si)
	if roundTop {
		c.roundedTopBar(cls, x, y, w, h)
		return
	}
	// An interior stacked segment: square both ends (only the stack's top cap is
	// rounded, by the last segment's roundTop=true).
	c.rawf(`<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="currentColor"/>`,
		cls, x, y, w, h)
}

// drawChrome draws the title, axis titles, and legend shared by both orientations.
func (b BarChart) drawChrome(c *canvas, l, r, top, bottom, maxYLabel float64) {
	c.axisChrome(b.opts, l, r, top, bottom, maxYLabel)
	if b.namedGroups() >= 2 {
		var names []string
		var idx []int
		for i, s := range b.groups {
			if s.Name != "" {
				names = append(names, s.Name)
				idx = append(idx, i)
			}
		}
		ly := padTop + fontLabel*0.5 + 2
		if b.opts.Title != "" {
			ly += fontTitle + 8
		}
		c.swatchLegend(l, ly, names, idx)
	}
}
