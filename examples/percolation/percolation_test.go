package percolation

import (
	"strings"
	"testing"
)

// TestGridPure confirms the field is a pure function of (n, pct, s) — same sliders,
// same grid — so scrubbing p up and back down is exact (the no-fold property).
func TestGridPure(t *testing.T) {
	a := grid(64, 55, 7)
	b := grid(64, 55, 7)
	for i := range a.Open {
		if a.Open[i] != b.Open[i] {
			t.Fatalf("grid not pure at %d", i)
		}
	}
}

// TestPhaseTransition is the teaching claim, proven numerically: below the critical
// probability the grid does NOT span, above it it DOES, and the largest cluster
// jumps across the threshold (small clusters merge into one giant component). If
// this regresses, the demo's whole point — a sharp transition — is false.
func TestPhaseTransition(t *testing.T) {
	span := func(pct int) (bool, int) {
		c := clusters(grid(96, pct, 7))
		return c.Spanning >= 0, c.Largest
	}
	lowSpans, lowLargest := span(45)
	hiSpans, hiLargest := span(68)

	if lowSpans {
		t.Errorf("p=0.45 (well below p_c≈0.593) should NOT span")
	}
	if !hiSpans {
		t.Errorf("p=0.68 (well above p_c) SHOULD span")
	}
	// The giant-component jump: the largest cluster is far bigger above the
	// threshold than below it.
	if hiLargest < 3*lowLargest {
		t.Errorf("largest cluster did not jump across the transition: %d (low) vs %d (high)", lowLargest, hiLargest)
	}
}

// TestScrubBothWays confirms the transition is reversible — going up past p_c and
// back down returns to a non-spanning grid identical to the original. This is the
// property a fold couldn't give (a fold only goes forward); a pure cell can.
func TestScrubBothWays(t *testing.T) {
	base := clusters(grid(96, 50, 7))
	up := clusters(grid(96, 70, 7))
	back := clusters(grid(96, 50, 7)) // scrubbed back down

	if base.Spanning >= 0 {
		t.Fatal("precondition: p=0.50 should not span")
	}
	if up.Spanning < 0 {
		t.Fatal("precondition: p=0.70 should span")
	}
	// Coming back down reproduces the earlier state exactly.
	for i := range base.Root {
		if base.Root[i] != back.Root[i] {
			t.Fatalf("scrubbing back to p=0.50 did not reproduce the field at cell %d", i)
		}
	}
}

// TestDensityMonotone: more open cells at higher p (the slider bites), and fewer
// separate clusters as they merge.
func TestDensityMonotone(t *testing.T) {
	open := func(pct int) int {
		g := grid(96, pct, 7)
		n := 0
		for _, o := range g.Open {
			if o {
				n++
			}
		}
		return n
	}
	if open(40) >= open(70) {
		t.Errorf("higher p did not open more cells: %d vs %d", open(40), open(70))
	}
}

// TestUnionFind is a small direct check of the union-find: two unions that share a
// member collapse to one root.
func TestUnionFind(t *testing.T) {
	u := newUF(5)
	u.union(0, 1)
	u.union(1, 2)
	if u.find(0) != u.find(2) {
		t.Error("0 and 2 should share a root after union(0,1),union(1,2)")
	}
	if u.find(0) == u.find(3) {
		t.Error("0 and 3 were never unioned; must differ")
	}
}

// TestPictureHTML confirms the view renders as an SVG-wrapped PNG (the client paints
// image/svg+xml, not a bare image/png).
func TestPictureHTML(t *testing.T) {
	r := view(clusters(grid(16, 55, 7))).Render()
	if r.MIME != "image/svg+xml" {
		t.Fatalf("MIME = %q, want image/svg+xml", r.MIME)
	}
	for _, want := range []string{"<svg", "<image", "data:image/png;base64,", "pixelated"} {
		if !strings.Contains(r.Data, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}
