// Package chart is the optional dataviz sibling of nb: five chart forms and a
// handful of summary statistics, drawn well, for go-notebook notebooks.
//
// It exists because the raw SVG escape hatch — hand-writing <path> and <circle>
// math in a cell's Render — is a cliff. Most analysis needs the same 1% of a
// plotting library: a line, a scatter, some bars, a histogram, a table, and the
// five numbers that summarize a column. This package does that 1% with the craft
// the hand-rolled corpus SVGs skip: 1-2-5 "nice" axis ticks, a recessive hairline
// grid, a legend for two-or-more series with selective direct labels, and a
// colorblind-safe palette that is validated on both a light and a dark surface.
//
// # The boundary
//
// chart draws five forms well and will never grow a sixth axis, subplots,
// secondary y-axes, custom themes, animation, or a legend DSL. It is the 1% done
// excellently, not a plotting library. When you need more: the raw HTML/SVG
// Render() escape hatch (always there) or import gonum/plot. The toolchain never
// depends on this package — delete nb/chart and no notebook changes its answer,
// only its convenience.
//
// # Using it
//
// Every form has a bare constructor for the common case and a *With variant that
// takes a flat [Opts] when you need a title, axis labels, a log scale, or a
// height:
//
//	func revenue() (v chart.LineChart) {
//		return chart.Line(
//			chart.Series{Name: "2024", XY: q1},
//			chart.Series{Name: "2025", XY: q2},
//		)
//	}
//
//	func revenue() (v chart.LineChart) {
//		return chart.LineWith(
//			chart.Opts{Title: "Revenue", YLabel: "$M", YLog: true},
//			chart.Series{Name: "2024", XY: q1},
//		)
//	}
//
// Each form is a value type whose Render method returns [nb.Rendered] tagged
// image/svg+xml (or text/html for [Table]), so it rides the engine's existing
// render path exactly like any hand-rolled view.
//
// # Portability
//
// Like nb, the constructors here are called from a Render method, not from a cell
// body, so the fmt→os WASM gate never sees them. A cell returns the chart value;
// the engine calls Render. Keep it that way.
package chart

// Pt is one (x, y) datum. It is the shared vocabulary of the point-based forms
// ([Line], [Scatter]); the categorical forms take their own shapes.
type Pt struct{ X, Y float64 }

// Series is a named sequence of points. The Name drives the legend and the
// selective direct label at the series' end; XY is the data, taken in order for a
// line and as a cloud for a scatter.
type Series struct {
	Name string
	XY   []Pt
}

// Opts is the flat, shared option set — every field optional, zero value sane.
// It is deliberately small and non-extensible: no builder, no functional
// options, nothing that grows into a configuration language. When a chart needs
// something not expressible here, that is the signal to reach for the raw SVG
// escape hatch, not to add a field.
type Opts struct {
	// Title is drawn above the plot; empty draws nothing.
	Title string
	// XLabel and YLabel name the axes; empty draws nothing.
	XLabel, YLabel string
	// YLog puts the y-axis on a base-10 log scale (values must be positive).
	YLog bool
	// Height is the drawing height in px; 0 uses a per-form default.
	Height int
	// Fit draws a least-squares trend line through a [Scatter]'s points (via
	// [LinFit]) — the one sanctioned overlay, because it annotates the same data
	// on the same axes rather than adding a form. Ignored by the other charts.
	Fit bool
}

// height returns the configured height or the given default.
func (o Opts) height(def int) int {
	if o.Height > 0 {
		return o.Height
	}
	return def
}
