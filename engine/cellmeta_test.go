package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCellMetaLeafTypeShape pins the JSON a leaf's CellMeta puts on the port.
// jsonToJS (engine/wasm) marshals CellMeta through encoding/json, so this shape
// IS what globalThis.notebook.meta carries and what the JS client's leaves()[].type
// reads. B4b's contract: a leaf reports its Go result type in two coordinates —
// the declared Name and the resolved Underlying kind — so a host can validate a
// set() value's shape without knowing Go. This freezes that shape (PascalCase
// keys, both coordinates present for a named-over-basic leaf).
func TestCellMetaLeafTypeShape(t *testing.T) {
	m := CellMeta{
		ID:   "arrivalRate",
		Leaf: "lambda",
		Type: &LeafType{Name: "PerHour", Underlying: "float64"},
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// The client reads m.Type.Name / m.Type.Underlying off exactly these keys.
	want := `{"ID":"arrivalRate","Leaf":"lambda","Label":"","Directives":null,"Type":{"Name":"PerHour","Underlying":"float64"}}`
	if string(got) != want {
		t.Errorf("leaf CellMeta JSON drifted:\n got %s\nwant %s", got, want)
	}
}

// TestCellMetaTypeOmittedForNonLeaf: a non-leaf cell carries no Type key at all
// (omitempty), so a client's `m.Type ?? null` reads null — not an empty object.
// This is the inertness half of the field: a consumer that ignores Type sees the
// same bytes it saw before B4b for every non-leaf cell.
func TestCellMetaTypeOmittedForNonLeaf(t *testing.T) {
	got, err := json.Marshal(CellMeta{ID: "utilization"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Type") {
		t.Errorf("a non-leaf CellMeta must omit Type, got %s", got)
	}
}

// TestLeafTypeOmitsUnderlyingForComposite: a composite/interface leaf has no
// single scalar kind, so Underlying is omitted (not sent as ""). A client then
// sees {Name} with no Underlying and falls back to the coercer, exactly as the
// okForLeaf shape-checker's default branch does.
func TestLeafTypeOmitsUnderlyingForComposite(t *testing.T) {
	got, err := json.Marshal(&LeafType{Name: "Multi[Theme]"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Name":"Multi[Theme]"}`
	if string(got) != want {
		t.Errorf("composite LeafType should omit Underlying:\n got %s\nwant %s", got, want)
	}
}
