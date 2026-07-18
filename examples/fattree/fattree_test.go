package fattree

import (
	"math"
	"strings"
	"testing"
)

// res is the analyzed result for a (depth, fatness%) tree.
func res(depth, fatness int) Result { return analyze(topology(depth, fatness)) }

// TestTopologyIsPure: the tree is a pure function of (depth, fatness) — same inputs,
// identical structure and metrics.
func TestTopologyIsPure(t *testing.T) {
	a := res(4, 50)
	b := res(4, 50)
	if a.Bisection != b.Bisection || a.Slowdown != b.Slowdown || a.CostSaving != b.CostSaving {
		t.Error("analyze is not pure")
	}
}

// TestNonBlockingScalesWithNodes is the ideal: at 100% fatness bisection bandwidth
// equals the node count (full), the slowdown is 1, and it holds at every depth.
func TestNonBlockingScalesWithNodes(t *testing.T) {
	for _, d := range []int{2, 3, 4, 5} {
		r := res(d, 100)
		if math.Abs(r.Bisection-float64(r.Tree.Nodes)) > 1e-9 {
			t.Errorf("depth %d: non-blocking bisection %.2f should equal nodes %d", d, r.Bisection, r.Tree.Nodes)
		}
		if math.Abs(r.Slowdown-1.0) > 1e-9 {
			t.Errorf("depth %d: non-blocking slowdown %.3f should be 1", d, r.Slowdown)
		}
	}
}

// TestOversubscriptionCompounds is THE teaching claim: the all-to-all slowdown is
// 1/f^(depth−1), so a fixed per-tier fatness bites exponentially harder in a deeper
// tree. Verify the exact compounding, and the headline case: 50% over 4 tiers = 8×.
func TestOversubscriptionCompounds(t *testing.T) {
	// 50% fatness (f=0.5): slowdown should be 2^(depth-1).
	for _, d := range []int{2, 3, 4, 5} {
		r := res(d, 50)
		want := math.Pow(2, float64(d-1))
		if math.Abs(r.Slowdown-want) > 1e-6 {
			t.Errorf("depth %d @ 50%%: slowdown %.3f, want %.3f (2^(depth-1))", d, r.Slowdown, want)
		}
	}
	// the headline: 4 tiers, 2:1 per tier → 8× slower, not 2×.
	if s := res(4, 50).Slowdown; math.Abs(s-8) > 1e-6 {
		t.Errorf("headline case (depth 4, 50%%) should be 8× slower, got %.2f", s)
	}
}

// TestDeeperTreeHurtsMoreAtSameFatness: at a fixed oversubscription, a deeper tree has
// strictly worse all-to-all slowdown — the compounding, stated as a monotonicity.
func TestDeeperTreeHurtsMoreAtSameFatness(t *testing.T) {
	prev := 0.0
	for _, d := range []int{2, 3, 4, 5} {
		s := res(d, 75).Slowdown
		if !(s > prev) {
			t.Errorf("deeper tree should slow all-to-all more at fixed fatness: depth %d slowdown %.3f not > %.3f", d, s, prev)
		}
		prev = s
	}
}

// TestDeeperTreeMakesOversubscriptionAWorseDeal is the trade the notebook exists to
// show, stated as the model actually behaves. (An earlier draft claimed "cost is
// linear / never a free lunch" — the sweep refuted both: cost also drops steeply, and
// at shallow depth 2 oversubscription actually saves MORE cost-fraction than the
// bandwidth-fraction it gives up. The honest, sharper lesson: the deal gets worse with
// depth.) Define gap = (bandwidth fraction lost) − (cost fraction saved): it rises
// monotonically with depth, is negative for a shallow tree (a fine trade), and turns
// positive for deep trees (bandwidth lost outruns cost saved).
func TestDeeperTreeMakesOversubscriptionAWorseDeal(t *testing.T) {
	gap := func(d int) float64 {
		r := res(d, 50)
		return (1 - r.Bisection/r.FullBisect) - r.CostSaving
	}
	prev := math.Inf(-1)
	for _, d := range []int{2, 3, 4, 5} {
		g := gap(d)
		if g < prev {
			t.Errorf("depth %d: the (bw-lost − cost-saved) gap %.3f should widen with depth (was %.3f)", d, g, prev)
		}
		prev = g
	}
	if gap(2) >= 0 {
		t.Errorf("at shallow depth 2, oversubscription should be a fine trade (gap<0), got %.3f", gap(2))
	}
	if gap(5) <= 0 {
		t.Errorf("at depth 5, bandwidth lost should outrun cost saved (gap>0), got %.3f", gap(5))
	}
	// a full-fat tree saves nothing.
	if s := res(4, 100).CostSaving; math.Abs(s) > 1e-9 {
		t.Errorf("non-blocking tree should have zero cost saving, got %.3f", s)
	}
}

// TestTierStructure: a binary fat-tree of depth d has 2^d nodes and 2^(d-level)
// switches per tier — the drawing depends on this being right.
func TestTierStructure(t *testing.T) {
	tr := topology(4, 100)
	if tr.Nodes != 16 {
		t.Errorf("depth 4 should have 16 nodes, got %d", tr.Nodes)
	}
	wantSwitches := []int{8, 4, 2, 1}
	for i, w := range wantSwitches {
		if tr.Tiers[i].Switches != w {
			t.Errorf("tier %d should have %d switches, got %d", i+1, w, tr.Tiers[i].Switches)
		}
	}
}

// TestViewsRender: the diagram is SVG with nodes, links, and the bisection cut; the
// verdict readout is HTML (else the client hides it — the pid/Readout lesson).
func TestViewsRender(t *testing.T) {
	r := res(4, 50)

	d := diagram(r).Render()
	if !strings.HasPrefix(d.Data, "<svg") {
		t.Fatal("diagram not SVG")
	}
	for _, w := range []string{"fat-tree", "bisection cut", "<circle", "<line"} {
		if !strings.Contains(d.Data, w) {
			t.Errorf("diagram missing %q", w)
		}
	}
	// oversubscribed tree should draw amber links.
	if !strings.Contains(d.Data, "#fab219") {
		t.Error("oversubscribed diagram should have amber (oversubscribed) links")
	}
	// a non-blocking tree should NOT.
	full := diagram(res(4, 100)).Render()
	if strings.Contains(full.Data, "#fab219") {
		t.Error("non-blocking diagram should have no amber links")
	}

	vd := verdict(r).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html", vd.MIME)
	}
	for _, w := range []string{"bisection bandwidth", "all-to-all penalty", "switch-cost saving"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
