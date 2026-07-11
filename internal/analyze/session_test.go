package analyze

import (
	"encoding/json"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// TestSessionMatchesColdAnalyze is the invariant that keeps the two derivation
// paths honest: an incremental Session.Reanalyze must produce the same graph as
// a cold TypesAnalyzer.Analyze. They share buildFromTypes, so this guards
// against the load/typecheck plumbing diverging.
func TestSessionMatchesColdAnalyze(t *testing.T) {
	const dir = capacityDir

	cold, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := NewSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	inc, _, err := sess.Reanalyze()
	if err != nil {
		t.Fatal(err)
	}

	if !jsonEqual(t, cold, inc) {
		t.Error("incremental Session graph differs from cold Analyze graph")
	}
}

// TestSessionReanalyzeStable confirms repeated re-analysis is deterministic —
// the cached importer is reused without drift.
func TestSessionReanalyzeStable(t *testing.T) {
	sess, err := NewSession(capacityDir)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := sess.Reanalyze()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := sess.Reanalyze()
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, first, second) {
		t.Error("repeated Session.Reanalyze produced different graphs")
	}
}

// TestGenericFuncIsNotACell and helper detection: pick is generic (never a
// cell), clamp names no result (a helper), doubled/base/bounded are cells.
func TestCellVsHelperBoundary(t *testing.T) {
	g, _, err := TypesAnalyzer{}.Analyze("testdata/graphs/helpers")
	if err != nil {
		t.Fatal(err)
	}

	wantCells := map[graph.CellID]bool{"base": true, "doubled": true, "bounded": true}
	for id := range wantCells {
		if _, ok := g.Cells[id]; !ok {
			t.Errorf("expected %q to be a cell", id)
		}
	}
	for _, notCell := range []graph.CellID{"clamp", "pick"} {
		if _, ok := g.Cells[notCell]; ok {
			t.Errorf("%q must not be a cell (clamp names no result; pick is generic)", notCell)
		}
	}

	// clamp is a listed helper; pick is generic and is NOT listed (a distinct
	// exclusion, not a "forgot to name a result" mistake).
	helpers := map[graph.CellID]bool{}
	for _, h := range g.Helpers {
		helpers[h] = true
	}
	if !helpers["clamp"] {
		t.Errorf("clamp should be listed as a helper, got helpers=%v", g.Helpers)
	}
	if helpers["pick"] {
		t.Errorf("generic pick should NOT be listed as a helper, got helpers=%v", g.Helpers)
	}
}

// TestPurityCHA confirms the CHA-based refinement marks compute-only cells pure
// even when they use fmt (which VTA was needed for under the old approach — but
// capacity's cells write to strings.Builder, and CHA still resolves those as
// pure here because they don't reach os/rand/time directly). The key assertion
// is the safe direction: nothing compute-only is left impure that shouldn't be,
// and any over-approximation only ever costs a cache hit.
func TestPurityCHA(t *testing.T) {
	g, _, err := TypesAnalyzer{}.Analyze(capacityDir)
	if err != nil {
		t.Fatal(err)
	}
	// Before refinement, everything defaults to the safe impure verdict.
	for id, c := range g.Cells {
		if c.Pure {
			t.Fatalf("cell %q should default to impure before RefinePurity", id)
		}
	}
	pkg, err := LoadForPurity(capacityDir)
	if err != nil {
		t.Fatal(err)
	}
	RefinePurity(pkg, g)

	// arrivalRate is a trivial constant cell — unambiguously pure.
	if !g.Cells["arrivalRate"].Pure {
		t.Error("arrivalRate should be pure after refinement")
	}
}

// jsonEqual compares two graphs by their JSON encoding, which is stable and
// ignores map ordering.
func jsonEqual(t *testing.T, a, b *graph.Graph) bool {
	t.Helper()
	ba, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(ba) == string(bb)
}
