package clt

import (
	"math"
	"strings"
	"testing"
)

// pop builds the default draggable population.
func pop() Draggable[Pt] { return population() }

// histStats returns the mean and SD of a density histogram over [0, bins).
func histStats(hist []float64) (mean, sd float64) {
	mb := len(hist)
	binW := float64(bins) / float64(mb)
	var mass, m float64
	for i, h := range hist {
		x := (float64(i) + 0.5) * binW
		mass += h * binW
		m += x * h * binW
	}
	m /= mass
	var v float64
	for i, h := range hist {
		x := (float64(i) + 0.5) * binW
		v += (x - m) * (x - m) * h * binW
	}
	return m, math.Sqrt(v / mass)
}

// TestSamplingPure is the load-bearing claim: the sampling distribution is a pure
// function of its inputs, so scrubbing n is exact and reversible. Same inputs →
// identical histogram, and — the reversibility that a fold couldn't give — the
// histogram at a given n is the same whether you arrived by going up or down.
func TestSamplingPure(t *testing.T) {
	d := distribution(pop())
	a := sampling(d, 5, 3000, 12)
	b := sampling(d, 5, 3000, 12)
	for i := range a.Hist {
		if a.Hist[i] != b.Hist[i] {
			t.Fatalf("sampling not pure at bin %d", i)
		}
	}
	// n=5 reached "from above" (recomputed) is identical to n=5 reached "from below"
	// — there is no history, so the path doesn't matter. This is the un-converge.
	_ = sampling(d, 20, 3000, 12) // simulate having scrubbed up first
	c := sampling(d, 5, 3000, 12)
	for i := range a.Hist {
		if a.Hist[i] != c.Hist[i] {
			t.Fatalf("n=5 differs by path at bin %d — not reversible", i)
		}
	}
}

// TestCLTWidthShrinksAsSqrtN is the theorem itself, on the empirical histogram (not
// just the formula): the sample-mean distribution's measured SD tracks σ/√n. If the
// sampling code were wrong, the histogram would not match the prediction.
func TestCLTWidthShrinksAsSqrtN(t *testing.T) {
	d := distribution(pop())
	for _, n := range []int{4, 16, 36} {
		s := sampling(d, n, 6000, 7)
		_, emp := histStats(s.Hist)
		pred := d.SD / math.Sqrt(float64(n))
		if s.PredSD != pred {
			t.Errorf("n=%d: reported PredSD %.4f != σ/√n %.4f", n, s.PredSD, pred)
		}
		// empirical SD within 12% of prediction (histogram binning + sampling noise)
		if rel := math.Abs(emp-pred) / pred; rel > 0.12 {
			t.Errorf("n=%d: empirical SD %.3f vs predicted %.3f (%.0f%% off) — CLT not matched", n, emp, pred, rel*100)
		}
	}
}

// TestMeanIsUnbiased: the sampling distribution is centred on the population mean,
// for every n (the CLT centres the mean on μ regardless of n).
func TestMeanIsUnbiased(t *testing.T) {
	d := distribution(pop())
	for _, n := range []int{1, 8, 30} {
		s := sampling(d, n, 6000, 3)
		emp, _ := histStats(s.Hist)
		if math.Abs(emp-d.Mean) > 0.15 {
			t.Errorf("n=%d: sample-mean distribution centred at %.3f, population μ=%.3f", n, emp, d.Mean)
		}
	}
}

// TestDistributionMomentsClosedForm checks the population's own μ/σ against a direct
// computation, since the notebook relies on the closed form (each bar a uniform
// slab) rather than sampling for the population statistics.
func TestDistributionMomentsClosedForm(t *testing.T) {
	d := distribution(pop())
	// recompute μ independently
	var total, mu float64
	seedH := []float64{0.20, 0.95, 0.70, 0.18, 0.10, 0.10, 0.12, 0.20, 0.65, 0.90, 0.35, 0.12}
	for _, h := range seedH {
		total += h
	}
	for i, h := range seedH {
		mu += h / total * (float64(i) + 0.5)
	}
	if math.Abs(d.Mean-mu) > 1e-9 {
		t.Errorf("population mean %.6f != independent %.6f", d.Mean, mu)
	}
	if d.SD <= 0 {
		t.Errorf("population SD must be positive, got %.4f", d.SD)
	}
}

// TestFlattenedPopulationFallsBackUniform: dragging every bar to zero shouldn't
// divide by zero — it falls back to a uniform population.
func TestFlattenedPopulationFallsBackUniform(t *testing.T) {
	flat := make([]Pt, bins)
	for i := range flat {
		flat[i] = Pt{X: float64(i) + 0.5, Y: 0}
	}
	d := distribution(Draggable[Pt]{Value: flat})
	// uniform over [0,bins): mean = bins/2
	if math.Abs(d.Mean-float64(bins)/2) > 1e-9 {
		t.Errorf("flattened population mean %.4f, want %.1f (uniform fallback)", d.Mean, float64(bins)/2)
	}
}

// TestViewsRenderAndGripEveryBar: both panels render SVG, and the population panel
// carries one draggable grip per bar.
func TestViewsRenderAndGrip(t *testing.T) {
	p := population().WithLeaf("pop")
	d := distribution(p)
	pv := populationView(p, d).Render()
	if !strings.HasPrefix(pv.Data, "<svg") {
		t.Error("population view is not SVG")
	}
	if got := strings.Count(pv.Data, "data-grip"); got != bins {
		t.Errorf("population view has %d grips, want %d (one per bar)", got, bins)
	}
	mv := samplingView(sampling(d, 5, 1000, 1)).Render()
	if !strings.HasPrefix(mv.Data, "<svg") {
		t.Error("sampling view is not SVG")
	}
}
