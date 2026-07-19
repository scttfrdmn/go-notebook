package chart

import "math"

// renderHorizontal draws bars as rows growing right from a left baseline — the
// orientation for long category labels, which read straight instead of rotated.
// Grouped and stacked both supported, same mark specs as the vertical form.
func (b BarChart) renderHorizontal(c *canvas) {
	_, hi := b.valueExtent()
	nlo, nhi, ticks := niceScale(0, hi, 5)

	// Reserve the left for the widest category label.
	maxCat := 0.0
	for _, cat := range b.cats {
		maxCat = math.Max(maxCat, textW(cat, fontTick))
	}
	left := 14.0 + maxCat + 10.0
	top := padTop
	if b.opts.Title != "" {
		top += fontTitle + 8
	}
	if b.namedGroups() >= 2 {
		top += fontLabel + 10
	}
	bottom := float64(c.h) - (fontTick + 14)
	if b.opts.XLabel != "" {
		bottom -= fontAxis + 2
	}
	l, r := left, float64(c.w)-padRight

	vx := scale{lo: nlo, hi: nhi, p0: l, p1: r}

	// Vertical gridlines + value-tick labels along the bottom.
	for _, v := range ticks {
		x := vx.at(v)
		c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--grid)" stroke-width="1"/>`, x, top, x, bottom)
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="middle" style="font-variant-numeric:tabular-nums">%s</text>`,
			x, bottom+fontTick+8, fontTick, esc(fmtNum(v)))
	}
	// The value baseline is the left edge (x = nlo).
	c.rawf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--axis)" stroke-width="1"/>`, l, top, l, bottom)

	n := len(b.cats)
	if n == 0 {
		n = 1
	}
	bandH := (bottom - top) / float64(n)
	for ci, cat := range b.cats {
		cy := top + (float64(ci)+0.5)*bandH
		b.drawRow(c, ci, cy, bandH, l, vx)
		// Category label, right-aligned in the left gutter, vertically centered.
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--muted)" text-anchor="end" dominant-baseline="middle">%s</text>`,
			l-8, cy, fontTick, esc(cat))
	}

	b.drawChromeH(c, l, r, top, bottom)
}

// drawRow draws all series' bars for one category centered at cy.
func (b BarChart) drawRow(c *canvas, ci int, cy, bandH, baseline float64, vx scale) {
	const gap = 2.0
	if b.stack {
		x := baseline
		thick := math.Min(maxBarThick, bandH*0.7)
		for si, s := range b.groups {
			v := valAt(s, ci)
			if v <= 0 {
				continue
			}
			w := vx.at(v) - baseline
			b.drawBarH(c, si, x, cy-thick/2, w-gap, thick, si == len(b.groups)-1)
			x += w
		}
		return
	}
	k := len(b.groups)
	if k == 0 {
		return
	}
	slot := math.Min(maxBarThick, (bandH*0.8)/float64(k))
	groupH := slot*float64(k) + gap*float64(k-1)
	y0 := cy - groupH/2
	for si, s := range b.groups {
		v := valAt(s, ci)
		y := y0 + float64(si)*(slot+gap)
		w := vx.at(v) - baseline
		b.drawBarH(c, si, baseline, y, w, slot, true)
	}
}

// drawBarH emits one horizontal bar; roundEnd rounds the right (data) end while
// the left (baseline) stays square.
func (b BarChart) drawBarH(c *canvas, si int, x, y, w, h float64, roundEnd bool) {
	if w <= 0.5 || h <= 0 {
		return
	}
	cls := c.use(si)
	const rad = 4.0
	if !roundEnd || w < rad*2 || h < rad*2 {
		c.rawf(`<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="currentColor"/>`,
			cls, x, y, w, h)
		return
	}
	// Square left, rounded right end.
	c.rawf(`<path class="%s" fill="currentColor" d="M%.1f %.1f h%.1f a%.0f %.0f 0 0 1 %.0f %.0f v%.1f a%.0f %.0f 0 0 1 %.0f %.0f h%.1f z"/>`,
		cls,
		x, y, // top-left
		w-rad,              // across the top
		rad, rad, rad, rad, // top-right arc
		h-2*rad,             // down the right side
		rad, rad, -rad, rad, // bottom-right arc
		-(w - rad), // back across the bottom
	)
}

// drawChromeH draws title, axis title, and legend for the horizontal form. The
// title sits at the left page margin (padRight) rather than at the plot's left
// edge, which is pushed far right by the category-label gutter; there is no
// y-axis title in the horizontal form (the categories are the y-axis).
func (b BarChart) drawChromeH(c *canvas, l, r, top, bottom float64) {
	if b.opts.Title != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" font-weight="600" fill="var(--ink)">%s</text>`,
			padRight, padTop+fontTitle*0.5, fontTitle, esc(b.opts.Title))
	}
	if b.opts.XLabel != "" {
		c.rawf(`<text x="%.1f" y="%.1f" font-size="%.0f" fill="var(--secondary)" text-anchor="middle">%s</text>`,
			(l+r)/2, float64(c.h)-6, fontAxis, esc(b.opts.XLabel))
	}
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
