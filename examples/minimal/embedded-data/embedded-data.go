//go:notebook
//
// embedded-data — ship a dataset inside the binary with go:embed, and stay
// WASM-able.
//
// The data is a real file (sales.csv, right next to this one), not a string
// constant — but go:embed reads it AT COMPILE TIME and bakes the bytes into the
// program, so no filesystem access happens at run time. That is the whole point:
// an embedded file is portable in a way an os.Open is not. This notebook builds
// to WebAssembly and runs in the browser, where there is no disk to open (the
// csv-native recipe is the same analysis with os.Open + encoding/csv, and it is
// native-only precisely because it does touch the filesystem).
//
// The parser is strings.Split, not encoding/csv — encoding/csv transitively
// reaches os, which the WASM gate refuses. For the simple unquoted data here,
// splitting on commas is enough and stays portable. That tradeoff is visible in
// what each cell is allowed to import.
//
//	go tool notebook run ./examples/minimal/embedded-data
//
// Demonstrates: go:embed a dataset, a WASM-able strings parser, one derived table.

package embeddeddata

import (
	_ "embed"
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// The dataset, embedded at compile time. The bytes are baked into the binary, so
// reading `salesCSV` at run time is just a string read — no disk, hence still
// WASM-able. Point this at any file in the package directory.
//
//go:embed sales.csv
var salesCSV string

// Parse the embedded bytes into typed rows. strings.Split, not encoding/csv:
// the latter reaches os and would make this cell non-portable. One parse cell,
// so everything downstream receives [Sale], not raw text.
func rows() (sales []Sale) {
	lines := strings.Split(strings.TrimSpace(salesCSV), "\n")
	for _, line := range lines[1:] { // skip the header
		rec := strings.Split(line, ",")
		if len(rec) < 4 {
			continue
		}
		units, _ := strconv.Atoi(strings.TrimSpace(rec[2]))
		rev, _ := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		sales = append(sales, Sale{
			Region:  rec[0],
			Quarter: rec[1],
			Units:   units,
			Revenue: rev,
		})
	}
	return sales
}

// The embedded rows, as a table. chart.Rows reflects over the []Sale: the field
// names become the column headers.
func rowsTable(sales []Sale) (table chart.Table) {
	return chart.RowsWith(chart.Opts{Title: "Embedded sales.csv"}, sales)
}

// Sale is one parsed row. The field names double as the table's column headers.
type Sale struct {
	Region  string
	Quarter string
	Units   int
	Revenue float64
}
