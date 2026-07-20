//go:notebook
//
// file-and-artifact — where a notebook's data comes in, and where its results go
// out. Both boundaries live OUTSIDE the pure cells.
//
// A cell is a pure function: it cannot open a file or write one, because I/O is
// exactly what breaks the "output is a function of the inputs" contract the graph
// relies on. So this recipe shows the two seams where a pure graph meets the disk,
// and how each keeps the cells pure:
//
//   - IN, at compile time. `go:embed` folds downloads.csv into the binary as
//     bytes. By the time `rows` runs, there is no file to open — the cell sees a
//     value, so it stays pure AND WASM-able (it runs in the browser, where there
//     is no disk). Contrast csv-native, which opens the file by PATH at RUN time:
//     that is real I/O, so it is native-only and its parse cell returns an error.
//     A path is a name to resolve later; embedded bytes are a value you already
//     hold. The graph edge should carry the value, not the handle.
//
//   - OUT, in a separate program. Writing the chart to a .svg file is not a cell —
//     it is a headless run's job, done by ./report, an ordinary `package main`
//     that IMPORTS this notebook and calls its cells like the plain Go functions
//     they are. The impurity (os.WriteFile) lives there, in the writer, never in a
//     cell. This is the "a notebook is an ordinary Go package" claim made literal:
//     `chart := Chart(Rows())` is the whole pipeline, callable from any Go code.
//
// The cells are Exported (Rows, Chart, …) precisely so ./report can import them;
// the RESULT names — the lowercase edges (`sales`, `bars`) — are unchanged, so the
// graph the notebook UI draws is identical. Function name and edge name are
// separate things (see docs/names.html).
//
//	go tool notebook run ./examples/minimal/file-and-artifact      # interactive, in a browser
//	go run ./examples/minimal/file-and-artifact/report             # write downloads.svg (headless)
//
// Demonstrates: go:embed input (compile-time, WASM-able), cells as exported plain
// functions, and writing an artifact from a program outside the graph.

package fileandartifact

import (
	_ "embed"
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// The dataset, embedded at COMPILE TIME. go:embed bakes downloads.csv's bytes into
// the binary, so reading `downloadsCSV` at run time is a string read, not a file
// open — no disk, hence the parse cell below stays pure and WASM-able. (csv-native
// reads this same shape with os.Open at run time and is native-only for it.)
//
//go:embed downloads.csv
var downloadsCSV string

// Rows parses the embedded bytes into typed points. strings.Split, not
// encoding/csv: the latter reaches os and would make this cell non-portable, and
// there is no file handle to hand it anyway — the bytes are already here. One
// parse cell, so everything downstream receives [Point], not raw text.
func Rows() (sales []Point) {
	lines := strings.Split(strings.TrimSpace(downloadsCSV), "\n")
	for _, line := range lines[1:] { // skip the header
		rec := strings.Split(line, ",")
		if len(rec) < 2 {
			continue
		}
		n, _ := strconv.Atoi(strings.TrimSpace(rec[1]))
		sales = append(sales, Point{Month: rec[0], Downloads: n})
	}
	return sales
}

// Chart draws monthly downloads as bars. Pure: a function of the parsed rows and
// nothing else — the same input always draws the same picture, which is why
// ./report can call it headless and get a stable file. The BarChart it returns has
// a Render method, so the notebook UI paints it and the writer can ask it for its
// SVG bytes; same value, two consumers.
func Chart(sales []Point) (bars chart.BarChart) {
	cats := make([]string, len(sales))
	vals := make([]float64, len(sales))
	for i, s := range sales {
		cats[i] = s.Month
		vals[i] = float64(s.Downloads)
	}
	return chart.BarWith(chart.Opts{Title: "Monthly downloads", YLabel: "count"},
		cats, chart.Series2{Name: "downloads", Values: vals})
}

// Total is a second downstream cell — the headline number, computed from the same
// rows. Having two consumers of `sales` makes the point that ./report can pick any
// cell's output to materialize, not just the chart.
func Total(sales []Point) (readout Readout) {
	var t int
	for _, s := range sales {
		t += s.Downloads
	}
	// strconv (not fmt) keeps this cell body on the WASM-able path.
	return Readout{Label: "downloads this year", Value: strconv.Itoa(t)}
}

// Point is one parsed row: a month and its download count.
type Point struct {
	Month     string
	Downloads int
}

// Readout is a single stat card. A locally-declared Render (returning the
// MIME/Data envelope by field shape) keeps this file zero-import beyond nb/chart.
type Readout struct{ Label, Value string }

func (r Readout) Render() Rendered {
	return Rendered{MIME: "text/html", Data: `<div style="font-family:system-ui,sans-serif">` +
		`<div style="font-size:12px;color:#5b6472">` + r.Label + `</div>` +
		`<div style="font:600 22px/1.2 system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">` + r.Value + `</div>` +
		`</div>`}
}

// Rendered is the display envelope the engine reads by field shape — the
// zero-import twin of nb.Rendered.
type Rendered struct{ MIME, Data string }
