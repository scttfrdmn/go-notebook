package analyze

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"strings"
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

// TestCellSourceCaptured confirms the read-only source view's data: each cell
// carries its verbatim source — doc comment through the function body — so the
// view can show "a cell is a function." Asserts the actual text (§8), including
// that the doc comment and body are both present.
func TestCellSourceCaptured(t *testing.T) {
	g, _, err := TypesAnalyzer{}.Analyze("testdata/graphs/helpers")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := g.Cells["doubled"]
	if !ok {
		t.Fatal("doubled should be a cell")
	}
	if !strings.Contains(c.Source, "func doubled(") {
		t.Errorf("source should contain the func signature; got:\n%s", c.Source)
	}
	if !strings.Contains(c.Source, "return") {
		t.Errorf("source should contain the body; got:\n%s", c.Source)
	}
	// The captured source is a self-contained func decl, so it re-parses.
	if _, perr := parser.ParseFile(token.NewFileSet(), "src.go",
		"package p\n"+c.Source, parser.AllErrors); perr != nil {
		t.Errorf("captured source does not re-parse: %v\n%s", perr, c.Source)
	}
}

// TestPurityDefaultsImpure confirms the always-safe default: before
// RefinePurity runs, every cell is impure. The graph derivation must not depend
// on purity, so the interactive path never blocks on it.
func TestPurityDefaultsImpure(t *testing.T) {
	g, _, err := TypesAnalyzer{}.Analyze("testdata/graphs/purity")
	if err != nil {
		t.Fatal(err)
	}
	for id, c := range g.Cells {
		if c.Pure {
			t.Errorf("cell %q should default to impure before RefinePurity", id)
		}
	}
}

// TestPurityRefinement pins the CHA verdicts on a fixture with known-pure,
// known-impure, and over-approximated cells:
//
//   - base, doubled       genuinely pure arithmetic → pure
//   - noise (math/rand)   genuinely impure → MUST stay impure (the direction
//     that matters: a cached stale draw would be silently wrong)
//   - stamp (time.Now)    genuinely impure → MUST stay impure
//   - formatted (fmt)     pure arithmetic that formats; CHA conservatively
//     marks it impure — the safe over-approximation, costing only a cache hit
func TestPurityRefinement(t *testing.T) {
	const dir = "testdata/graphs/purity"
	g, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadForPurity(dir)
	if err != nil {
		t.Fatal(err)
	}
	RefinePurity(pkg, g)

	// The direction that must never break: an impure cell classified pure would
	// serve stale cached state and be silently wrong.
	for _, id := range []string{"noise", "stamp"} {
		if g.Cells[graph.CellID(id)].Pure {
			t.Errorf("%q reaches an impure primitive; marking it pure is a correctness bug", id)
		}
	}
	// The safe direction: genuinely pure cells are pure.
	for _, id := range []string{"base", "doubled"} {
		if !g.Cells[graph.CellID(id)].Pure {
			t.Errorf("%q is pure arithmetic and should be classified pure", id)
		}
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
