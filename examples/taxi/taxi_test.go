package taxi

import "testing"

// TestPathIsNotAHandle is the rule the design claims, proven by code rather than
// asserted in a comment: a handle IDENTIFIES ITS CONTENTS. Change the file and
// the handle changes; downstream, keyed on the handle, must therefore recompute.
//
// This is the distinction that separates Rel from the portfolio tracker's
// parent_folder constant — a constant path is identical whether the download
// succeeded, failed, or fetched the wrong company, so the graph is blind. Rel is
// a value: its identity travels with its contents.
func TestPathIsNotAHandle(t *testing.T) {
	orig := tripData
	changed := tripData + "\n2024-03-01T23:00:00Z,2024-03-01T23:15:00Z,20.00,4.00,1"

	a, err := Open[Trip](orig)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open[Trip](changed)
	if err != nil {
		t.Fatal(err)
	}

	// The whole rule: different contents → different handle.
	if a.Schema == b.Schema {
		t.Error("changing the file did not change the content hash — the handle is blind to its contents (the portfolio bug)")
	}
	if a.Rows == b.Rows {
		t.Error("adding a row did not change the row count")
	}
	// And identical contents → identical handle (so an unchanged file does NOT
	// spuriously invalidate downstream).
	a2, _ := Open[Trip](orig)
	if a.Schema != a2.Schema {
		t.Error("identical contents produced different hashes — the handle is unstable")
	}
}

// TestHandleEqualDrivesPruning confirms Rel.Equal reports handle identity, which
// is what lets the engine's Equal(any) pruning rung treat an unchanged relation
// as unchanged (no downstream wake) and a changed one as changed (invalidate).
// This is the path-is-not-a-handle rule expressed as an engine-visible value —
// the same rung exercised generically in engine's TestEqualPruningRung, here on
// the real handle type.
func TestHandleEqualDrivesPruning(t *testing.T) {
	a, _ := Open[Trip](tripData)
	same, _ := Open[Trip](tripData)
	diff, _ := Open[Trip](tripData + "\n2024-03-01T23:00:00Z,2024-03-01T23:15:00Z,20,4,1")

	if !a.Equal(same) {
		t.Error("two handles over identical contents must be Equal (else an unchanged file wakes the subtree every wave)")
	}
	if a.Equal(diff) {
		t.Error("two handles over different contents must NOT be Equal (else a changed file is silently ignored)")
	}
}

// TestScanIsOutOfCore confirms Scan streams rows through the callback rather than
// returning a materialized slice — the out-of-core shape. (Here over embedded
// data; the property is that the API never hands back []Trip of the whole file.)
func TestScanIsOutOfCore(t *testing.T) {
	rel, _ := Open[Trip](tripData)
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
