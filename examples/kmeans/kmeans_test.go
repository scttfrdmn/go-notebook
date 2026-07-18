package kmeans

import (
	"strings"
	"testing"
)

// TestClusterPure confirms the clustering is a pure function of (points, init,
// steps): same inputs → identical result. This is what lets a drag re-run from
// scratch and the iteration slider rewind convergence exactly.
func TestClusterPure(t *testing.T) {
	pts := data()
	init := centroids(3, 3)
	a := cluster(pts, init, 12)
	b := cluster(pts, init, 12)
	if a.Inertia != b.Inertia {
		t.Fatalf("inertia not pure: %v vs %v", a.Inertia, b.Inertia)
	}
	for i := range a.Assign {
		if a.Assign[i] != b.Assign[i] {
			t.Fatalf("assignment not pure at %d", i)
		}
	}
}

// TestConverges: inertia is non-increasing as Lloyd's algorithm iterates (each
// assign/update step can only lower or hold the objective). If it ever rose, the
// implementation would be wrong.
func TestConverges(t *testing.T) {
	pts := data()
	init := centroids(3, 3)
	prev := cluster(pts, init, 0).Inertia
	for steps := 1; steps <= 12; steps++ {
		cur := cluster(pts, init, steps).Inertia
		if cur > prev+1e-6 {
			t.Errorf("inertia rose from %.1f to %.1f at step %d — Lloyd's must be non-increasing", prev, cur, steps)
		}
		prev = cur
	}
}

// TestSeedSensitivity is the teaching claim (the footgun): different initial seeds
// converge to different inertia — the local-minimum problem. If every seed gave the
// same answer, the whole "initialization ruins your day" point would be false.
func TestSeedSensitivity(t *testing.T) {
	pts := data()
	seen := map[int]bool{}
	best, worst := 1e18, 0.0
	for s := 1; s <= 40; s++ {
		in := cluster(pts, centroids(3, s), 20).Inertia
		bucket := int(in / 1000) // coarse bucket so tiny float diffs don't over-count
		seen[bucket] = true
		if in < best {
			best = in
		}
		if in > worst {
			worst = in
		}
	}
	if len(seen) < 2 {
		t.Errorf("every seed converged to the same inertia bucket — no local-minimum problem to show")
	}
	// the worst local minimum should be meaningfully worse than the best.
	if worst < 1.5*best {
		t.Errorf("worst inertia %.0f is not meaningfully worse than best %.0f — footgun not demonstrable", worst, best)
	}
}

// TestGoodSeedSeparatesBlobs: on a well-chosen seed, the three blobs each become
// their own cluster of ~40 points (the data has three groups of 40).
func TestGoodSeedSeparatesBlobs(t *testing.T) {
	pts := data()
	res := cluster(pts, centroids(3, 3), 20)
	counts := make([]int, 3)
	for _, a := range res.Assign {
		counts[a]++
	}
	for c, n := range counts {
		if n < 25 || n > 55 { // each true blob has 40; allow slop but reject collapse
			t.Errorf("cluster %d has %d points; expected ~40 (blobs not cleanly separated)", c, n)
		}
	}
}

// TestArityReconcile: when k changes, the centroid count changes, so a saved drag
// for a different k must reset to the fresh seeding rather than mis-fit.
func TestArityReconcile(t *testing.T) {
	three := centroids(3, 3).WithLeaf("c")
	// a saved selection sized for k=2 must not be adopted onto a k=3 leaf.
	savedForK2 := []float64{10, 10, 90, 90}
	got := three.Reconcile(savedForK2).(Draggable[Pt])
	if len(got.Value) != 3 {
		t.Errorf("reconcile adopted a k=2 selection onto a k=3 leaf (got %d centroids)", len(got.Value))
	}
}

// TestPlotRendersWithGrips: the chart renders SVG, colours points by assignment, and
// carries one draggable grip per initial centroid.
func TestPlotRendersWithGrips(t *testing.T) {
	pts := data()
	init := centroids(3, 3).WithLeaf("c")
	data := plot(pts, init, cluster(pts, init, 12)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("plot is not SVG")
	}
	if got := strings.Count(data, "data-grip"); got != 3 {
		t.Errorf("%d grips, want 3 (one per centroid)", got)
	}
	// at least two distinct cluster colours are used (points really are grouped) —
	// the first two brand categorical slots (blue, aqua)
	if !strings.Contains(data, "#2a78d6") || !strings.Contains(data, "#0797b8") {
		t.Error("plot does not colour points by cluster")
	}
}
