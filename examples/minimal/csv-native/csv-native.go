//go:notebook
//
// csv-native — read a real CSV file with os.Open + encoding/csv, and handle the
// errors honestly.
//
// This is the parser you actually reach for: encoding/csv handles quoting,
// escaping, and embedded commas that a strings.Split never will. The price is
// portability — encoding/csv transitively reaches os, so a cell that imports it
// is NOT WASM-able, and this notebook is native-only. (The embedded-data recipe
// is the same analysis kept browser-portable with go:embed + strings; the two
// are the two ends of the same tradeoff.)
//
// The point this recipe makes that its siblings don't: a parse that discards its
// errors is a bug waiting to happen. `rows` returns (sales, error) — a bad number
// in the file FAILS the cell instead of silently coercing to zero, and the graph
// shows the downstream table and summary as "blocked upstream" rather than
// charting a quiet lie. That is the errorcell recipe's lesson, applied to real
// I/O: the same discipline as a (value, error) cell, at the point data enters.
//
// The path is a plain string edge, resolved against the process's working
// directory (the module root under `notebook run`) — not the source file's
// location. A path is a name to look up at run time, not a handle to the bytes.
//
//	go tool notebook run ./examples/minimal/csv-native
//
// Demonstrates: os.Open + encoding/csv, an honest (rows, error) parse cell,
// blocked-upstream propagation from a real file read. Native-only.

package csvnative

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/scttfrdmn/go-notebook/nb"
	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// The file to read, as an editable string. It resolves against the working
// directory — the module root when you `notebook run` this — so the default is
// written relative to there. Point it at a missing file and watch `rows` fail;
// point it at a malformed one and watch it fail differently.
func path() (file string) { return "examples/minimal/csv-native/sales.csv" }

// Open the file and parse it with encoding/csv, returning an error rather than
// swallowing one. This is the cell that makes the recipe: os.Open can fail (no
// such file), and a cell in the data column can hold a number that isn't one —
// both become the cell's error, and every cell downstream shows blocked-upstream
// instead of a wrong total. Contrast the WASM-able recipes, which split with
// strings and quietly `_` away a parse error.
func rows(file string) (sales []Sale, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	return parse(recs)
}

// parse turns raw CSV records into typed rows, failing on the first bad number
// instead of coercing it to zero. Split out from the file read so it is an
// ordinary function to test — feed it a malformed record and it returns the
// error the cell would surface (see csv-native_test.go).
func parse(recs [][]string) ([]Sale, error) {
	var sales []Sale
	for i, rec := range recs {
		if i == 0 {
			continue // header
		}
		if len(rec) < 4 {
			return nil, fmt.Errorf("row %d: want 4 columns, got %d", i, len(rec))
		}
		units, err := strconv.Atoi(rec[2])
		if err != nil {
			return nil, fmt.Errorf("row %d: units %q: %w", i, rec[2], err)
		}
		rev, err := strconv.ParseFloat(rec[3], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: revenue %q: %w", i, rec[3], err)
		}
		sales = append(sales, Sale{Region: rec[0], Quarter: rec[1], Units: units, Revenue: rev})
	}
	return sales, nil
}

// The parsed rows, as a table. Downstream of `rows`, so it shows blocked-upstream
// whenever the read or a parse fails.
func rowsTable(sales []Sale) (table chart.Table) {
	return chart.RowsWith(chart.Opts{Title: "sales.csv"}, sales)
}

// Total revenue over the parsed rows — a second downstream cell, so a parse
// failure blocks it too, rather than reporting a total that quietly dropped a row.
func total(sales []Sale) (readout Readout) {
	var t float64
	for _, s := range sales {
		t += s.Revenue
	}
	return Readout{Label: "total revenue", Value: "$" + strconv.FormatFloat(t, 'f', 0, 64)}
}

// Sale is one parsed row. The field names double as the table's column headers.
type Sale struct {
	Region  string
	Quarter string
	Units   int
	Revenue float64
}

// Readout is a single stat card.
type Readout struct{ Label, Value string }

func (r Readout) Render() nb.Rendered {
	return nb.HTML(`<div style="font-family:system-ui,sans-serif">` +
		`<div style="font-size:12px;color:#5b6472">` + r.Label + `</div>` +
		`<div style="font:600 22px/1.2 system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">` + r.Value + `</div>` +
		`</div>`)
}
