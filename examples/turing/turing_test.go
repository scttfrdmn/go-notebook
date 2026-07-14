package turing

import (
	"math"
	"testing"
)

// defaults mirror the notebook's slider defaults (feed 0.030, kill 0.057, 6000 steps).
const (
	defF, defK, defN = 30, 57, 6000
)

func run(f, k, n int) Field { return simulate(float64(f)/1000, float64(k)/1000, n) }

// TestPatternDevelops is the teaching claim, numerically: at the default parameters
// a real Turing pattern forms — it fills a substantial fraction of the grid and
// reaches the edges, rather than staying the tiny seed square. If this regresses
// (as it did at the first-guessed defaults, ~1% coverage), the demo shows a dot,
// not a pattern, and must not ship.
func TestPatternDevelops(t *testing.T) {
	fld := run(defF, defK, defN)

	var active, edge int
	for i, v := range fld.V {
		if v > 0.2 {
			active++
		}
		x, y := i%grid, i/grid
		if (x < 4 || x > grid-5 || y < 4 || y > grid-5) && v > 0.2 {
			edge++
		}
	}
	coverage := float64(active) / float64(len(fld.V))
	if coverage < 0.15 {
		t.Errorf("pattern coverage %.1f%% too low — it never developed past the seed", coverage*100)
	}
	if edge == 0 {
		t.Errorf("pattern never reached the grid edges — it's a localized dot, not a Turing pattern")
	}
}

// TestSimulatePure confirms simulate is a pure function of (feed, kill, steps): the
// same inputs give a bit-identical field. This is what lets the step slider scrub
// backward exactly — the design's no-fold, pure-cell property.
func TestSimulatePure(t *testing.T) {
	a := run(defF, defK, 1500)
	b := run(defF, defK, 1500)
	for i := range a.V {
		if a.V[i] != b.V[i] {
			t.Fatalf("simulate is not pure: cell %d differs (%v vs %v)", i, a.V[i], b.V[i])
		}
	}
}

// TestParametersMatter guards that the two numbers actually change the outcome — a
// different kill rate must produce a materially different field, or the sliders are
// decoration.
func TestParametersMatter(t *testing.T) {
	a := run(defF, defK, defN)
	b := run(defF, defK+8, defN) // a different regime

	var diff float64
	for i := range a.V {
		diff += math.Abs(a.V[i] - b.V[i])
	}
	mean := diff / float64(len(a.V))
	if mean < 0.02 {
		t.Errorf("changing kill rate barely changed the field (mean |Δ|=%.4f) — the sliders don't bite", mean)
	}
}

// TestFieldBounded confirms concentrations stay in a sane [0,1.05] range — Gray-Scott
// should not blow up at these parameters; a NaN or a runaway would mean the stencil
// or timestep is wrong.
func TestFieldBounded(t *testing.T) {
	fld := run(defF, defK, defN)
	for i, v := range fld.V {
		if math.IsNaN(v) || v < -0.01 || v > 1.05 {
			t.Fatalf("field out of range at %d: %v", i, v)
		}
	}
}
