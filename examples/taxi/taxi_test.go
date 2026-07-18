package taxi

import "testing"

// TestPathIsNotAHandle is the rule the design claims, proven by code rather than
// asserted in a comment: a handle IDENTIFIES ITS CONTENTS. Two different sources
// (day vs. night) produce different handles; the same source produces an
// identical handle.
//
// This is the distinction that separates Rel from the portfolio tracker's
// parent_folder constant — a constant path is identical whether the download
// succeeded, failed, or fetched the wrong company, so the graph is blind. Rel is
// a value: its identity travels with its contents.
func TestPathIsNotAHandle(t *testing.T) {
	day, err := Open[Trip]("trips-day.csv")
	if err != nil {
		t.Fatal(err)
	}
	night, err := Open[Trip]("trips-night.csv")
	if err != nil {
		t.Fatal(err)
	}

	// The whole rule: different contents → different handle.
	if day.Schema == night.Schema {
		t.Error("two different datasets share a content hash — the handle is blind to its contents (the portfolio bug)")
	}
	if day.Rows == night.Rows {
		t.Error("two datasets with different row counts reported the same Rows")
	}
	// And identical contents → identical handle (so an unchanged source does NOT
	// spuriously invalidate downstream).
	day2, _ := Open[Trip]("trips-day.csv")
	if day.Schema != day2.Schema {
		t.Error("the same source produced different hashes — the handle is unstable")
	}

	// An unknown source is an error, never a silent empty handle.
	if _, err := Open[Trip]("does-not-exist.csv"); err == nil {
		t.Error("Open of an unknown source should error, not return a dangling handle")
	}
}

// TestHandleEqualDrivesPruning confirms Rel.Equal reports handle identity, which
// is what lets the engine's Equal(any) pruning rung treat an unchanged relation
// as unchanged (no downstream wake) and a changed one as changed (invalidate).
func TestHandleEqualDrivesPruning(t *testing.T) {
	a, _ := Open[Trip]("trips-day.csv")
	same, _ := Open[Trip]("trips-day.csv")
	diff, _ := Open[Trip]("trips-night.csv")

	if !a.Equal(same) {
		t.Error("two handles over the same source must be Equal (else an unchanged file wakes the subtree every wave)")
	}
	if a.Equal(diff) {
		t.Error("two handles over different sources must NOT be Equal (else a changed dataset is silently ignored)")
	}
}

// TestReconcileDeliversNewData is the KC17 property at the unit level: setting
// the handle leaf to a new source (a wire {Source} selection) yields a handle
// over that source, and Scan streams the NEW dataset's rows. Bulk data-in as a
// content-addressed handle: identity crosses the wire, contents follow because
// Scan reads the source the handle names. Rows/Schema are DERIVED by re-Opening,
// never trusted from the wire — a lie about the row count cannot survive.
func TestReconcileDeliversNewData(t *testing.T) {
	day, _ := Open[Trip]("trips-day.csv")

	// A host sets a new handle: only the source is the host's to choose.
	got := day.Reconcile(map[string]any{"Source": "trips-night.csv"})
	night, ok := got.(Rel[Trip])
	if !ok {
		t.Fatalf("Reconcile returned %T, want Rel[Trip]", got)
	}
	if night.Source != "trips-night.csv" {
		t.Errorf("Reconcile kept source %q, want trips-night.csv", night.Source)
	}
	// Rows/Schema are DERIVED, matching a fresh Open — not whatever the wire said.
	fresh, _ := Open[Trip]("trips-night.csv")
	if night.Rows != fresh.Rows || night.Schema != fresh.Schema {
		t.Error("Reconcile did not derive Rows/Schema from the source — a handle could then lie about its contents")
	}

	// A wire selection that lies about Rows is ignored: identity comes from the
	// contents, never the wire.
	lying := day.Reconcile(map[string]any{"Source": "trips-night.csv", "Rows": float64(9999)})
	if lying.(Rel[Trip]).Rows == 9999 {
		t.Error("Reconcile trusted a wire Rows count — the path-is-not-a-handle lie")
	}

	// Scan over each handle streams that dataset; a new handle delivers new data.
	dayRows, nightRows := 0, 0
	_ = Scan(day, func(Trip) { dayRows++ })
	_ = Scan(night, func(Trip) { nightRows++ })
	if int64(dayRows) != day.Rows || int64(nightRows) != night.Rows {
		t.Errorf("Scan streamed %d/%d rows, handles report %d/%d", dayRows, nightRows, day.Rows, night.Rows)
	}
	if dayRows == nightRows {
		t.Error("day and night streamed the same row count — the new handle did not deliver new data")
	}

	// An unknown or malformed selection leaves the handle unchanged (degrade to
	// the working dataset, never a broken one).
	if day.Reconcile("nonsense").(Rel[Trip]).Source != "trips-day.csv" {
		t.Error("a non-handle selection should leave the current handle unchanged")
	}
	if day.Reconcile(map[string]any{"Source": "ghost.csv"}).(Rel[Trip]).Source != "trips-day.csv" {
		t.Error("an unknown source should leave the current handle unchanged, not dangle")
	}
}

// TestScanIsOutOfCore confirms Scan streams rows through the callback rather than
// returning a materialized slice — the out-of-core shape. (Here over embedded
// data; the property is that the API never hands back []Trip of the whole file.)
func TestScanIsOutOfCore(t *testing.T) {
	rel, _ := Open[Trip]("trips-day.csv")
	seen := 0
	var revenue USD
	if err := Scan(rel, func(tr Trip) {
		seen++
		revenue += tr.Fare + tr.Tip
	}); err != nil {
		t.Fatal(err)
	}
	if int64(seen) != rel.Rows {
		t.Errorf("Scan streamed %d rows, handle reports %d", seen, rel.Rows)
	}
	if revenue == 0 {
		t.Error("Scan produced no revenue — rows didn't parse")
	}
}
