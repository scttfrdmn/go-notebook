package analyze

import (
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// TestWASMability pins the distinct-from-purity analysis: a notebook of pure
// arithmetic is WASM-able; one that touches net/os is not. Decided from the
// graph, not by hand.
func TestWASMability(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		nb       string
		wantOK   bool
		wantCell string // a cell expected on the WASMable side of the verdict
	}{
		{"capacity", true, "utilization"},
		{"seam", false, ""}, // fetches an image over net/http
	}
	for _, tc := range cases {
		dir := filepath.Join(root, "examples", tc.nb)
		g, _, err := TypesAnalyzer{}.Analyze(dir)
		if err != nil {
			t.Fatalf("%s: analyze: %v", tc.nb, err)
		}
		pkg, err := LoadForPurity(dir)
		if err != nil {
			t.Fatalf("%s: load: %v", tc.nb, err)
		}
		WASMability(pkg, g)
		ok, blockers := NotebookWASMable(g)
		if ok != tc.wantOK {
			t.Errorf("%s: NotebookWASMable = %v (blockers %v), want %v", tc.nb, ok, blockers, tc.wantOK)
		}
		if tc.wantCell != "" && !g.Cells[graph.CellID(tc.wantCell)].WASMable {
			t.Errorf("%s: cell %q should be WASM-able", tc.nb, tc.wantCell)
		}
	}
}

// TestWASMabilityIsNotPurity is the finding, pinned: a cell that is impure
// (touches time/rand) can still be WASM-able, because time and randomness work
// in the browser. Purity and WASM-ability must not be conflated.
func TestWASMabilityIsNotPurity(t *testing.T) {
	// time.Now is impure (breaks caching) but perfectly WASM-able (the browser
	// has a clock) — the crux of the finding: WASM-ability ≠ purity. os is
	// neither pure nor WASM-able.
	src := `
import (
	"time"
	"os"
)
// clockCell is impure (time.Now) but perfectly WASM-able.
func clockCell() (t int64) { return time.Now().Unix() }
// fileCell touches os — NOT WASM-able.
func fileCell() (s string) { b, _ := os.ReadFile("/x"); return string(b) }
`
	dir := writeNotebook(t, src)
	g, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadForPurity(dir)
	if err != nil {
		t.Fatal(err)
	}
	WASMability(pkg, g)

	// The finding, pinned: an impure cell (time.Now) is still WASM-able.
	if !g.Cells["clockCell"].WASMable {
		t.Error("time.Now is impure but WASM-able; clockCell should be WASM-able")
	}
	// And genuine host access is correctly disqualified.
	if g.Cells["fileCell"].WASMable {
		t.Error("os.ReadFile is not WASM-able; fileCell must be flagged")
	}
	// (math/rand is also WASM-able in truth, but CHA over-approximates it as
	// non-portable via the global Source interface — the same conservatism as
	// fmt→os in purity. Over-rejection is the safe direction; not asserted here.)
}
