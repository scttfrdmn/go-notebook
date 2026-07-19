//go:notebook
//
// csv — normal analysis: read a CSV, filter it, summarize it, chart it.
//
// This is the shape most data work actually takes, and the one the other
// examples skip: parse rows, drop the ones you don't want, compute a few
// numbers, and look at the result as a table and a picture. No dataframe, no
// query language — the standard library and the optional nb/chart package.
//
// The CSV lives in a string constant (not a file) and is split with strings, so
// the notebook is WASM-able and runs in the browser — strings touches neither the
// filesystem nor the network. A note on the stdlib encoding/csv: it is the right
// parser for real quoted/escaped CSV, but it transitively reaches os (through
// fmt), so a cell that imports it is NOT browser-portable and the WASM gate will
// refuse it. Use encoding/csv in a native/headless run (where you'd os.Open a
// real file anyway); for the simple unquoted data here, a strings.Split parser is
// both enough and portable. That tradeoff is the point, and it is visible in what
// each cell is allowed to import.
//
//	go tool notebook run ./examples/minimal/csv
//
// Demonstrates: CSV parsing + filtering in a cell, nb/chart stats + Table + Bar,
// and the WASM portability line. See https://go-notebook.dev/docs/reference-charts.html.
//
//notebook:layout intro
//notebook:layout summary | byRegion
//notebook:layout rowsTable

package csv

import (
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb"
	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// The dataset, inlined so the notebook runs anywhere. Quarterly sales rows:
// region, quarter, units, revenue. In a native run this would be os.Open(path)
// piped to a parser.
const salesCSV = `region,quarter,units,revenue
North,Q1,1200,54000
North,Q2,1350,60750
South,Q1,890,40050
South,Q2,1020,45900
East,Q1,1580,71100
East,Q2,1720,77400
West,Q1,640,28800
West,Q2,710,31950
North,Q3,1410,63450
South,Q3,1130,50850
East,Q3,1810,81450
West,Q3,780,35100`

// ---------------------------------------------------------------------------
// Cells
// ---------------------------------------------------------------------------

// A minimum revenue to keep — drag it and watch the table, the summary, and the
// bars all recompute. The filter is just a Go `if`; there is no query language.
//
//notebook:slider min=0 max=60000 step=5000
func minRevenue() (floor float64) { return 0 }

// Parse the CSV and keep the rows at or above the floor. Split on newlines and
// commas with strings — pure and WASM-able, no file and no socket (see the note
// at the top on why encoding/csv would not be). Parsing lives in one cell so
// everything downstream receives typed [Sale] rows, not raw strings.
func rows(floor float64) (sales []Sale) {
	lines := strings.Split(strings.TrimSpace(salesCSV), "\n")
	for _, line := range lines[1:] { // skip the header
		rec := strings.Split(line, ",")
		if len(rec) < 4 {
			continue
		}
		units, _ := strconv.Atoi(strings.TrimSpace(rec[2]))
		rev, _ := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		if rev < floor {
			continue
		}
		sales = append(sales, Sale{
			Region:  rec[0],
			Quarter: rec[1],
			Units:   units,
			Revenue: rev,
		})
	}
	return sales
}

// The kept rows, as a table. chart.Rows reflects over the []Sale: field names
// become headers ("Revenue"), numeric columns right-align with tabular figures.
func rowsTable(sales []Sale) (table chart.Table) {
	return chart.RowsWith(chart.Opts{Title: "Filtered rows"}, sales)
}

// Summary statistics over the kept revenues — the numbers a report leads with,
// computed with nb/chart's stats functions (pure Go, no dependency on the chart
// rendering). Mean, spread, median, and the min/max as quantiles.
func summary(sales []Sale) (stats Readout) {
	rev := make([]float64, len(sales))
	for i, s := range sales {
		rev[i] = s.Revenue
	}
	// strconv (not fmt) keeps this cell body on the WASM-able path.
	money := func(v float64) string { return "$" + strconv.FormatFloat(v, 'f', 0, 64) }
	return Readout{Cards: []Card{
		{Label: "rows kept", Value: strconv.Itoa(len(sales))},
		{Label: "total revenue", Value: money(sum(rev))},
		{Label: "mean revenue", Value: money(chart.Mean(rev))},
		{Label: "median revenue", Value: money(chart.Quantile(rev, 0.5))},
		{Label: "std dev", Value: money(chart.Std(rev)), Caption: "population"},
	}}
}

// Revenue by region, as grouped bars — one bar per region, summed across the
// quarters that survived the filter. This is the "group by" you'd reach a
// dataframe for, written as a plain map accumulation.
func byRegion(sales []Sale) (bars chart.BarChart) {
	order := []string{"North", "South", "East", "West"}
	total := map[string]float64{}
	for _, s := range sales {
		total[s.Region] += s.Revenue
	}
	// Keep only regions that still have rows, preserving a stable order.
	var cats []string
	var vals []float64
	for _, region := range order {
		if _, ok := total[region]; ok {
			cats = append(cats, region)
			vals = append(vals, total[region])
		}
	}
	return chart.BarWith(chart.Opts{Title: "Revenue by region", YLabel: "$"},
		cats, chart.Series2{Name: "Revenue", Values: vals})
}

// What this notebook is. Returns a Markdown value (a type with a Render method) —
// a cell's result is drawn only if it is renderable, and a bare nb.Rendered is
// not (nb.HTML/nb.Markdown are meant to be called inside a Render, not returned
// as the cell result).
func intro() (md Markdown) {
	return `**Normal analysis, no dataframe.** A CSV in a string, parsed with the
standard library (` + "`strings.Split`" + ` — see the source note on ` + "`encoding/csv`" + `
and the browser), filtered with a plain ` + "`if`" + `, summarized and charted with
the optional ` + "`nb/chart`" + ` package.

Drag **min revenue** above: the parse-and-filter cell reruns, and the table, the
summary, and the bars downstream of it all recompute — because each is a function
of the filtered rows, wired by name and type like every other edge.`
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Sale is one parsed row. The field names double as the table's column headers.
type Sale struct {
	Region  string
	Quarter string
	Units   int
	Revenue float64
}

func sum(xs []float64) float64 {
	var t float64
	for _, x := range xs {
		t += x
	}
	return t
}

// Markdown is a prose cell: the engine converts it to a safe HTML subset at its
// single render chokepoint. A type-with-Render, so the cell result is drawn.
type Markdown string

func (m Markdown) Render() nb.Rendered { return nb.Markdown(string(m)) }

// Readout is a small stat-card list, rendered as a vertical label/value stack.
// (Local view type — the notebook owns its presentation; nb/chart draws data, not
// bespoke readouts.)
type Readout struct{ Cards []Card }

type Card struct{ Label, Value, Caption string }

func (r Readout) Render() nb.Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;flex-direction:column;gap:.6rem;font-family:system-ui,-apple-system,sans-serif">`)
	for _, c := range r.Cards {
		b.WriteString(`<div>`)
		b.WriteString(`<div style="font-size:12px;color:#5b6472">` + c.Label + `</div>`)
		b.WriteString(`<div style="font:600 20px/1.2 system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">` + c.Value + `</div>`)
		if c.Caption != "" {
			b.WriteString(`<div style="font-size:11px;color:#5b6472">` + c.Caption + `</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return nb.HTML(b.String())
}
