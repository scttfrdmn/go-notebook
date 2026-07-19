package chart

import (
	"math"

	"github.com/scttfrdmn/go-notebook/nb"
)

// BarChart is a categorical bar chart: one bar per category, or grouped/stacked
// bars when there are multiple series. Build it with [Bar] / [BarWith]. Bars are
// vertical columns by default; set [Opts].Height and pass many categories and it
// stays vertical — horizontal bars are chosen automatically when labels are long
// (see the Horizontal note on [Bars]).
type BarChart struct {
	cats   []string
	groups []Series2 // one per series; each has a value per category
	opts   Opts
	stack  bool
	horiz  bool
}

// Series2 is a named series of scalar values aligned to a bar chart's categories:
// values[i] is the height of this series' bar in category i. (Distinct from
// [Series], which is (x,y) points for line/scatter.)
type Series2 struct {
	Name   string
	Values []float64
}

// Bar draws a simple or grouped bar chart: categories on the x-axis, one bar per
// category per series. A single series needs no legend; two or more get one.
func Bar(cats []string, series ...Series2) BarChart {
	return BarChart{cats: cats, groups: series}
}

// BarWith is [Bar] with options.
func BarWith(opts Opts, cats []string, series ...Series2) BarChart {
	return BarChart{cats: cats, groups: series, opts: opts}
}

// Stacked draws the series stacked within each category (part-to-whole) rather
// than side by side. Segments are separated by a 2px surface gap.
func (b BarChart) Stacked() BarChart { b.stack = true; return b }

// Horizontal lays the bars out as horizontal rows — the right choice when
// category labels are long, since a vertical column can't carry a long label
// without rotating it.
func (b BarChart) Horizontal() BarChart { b.horiz = true; return b }

const maxBarThick = 24.0 // the method's cap: never fill the slot, leave air

// Render draws the bar chart.
func (b BarChart) Render() nb.Rendered {
	const defW = 720
	defH := 380
	if b.horiz {
		// Height grows with the category count for horizontal bars.
		defH = 80 + len(b.cats)*38
	}
	c := newCanvas(defW, b.opts.height(defH))
	if b.horiz {
		b.renderHorizontal(c)
	} else {
		b.renderVertical(c)
	}
	return nb.SVG(c.finish())
}

// seriesTotals returns, per category, the sum across series (for stacked) or the
// max single value (for grouped) — the extent the value axis must cover.
func (b BarChart) valueExtent() (lo, hi float64) {
	hi = math.Inf(-1)
	lo = 0 // bars grow from a zero baseline
	for ci := range b.cats {
		if b.stack {
			sum := 0.0
			for _, s := range b.groups {
				sum += valAt(s, ci)
			}
			hi = math.Max(hi, sum)
		} else {
			for _, s := range b.groups {
				hi = math.Max(hi, valAt(s, ci))
				lo = math.Min(lo, valAt(s, ci))
			}
		}
	}
	if math.IsInf(hi, -1) {
		hi = 1
	}
	return lo, hi
}

func valAt(s Series2, i int) float64 {
	if i < len(s.Values) {
		return s.Values[i]
	}
	return 0
}

// namedGroups reports how many series carry a name (drives the legend).
func (b BarChart) namedGroups() int {
	n := 0
	for _, s := range b.groups {
		if s.Name != "" {
			n++
		}
	}
	return n
}
