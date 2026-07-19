# Charts

*The optional `nb/chart` package: five chart forms and a handful of summary statistics, drawn well, so a cell can show a line or a table without hand-writing SVG. It does the 1% of a plotting library most analysis actually needs — and stops there on purpose.*

## Why it exists

A cell draws by returning a value whose `Render()` emits `image/svg+xml` or `text/html` (see [rendering](reference-rendering.html)). That is complete freedom and a cliff: a real chart means hand-writing `<path>` math, axis ticks, and a legend, every time. `nb/chart` is the step before the cliff. Import it and a cell returns a chart value:

```go
import "github.com/scttfrdmn/go-notebook/nb/chart"

func revenue() (v chart.LineChart) {
	return chart.Line(
		chart.Series{Name: "2024", XY: q1},
		chart.Series{Name: "2025", XY: q2},
	)
}
```

The returned value has a `Render()` like any other view, so it rides the same path as a hand-rolled one. **Nothing in the toolchain depends on `nb/chart`** — delete the package and no notebook changes its answer, only its convenience. It is a sibling of the optional [`nb`](reference-rendering.html) package, not part of the engine.

## The five forms

Every form has a bare constructor for the common case and a `*With` variant that takes a flat `Opts` for a title, axis labels, a log scale, or a height.

| Form | Constructor | Draws |
|------|-------------|-------|
| Line | `chart.Line(series…)` | multi-series line plot; each line named at its own end |
| Scatter | `chart.Scatter(series…)` | point clouds; `Opts{Fit:true}` adds a least-squares trend line |
| Bar | `chart.Bar(cats, series…)` | grouped or `.Stacked()`, vertical or `.Horizontal()` |
| Histogram | `chart.Hist(values)` | binned distribution (Sturges' rule, or `.Bins(n)`) |
| Table | `chart.Rows(data)` | a slice of structs / maps / `[][]string` as an HTML table |

`Series` is `{Name string; XY []Pt}` for the point-based forms; `Series2` is `{Name string; Values []float64}` aligned to a bar chart's categories. The `Name` drives the legend and the direct label.

```go
chart.BarWith(chart.Opts{Title: "Revenue by region", YLabel: "$"},
	[]string{"North", "South", "East", "West"},
	chart.Series2{Name: "Revenue", Values: totals})

chart.RowsWith(chart.Opts{Title: "Order lines"}, sales) // []struct → table
```

## The options

`Opts` is flat and small on purpose — every field optional, the zero value sane:

```go
type Opts struct {
	Title          string  // above the plot
	XLabel, YLabel string  // axis titles
	YLog           bool    // base-10 log y-axis
	Height         int     // px; 0 = per-form default
	Fit            bool    // Scatter only: draw a LinFit trend line
}
```

There is no builder and no functional-options API. That is a deliberate limit, not an omission: an option set that is easy to extend is how a focused tool becomes a plotting library.

## The statistics

The other half of the 1% — the numbers a summary leads with, as pure functions over `[]float64` (safe in a cell body or a `Render`):

| Function | Returns |
|----------|---------|
| `chart.Mean(xs)` | arithmetic mean |
| `chart.Std(xs)` | population standard deviation |
| `chart.Quantile(xs, p)` | the *p*-quantile (linear interpolation; `0.5` is the median) |
| `chart.Corr(xs, ys)` | Pearson correlation |
| `chart.LinFit(xs, ys)` | least-squares `(slope, intercept)` |

They pair with the forms: `LinFit` is what `Scatter`'s `Opts{Fit:true}` draws, and `Mean`/`Quantile` fill a summary card beside a `Table`.

## What "drawn well" means

The craft is the value, and it is the part a hand-rolled SVG usually skips:

- **Nice axis ticks** — the 1-2-5 algorithm, so an axis reads `0 / 20 / 40 / 60`, never `0 / 17.3 / 34.6`.
- **A recessive frame** — hairline gridlines one shade off the surface, no boxy border, the data the only loud thing.
- **Direct labels over legends** — a line is named at its own end (with a leader line when ends crowd); a legend appears only where direct labels can't (a scatter's cloud). A single series needs neither.
- **A colorblind-safe palette, baked in** — eight categorical hues in a fixed, validated order (worst-case adjacent separation checked under three types of color-vision deficiency), assigned in order and never cycled; a ninth series folds to "Other".
- **Light and dark** — every chart carries its own themed styling and follows the viewer's `prefers-color-scheme`, validated on both surfaces.

You do not configure any of this. It is the same on every chart, which is what lets a notebook's charts read as one system.

## The boundary

> `nb/chart` draws five forms well and will never grow a sixth axis, subplots, secondary y-axes, custom themes, animation, or a legend DSL. It is the 1% done excellently, not a plotting library. When you need more: the raw HTML/SVG `Render()` escape hatch (always there) or import `gonum/plot`. The toolchain never depends on this package.

This paragraph is the whole safety mechanism. The package is a **product boundary** — depth on a fixed set — not a scope that grows. The trap it avoids is the one every plotting library falls into: one more chart type, one more option, until the surface is unlearnable. There is also, deliberately, **no dataframe or query API** — no filter, group, or join. That is a second, deeper trap (a pandas in miniature). Normal analysis closes the gap with plain Go: parse with the standard library, filter with an `if`, group with a `map`. See the [`minimal/csv`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/csv) example, which does exactly that and charts the result.

## Portability

Like `nb`, the constructors are meant to be called from a `Render` method or a cell that returns the chart value — not to have `fmt`-heavy string-building in the cell body, which the WASM gate forbids (see [build & run](reference-build-run.html)). A cell returns `chart.LineChart{…}`; the engine calls `Render`. Keep it that way and the notebook stays browser-portable.
