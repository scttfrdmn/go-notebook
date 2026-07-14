package portfolio

import (
	"reflect"
	"testing"
)

// TestFixtureDaily pins the reproducible price path: the checked-in prices.csv
// yields a non-empty, sorted series for a held ticker and ERRORS on an unknown
// one (a missing ticker is a fault, not a silent empty series — the same
// discipline the notebook is about). This is the path CI and `notebook run`
// verification take, so it must not depend on the network.
func TestFixtureDaily(t *testing.T) {
	for _, tk := range []Ticker{"MSFT", "AAPL"} {
		bars, err := fixtureDaily(tk)
		if err != nil {
			t.Fatalf("fixtureDaily(%s): %v", tk, err)
		}
		if len(bars) == 0 {
			t.Fatalf("fixtureDaily(%s): empty series", tk)
		}
		for i := 1; i < len(bars); i++ {
			if bars[i].Date.Before(bars[i-1].Date) {
				t.Errorf("fixtureDaily(%s): not sorted at %d", tk, i)
			}
		}
	}
	// Case-insensitive, matching fetchDaily's ToUpper handling.
	if _, err := fixtureDaily("msft"); err != nil {
		t.Errorf("fixtureDaily(msft) lower-case: %v", err)
	}
	// Unknown ticker is an error, never a silent empty series.
	if _, err := fixtureDaily("NOPE"); err == nil {
		t.Error("fixtureDaily(NOPE): want error for unknown ticker, got nil")
	}
}

// TestFixtureDrivesGraph proves the fixture path reaches the numbers: with the
// default holdings, performance() computes a non-empty series with a positive
// invested basis. This is the downstream numeric render that the dead Stooq
// endpoint blocked (#96) — here it runs offline, deterministically.
func TestFixtureDrivesGraph(t *testing.T) {
	lots := holdings()
	bars := Prices{}
	for _, tk := range tickers(lots.Value) {
		b, err := fixtureDaily(tk)
		if err != nil {
			t.Fatalf("fixtureDaily(%s): %v", tk, err)
		}
		bars[tk] = b
	}
	series, err := performance(lots, bars)
	if err != nil {
		t.Fatalf("performance: %v", err)
	}
	if len(series) == 0 {
		t.Fatal("performance produced no snapshots — the graph did not reach the numbers")
	}
	last := series[len(series)-1]
	if last.Invested <= 0 {
		t.Errorf("last snapshot Invested = %v, want > 0", last.Invested)
	}
}

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
