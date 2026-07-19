//go:notebook
//
// table — an editable grid.
//
// A type with a slice-of-struct Value field renders as a table; the columns come
// from the struct's fields. The user edits cells directly. Reconcile round-trips
// the edited rows (which arrive as []map[string]any) back into the row type via
// JSON, so a row that no longer fits the type is dropped rather than kept stale.
//
//	go tool notebook run ./examples/minimal/table
//
// Demonstrates: slice-of-struct Value -> table, JSON row reconcile. See docs/reference-controls.html.

package table

import (
	"encoding/json"
	"strconv"
)

// Line items. Edit the table directly.
func items() (rows Grid) {
	return Grid{Value: []Item{
		{Name: "widget", Qty: 3, Unit: 250},
		{Name: "gadget", Qty: 1, Unit: 1200},
	}}
}

// The total across all rows, in cents — derived from the edited table.
func total(rows Grid) (label Money) {
	sum := 0
	for _, it := range rows.Value {
		sum += it.Qty * it.Unit
	}
	return Money(sum)
}

// Item is one row; its exported fields become the table's columns.
type Item struct {
	Name string
	Qty  int
	Unit int // cents
}

// Grid is the editable table: a slice-of-struct Value.
type Grid struct {
	Value []Item
}

func (g Grid) Reconcile(saved any) any {
	rows, ok := saved.([]map[string]any)
	if !ok {
		return g
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return g
	}
	var out []Item
	if err := json.Unmarshal(b, &out); err != nil {
		return g
	}
	return Grid{Value: out}
}

// Money is a plain-text readout of a cent amount. strconv (not fmt) keeps the
// cell body portable — but this is a Render method, so fmt would be fine here too.
type Money int

func (m Money) Render() Rendered {
	return Rendered{MIME: "text/plain", Data: "$" + strconv.FormatFloat(float64(m)/100, 'f', 2, 64)}
}

// Rendered is redeclared locally — this example shows the zero-import track.
type Rendered struct{ MIME, Data string }
