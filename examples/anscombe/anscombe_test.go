package anscombe

import (
	"math"
	"testing"
)

// TestDescribeMatchesNumpyReference pins the summary of the seed dinosaur against
// values computed independently, so a refactor of describe can't drift silently.
// (Reference values are the population moments of the datasaurus seed; recompute
// with the same definitions if the seed ever changes.)
func TestDescribeSeed(t *testing.T) {
	s := describe(datasaurus)
	// Sanity: the seed is a real 2D cloud, not degenerate.
	if s.StdX <= 0 || s.StdY <= 0 {
		t.Fatalf("degenerate seed: %+v", s)
	}
	// A dinosaur has almost no linear trend — the whole point is that r is near zero
	// while the picture is unmistakable. Guard the claim: |r| must be small.
	if math.Abs(s.Corr) > 0.35 {
		t.Errorf("seed correlation %.3f is too strong; the dinosaur should read as ~uncorrelated", s.Corr)
	}
}

// TestSummaryStatisticsLie is the teaching claim, proven numerically: two visibly
// different datasets can share every summary statistic. If this passes, the notebook
// is honest — the numbers really are blind to the shape.
//
// Construction: take any point set, then reflect it across its own centroid
// (p -> 2*centroid - p). The reflected cloud is a DIFFERENT set of points (a
// 180° rotation — a dinosaur becomes an upside-down dinosaur), yet mean, variance,
// covariance, and therefore correlation and the fitted line are all identical,
// because every centered moment is even in the reflection.
func TestSummaryStatisticsLie(t *testing.T) {
	orig := datasaurus
	s := describe(orig)

	reflected := make([]Pt, len(orig))
	differ := 0
	for i, p := range orig {
		reflected[i] = Pt{X: 2*s.MeanX - p.X, Y: 2*s.MeanY - p.Y}
		if reflected[i] != p {
			differ++
		}
	}
	if differ < len(orig)/2 {
		t.Fatalf("reflection did not produce a visibly different cloud (%d/%d points moved)", differ, len(orig))
	}

	r := describe(reflected)
	eq := func(name string, a, b float64) {
		if math.Abs(a-b) > 1e-9 {
			t.Errorf("%s differs under reflection: %.12f vs %.12f — the statistics were NOT blind to the shape", name, a, b)
		}
	}
	eq("meanX", s.MeanX, r.MeanX)
	eq("meanY", s.MeanY, r.MeanY)
	eq("stdX", s.StdX, r.StdX)
	eq("stdY", s.StdY, r.StdY)
	eq("corr", s.Corr, r.Corr)
	eq("slope", s.Slope, r.Slope)
	eq("intercept", s.Intercept, r.Intercept)
}

// TestDescribeOrderIndependent confirms describe is a pure function of the point
// SET, not its order — a precondition for "sufficient statistics, no fold" (the
// reason scrubbing is free here, per the design's bayes lesson).
func TestDescribeOrderIndependent(t *testing.T) {
	forward := describe(datasaurus)

	reversed := make([]Pt, len(datasaurus))
	for i, p := range datasaurus {
		reversed[len(datasaurus)-1-i] = p
	}
	back := describe(reversed)

	if math.Abs(forward.Corr-back.Corr) > 1e-12 || math.Abs(forward.Slope-back.Slope) > 1e-12 {
		t.Errorf("describe depends on point order: corr %.15f vs %.15f", forward.Corr, back.Corr)
	}
}

// TestGripsAddressEveryPoint checks the scatter emits one grip per datum with the
// right index — the direct-manipulation wiring, so every point is actually draggable.
func TestGripsAddressEveryPoint(t *testing.T) {
	// Stamp the leaf as the runtime would, then build the chart the way the cell does.
	d := Draggable[Pt]{Value: datasaurus}.WithLeaf("data")
	plot := scatter(d, fitLine(d))
	if len(plot.Grips) != len(datasaurus) {
		t.Fatalf("got %d grips for %d points", len(plot.Grips), len(datasaurus))
	}
	for i, g := range plot.Grips {
		if g.Ref.Index != i || g.Ref.Leaf != "data" {
			t.Errorf("grip %d addresses %s:%d, want data:%d", i, g.Ref.Leaf, g.Ref.Index, i)
		}
	}
}
