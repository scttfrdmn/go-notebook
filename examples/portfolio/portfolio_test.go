package portfolio

import (
	"reflect"
	"testing"
)

// TestTableReconcile pins the Table's entry in the reconcile taxonomy: a client
// edit arrives as []map[string]any (the coerced wire row set) and Reconcile
// rebuilds []Lot, round-tripping each field through its own JSON codec —
// including Date, whose MarshalJSON/UnmarshalJSON put it on the wire as
// "2006-01-02" rather than time.Time's RFC3339. A selection that isn't a usable
// row set resets to the cell's default rows (never keeps a stale shape). This is
// the write path a real SSE edit exercises; the test makes it permanent.
func TestTableReconcile(t *testing.T) {
	def := holdings() // the cell's default rows (the schema)

	// A user edited Amount on row 0 and the ticker on row 1 — the whole set is
	// re-emitted, exactly as the grid does on any cell edit.
	edited := []map[string]any{
		{"Date": "2021-02-01", "Ticker": "MSFT", "Amount": 999.0},
		{"Date": "2023-02-01", "Ticker": "GOOG", "Amount": 800.0},
		{"Date": "2024-02-01", "Ticker": "AAPL", "Amount": 200.0},
	}
	got := def.Reconcile(edited).(Table[Lot])
	if len(got.Value) != 3 {
		t.Fatalf("rows = %d, want 3", len(got.Value))
	}
	if got.Value[0].Amount != 999 {
		t.Errorf("row0 Amount = %v, want 999 (the edit)", got.Value[0].Amount)
	}
	if got.Value[1].Ticker != "GOOG" {
		t.Errorf("row1 Ticker = %v, want GOOG (the edit)", got.Value[1].Ticker)
	}
	if got.Value[0].Date.String() != "2021-02-01" {
		t.Errorf("row0 Date = %v, want 2021-02-01 (codec round-trip)", got.Value[0].Date.String())
	}

	// RESET on an unusable selection: not a row set → the default rows stand.
	reset := def.Reconcile("garbage").(Table[Lot])
	if !reflect.DeepEqual(reset.Value, def.Value) {
		t.Errorf("bad selection should reset to the default rows, got %v", reset.Value)
	}
}

// TestDateJSONRoundTrip pins the Date codec directly: the string form the grid
// shows and the user types must survive a marshal/unmarshal cycle, or a table
// edit to the date column would silently corrupt (RFC3339 in, "2006-01-02" out).
func TestDateJSONRoundTrip(t *testing.T) {
	d := day("2022-07-14")
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `"2022-07-14"` {
		t.Errorf("marshal = %s, want \"2022-07-14\"", b)
	}
	var back Date
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.String() != "2022-07-14" {
		t.Errorf("round-trip = %v, want 2022-07-14", back.String())
	}
}
