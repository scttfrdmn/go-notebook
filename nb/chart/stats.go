package chart

import (
	"math"
	"sort"
)

// The summary statistics — the handful of numbers that describe a column. These
// are the other half of the 1%: most analysis wants a mean, a spread, a quantile,
// a correlation, and a fitted line, and nothing here pretends to be a stats
// library beyond that. All are pure functions over a []float64 (or two, for the
// bivariate pair), safe to call from a cell body or a Render.
//
// Empty-input convention: the univariate functions return 0 for an empty slice
// rather than NaN, so a summary readout shows a clean zero instead of "NaN"
// before any data arrives; Corr and LinFit return 0s when the fit is undefined.

// Mean is the arithmetic mean; 0 for an empty slice.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// Std is the population standard deviation (dividing by n, not n-1) — the spread
// of the data you have, not an estimate of a larger population's. 0 for fewer
// than two values.
func Std(xs []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}
	m := Mean(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return math.Sqrt(ss / float64(n))
}

// Quantile returns the p-quantile of xs (p in [0,1]) using linear interpolation
// between the two nearest ranks — the same method as NumPy's default and R's
// type-7. Quantile(xs, 0.5) is the median. It does not mutate xs (it sorts a
// copy). 0 for an empty slice; p is clamped to [0,1].
func Quantile(xs []float64, p float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return xs[0]
	}
	p = math.Max(0, math.Min(1, p))
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	// Rank position on [0, n-1].
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	frac := pos - float64(lo)
	return s[lo]*(1-frac) + s[hi]*frac
}

// Corr is the Pearson correlation coefficient between xs and ys — how tightly the
// two move together on a line, from -1 (perfectly opposed) through 0 (unrelated)
// to +1 (perfectly aligned). Returns 0 for mismatched lengths, fewer than two
// points, or a constant series (no variation to correlate).
func Corr(xs, ys []float64) float64 {
	n := len(xs)
	if n != len(ys) || n < 2 {
		return 0
	}
	mx, my := Mean(xs), Mean(ys)
	var sxy, sxx, syy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// LinFit returns the least-squares line y = slope·x + intercept fitting ys to xs.
// Returns (0, 0) for mismatched lengths, fewer than two points, or a vertical
// point cloud (no x-variation, so no line). Pair it with a scatter: feed the two
// endpoints back as a one-series [Line] over the same x-range to draw the fit.
func LinFit(xs, ys []float64) (slope, intercept float64) {
	n := len(xs)
	if n != len(ys) || n < 2 {
		return 0, 0
	}
	mx, my := Mean(xs), Mean(ys)
	var sxy, sxx float64
	for i := range xs {
		dx := xs[i] - mx
		sxy += dx * (ys[i] - my)
		sxx += dx * dx
	}
	if sxx == 0 {
		return 0, 0
	}
	slope = sxy / sxx
	return slope, my - slope*mx
}
