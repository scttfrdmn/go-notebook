//go:notebook
//
// multi-file-package — a notebook is an ordinary Go package, and can span as
// many .go files as any package does.
//
// The other recipes are one file each, which quietly implies a notebook must be
// one file. It doesn't. Only ONE file carries the //go:notebook marker (this
// one), and that is the only file the analyzer scans FOR CELLS. But the Go type
// checker sees the whole package, so the cells here freely use types, methods,
// and helpers defined in the sibling files:
//
//   - model.go   — the domain type Book and its methods (Decade, Long)
//   - data.go    — catalog(), the loader/parser
//   - nb.go      — THIS file: the cells (the //go:notebook file)
//
// A cell below names a []Book edge, calls b.Decade() and b.Long() (methods from
// model.go), and calls catalog() (a helper from data.go). None of that needs an
// import or a re-declaration, because it is all one package. Splitting a notebook
// across files is not a special feature — it is just what Go packages do, and
// notebooks inherit it for free.
//
// The payoff for real projects: your model and your parsing live in normal,
// separately-testable files, and the notebook file stays small — just the cells
// that turn that package into a reactive view. See the multi-file discovery rule
// (and the deliberate named-result gotcha) in data.go.
//
//	go tool notebook run ./examples/minimal/multi-file-package     # interactive, in a browser
//	go tool notebook check ./examples/minimal/multi-file-package   # print the graph — cells only, from THIS file
//
// Demonstrates: a notebook spanning several .go files; cells using types,
// methods, and helpers from sibling non-notebook files; the discovery/type-check
// split (only the marked file is scanned for cells, the whole package is typed).
//
//notebook:layout intro
//notebook:layout summary | byDecade
//notebook:layout longReads

package multifilepackage

import (
	"sort"
	"strconv"

	"github.com/scttfrdmn/go-notebook/nb"
	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// A minimum page count — drag it and the "long reads" table and the summary
// recompute. The predicate itself is Book.Long, defined in model.go.
//
//notebook:slider min=0 max=1000 step=20
func minPages() (floor int) { return 400 }

// loadCatalog is a cell that simply calls catalog(), the parser in data.go. This
// is the cross-file wiring made visible: the cell (discovered here) produces the
// `books` edge from a helper defined in another file (not discovered, just
// type-checked as part of the package).
func loadCatalog() (books []Book) { return catalog() }

// longReads keeps the books at or above the page floor, using Book.Long — a
// method defined in model.go. The cell reaches across the file boundary for both
// the type (Book) and its method; the type checker resolves them package-wide.
func longReads(books []Book, floor int) (table chart.Table) {
	var kept []Book
	for _, b := range books {
		if b.Long(floor) {
			kept = append(kept, b)
		}
	}
	return chart.RowsWith(chart.Opts{Title: "Long reads"}, kept)
}

// byDecade counts titles per decade, using Book.Decade (model.go) to bucket
// them. Grouped bars — the "group by" you'd reach a dataframe for, written as a
// plain map accumulation over a slice of the domain type.
func byDecade(books []Book) (bars chart.BarChart) {
	count := map[string]int{}
	for _, b := range books {
		count[b.Decade()]++ // Decade() lives in model.go
	}
	decades := make([]string, 0, len(count))
	for d := range count {
		decades = append(decades, d)
	}
	sort.Strings(decades)
	vals := make([]float64, len(decades))
	for i, d := range decades {
		vals[i] = float64(count[d])
	}
	return chart.BarWith(chart.Opts{Title: "Titles by decade", YLabel: "count"},
		decades, chart.Series2{Name: "titles", Values: vals})
}

// summary is the headline card: how many of the whole catalog are long reads at
// the current floor. Computed from both edges — the full catalog and the floor —
// so it reruns on either change.
func summary(books []Book, floor int) (readout Readout) {
	var long int
	for _, b := range books {
		if b.Long(floor) {
			long++
		}
	}
	return Readout{
		Label: "long reads (≥ " + strconv.Itoa(floor) + " pages)",
		Value: strconv.Itoa(long) + " of " + strconv.Itoa(len(books)),
	}
}

// intro is the prose cell. Returns a Markdown value (a type with a Render method,
// declared below) so the engine draws it.
func intro() (md Markdown) {
	return `**One notebook, three files.** The cells live here in ` + "`nb.go`" + `;
the ` + "`Book`" + ` type and its methods live in ` + "`model.go`" + `; the parser
lives in ` + "`data.go`" + `. Only this file carries ` + "`//go:notebook`" + `, so
only this file is scanned for cells — but the whole package is type-checked, so
the cells use everything in the sibling files with no import and no wiring.

Drag **min pages**: ` + "`longReads`" + ` and ` + "`summary`" + ` recompute,
because each is a function of the ` + "`books`" + ` edge and the floor.`
}

// ---------------------------------------------------------------------------
// View types (local to this file — the notebook owns its presentation)
// ---------------------------------------------------------------------------

// Markdown is a prose cell: the engine converts it to a safe HTML subset at its
// single render chokepoint.
type Markdown string

func (m Markdown) Render() nb.Rendered { return nb.Markdown(string(m)) }

// Readout is a single stat card, rendered as a label/value pair.
type Readout struct{ Label, Value string }

func (r Readout) Render() nb.Rendered {
	return nb.HTML(`<div style="font-family:system-ui,-apple-system,sans-serif">` +
		`<div style="font-size:12px;color:#5b6472">` + r.Label + `</div>` +
		`<div style="font:600 22px/1.2 system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">` + r.Value + `</div>` +
		`</div>`)
}

// Compile-time checks that the view types really are renderable (a misspelled
// Render becomes a build error, not a silently-blank cell).
var (
	_ nb.Renderable = Markdown("")
	_ nb.Renderable = Readout{}
)
