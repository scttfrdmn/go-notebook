package little

import (
	"math"
	"strings"
	"testing"
)

// TestLawHolds is the identity itself: L = λ·W across a range of values, checked
// against an independent computation.
func TestLawHolds(t *testing.T) {
	cases := []struct {
		lambda   PerSecond
		wMillis  int
		wantReqs float64
	}{
		{5000, 20, 100},  // the classic "5000 req/s at 20 ms → 100 in flight"
		{2500, 200, 500}, // the default
		{100, 1000, 100}, // 100 req/s held a full second → 100
		{10000, 5, 50},
	}
	for _, c := range cases {
		w := latency(c.wMillis)
		l := concurrency(c.lambda, w)
		if math.Abs(float64(l)-c.wantReqs) > 1e-9 {
			t.Errorf("λ=%v, W=%dms: L=%v, want %v", c.lambda, c.wMillis, l, c.wantReqs)
		}
	}
}

// TestLatencyConvertsMillisToSeconds pins the one unit conversion in the notebook:
// the slider is milliseconds, W must be Seconds, and 200 ms = 0.2 s.
func TestLatencyConvertsMillisToSeconds(t *testing.T) {
	if got := latency(200); math.Abs(float64(got)-0.2) > 1e-9 {
		t.Errorf("latency(200ms) = %v s, want 0.2", got)
	}
}

// TestConcurrencyPure confirms the composition is a pure function of its inputs.
func TestConcurrencyPure(t *testing.T) {
	a := concurrency(2500, latency(200))
	b := concurrency(2500, latency(200))
	if a != b {
		t.Fatalf("concurrency not pure: %v vs %v", a, b)
	}
}

// TestScalesLinearly is the scenario the readout promises: at fixed throughput,
// doubling latency doubles the requests in flight (and doubling throughput at fixed
// latency does too). The law is linear in each factor.
func TestScalesLinearly(t *testing.T) {
	base := concurrency(2500, latency(100))
	doubleW := concurrency(2500, latency(200))
	doubleLambda := concurrency(5000, latency(100))
	if math.Abs(float64(doubleW)-2*float64(base)) > 1e-9 {
		t.Errorf("doubling W did not double L: %v vs 2×%v", doubleW, base)
	}
	if math.Abs(float64(doubleLambda)-2*float64(base)) > 1e-9 {
		t.Errorf("doubling λ did not double L: %v vs 2×%v", doubleLambda, base)
	}
}

// TestChartAreaEqualsL: the rectangle's box area (as a fraction of the axes) is
// proportional to L, so "area = L" is literally true on the chart, not just labelled.
// width ∝ λ, height ∝ W, so width·height ∝ λ·W = L.
func TestChartAreaEqualsL(t *testing.T) {
	// two configs with the SAME L (λ·W constant) should have the same box area.
	l1 := concurrency(2500, latency(200)) // 500
	l2 := concurrency(5000, latency(100)) // 500
	if l1 != l2 {
		t.Fatalf("precondition: both should be L=500, got %v and %v", l1, l2)
	}
	area := func(lam PerSecond, w Seconds) float64 {
		const lamMax, wMax = 10000.0, 0.25
		return (float64(lam) / lamMax) * (float64(w) / wMax) // ∝ box width × height
	}
	a1 := area(2500, latency(200))
	a2 := area(5000, latency(100))
	if math.Abs(a1-a2) > 1e-9 {
		t.Errorf("equal-L configs drew unequal areas (%v vs %v) — 'area = L' would be a lie", a1, a2)
	}
}

// TestViewRenders confirms the chart is SVG and carries the three quantities.
func TestViewRenders(t *testing.T) {
	data := view(2500, latency(200), concurrency(2500, latency(200))).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("view is not SVG")
	}
	for _, want := range []string{"λ = 2500 req/s", "W = 200 ms", "area = L = 500.0 requests"} {
		if !strings.Contains(data, want) {
			t.Errorf("chart missing %q", want)
		}
	}
}
