package chart

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want, tol float64, what string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.6f, want %.6f (tol %g)", what, got, want, tol)
	}
}

func TestMean(t *testing.T) {
	approx(t, Mean([]float64{1, 2, 3, 4}), 2.5, 1e-9, "mean")
	approx(t, Mean(nil), 0, 0, "mean empty")
	approx(t, Mean([]float64{7}), 7, 1e-9, "mean single")
}

func TestStd(t *testing.T) {
	// Population std of {2,4,4,4,5,5,7,9} is 2 (classic textbook example).
	approx(t, Std([]float64{2, 4, 4, 4, 5, 5, 7, 9}), 2, 1e-9, "std")
	approx(t, Std([]float64{5}), 0, 0, "std single")
	approx(t, Std(nil), 0, 0, "std empty")
}

func TestQuantile(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	approx(t, Quantile(xs, 0.5), 3, 1e-9, "median odd")
	approx(t, Quantile(xs, 0), 1, 1e-9, "min")
	approx(t, Quantile(xs, 1), 5, 1e-9, "max")
	// Type-7 linear interpolation: p=0.25 on [1..5] → pos=1.0 → 2.
	approx(t, Quantile(xs, 0.25), 2, 1e-9, "q1")
	// Even count median interpolates.
	approx(t, Quantile([]float64{1, 2, 3, 4}, 0.5), 2.5, 1e-9, "median even")
	// Does not mutate input.
	in := []float64{3, 1, 2}
	_ = Quantile(in, 0.5)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("Quantile mutated its input: %v", in)
	}
	approx(t, Quantile(nil, 0.5), 0, 0, "quantile empty")
}

func TestCorr(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	up := []float64{2, 4, 6, 8, 10}
	approx(t, Corr(xs, up), 1, 1e-9, "perfect positive")
	down := []float64{10, 8, 6, 4, 2}
	approx(t, Corr(xs, down), -1, 1e-9, "perfect negative")
	approx(t, Corr(xs, []float64{5, 5, 5, 5, 5}), 0, 1e-9, "constant y")
	approx(t, Corr(xs, []float64{1, 2}), 0, 0, "mismatched length")
}

func TestLinFit(t *testing.T) {
	// y = 3x + 1 exactly.
	xs := []float64{0, 1, 2, 3}
	ys := []float64{1, 4, 7, 10}
	slope, intercept := LinFit(xs, ys)
	approx(t, slope, 3, 1e-9, "slope")
	approx(t, intercept, 1, 1e-9, "intercept")
	// Vertical / no x-variation → zero line.
	s, i := LinFit([]float64{2, 2, 2}, []float64{1, 2, 3})
	if s != 0 || i != 0 {
		t.Errorf("LinFit with no x-variation: got (%v,%v), want (0,0)", s, i)
	}
}

func TestFmtNum(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1000, "1,000"},
		{20000, "20,000"},
		{6452.5, "6,452.5"},
		{-1234.25, "-1,234.25"},
		{4.5, "4.5"},
		{12, "12"},
		{0.033, "0.033"},
		{999.9, "999.9"},
	}
	for _, c := range cases {
		if got := fmtNum(c.in); got != c.want {
			t.Errorf("fmtNum(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
