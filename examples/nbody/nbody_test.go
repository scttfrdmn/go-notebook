package nbody

import (
	"math"
	"testing"
)

// seed is the notebook's default initial condition (must match bodies()).
var seed = []Pt{{35, 45}, {65, 52}, {52, 68}}

// TestVerletConservesEulerDoesNot is the teaching claim, proven numerically: on the
// identical initial condition and step size, velocity Verlet holds total energy
// nearly constant while forward Euler drifts substantially. If this ever inverts,
// the notebook's whole point is false and the demo must not ship.
func TestVerletConservesEulerDoesNot(t *testing.T) {
	const n, h = 1500, 0.012 // the notebook defaults (12 ms)

	e := integrate(seed, n, h, stepEuler)
	v := integrate(seed, n, h, stepVerlet)

	// The system must be BOUND (total energy < 0) — that's the regime where Euler's
	// energy pumping is visible instead of the bodies simply flying apart.
	if e.Energy[0] >= 0 {
		t.Fatalf("initial energy %.3f is not bound (<0); Euler drift won't show", e.Energy[0])
	}

	eulerDrift := math.Abs(relDrift(e.Energy))
	verletDrift := math.Abs(relDrift(v.Energy))

	// Verlet is essentially flat.
	if verletDrift > 0.005 {
		t.Errorf("Verlet drift %.3f%% too large; symplectic integrator should conserve", verletDrift*100)
	}
	// Euler visibly drifts — the exhibit. Guard a real, human-visible gap.
	if eulerDrift < 0.02 {
		t.Errorf("Euler drift %.3f%% too small to see; the exhibit needs a visible gap", eulerDrift*100)
	}
	// And Euler must be at least an order of magnitude worse than Verlet.
	if eulerDrift < 10*verletDrift {
		t.Errorf("Euler drift %.4f not clearly worse than Verlet %.4f", eulerDrift, verletDrift)
	}
}

// TestDriftGrowsWithStepSize checks the interaction the slider promises: a bigger dt
// makes Euler drift worse. If turning the knob up didn't worsen it, the caption lies.
func TestDriftGrowsWithStepSize(t *testing.T) {
	small := math.Abs(relDrift(integrate(seed, 1500, 0.012, stepEuler).Energy))
	large := math.Abs(relDrift(integrate(seed, 1500, 0.024, stepEuler).Energy))
	if large <= small {
		t.Errorf("Euler drift did not grow with step size: dt=12ms %.3f%% vs dt=24ms %.3f%%", small*100, large*100)
	}
}

// TestTrajectoryPure confirms integrate is a pure function of its inputs — the
// property that lets the step slider scrub backward exactly (bayes' lesson): the
// same initial condition run twice gives bit-identical energy series.
func TestTrajectoryPure(t *testing.T) {
	a := integrate(seed, 800, 0.012, stepVerlet)
	b := integrate(seed, 800, 0.012, stepVerlet)
	if len(a.Energy) != len(b.Energy) {
		t.Fatalf("length differs: %d vs %d", len(a.Energy), len(b.Energy))
	}
	for i := range a.Energy {
		if a.Energy[i] != b.Energy[i] {
			t.Fatalf("integrate is not pure: sample %d differs (%v vs %v)", i, a.Energy[i], b.Energy[i])
		}
	}
}

// TestPrefixStability confirms a shorter horizon is an exact prefix of a longer one —
// so scrubbing the step slider down shows the true earlier state, not an approximation.
func TestPrefixStability(t *testing.T) {
	short := integrate(seed, 300, 0.012, stepVerlet)
	long := integrate(seed, 1000, 0.012, stepVerlet)
	for i := 0; i <= 300; i++ {
		if short.Energy[i] != long.Energy[i] {
			t.Fatalf("horizon is not a prefix at sample %d: %v vs %v", i, short.Energy[i], long.Energy[i])
		}
	}
}

// TestGripsAddressEveryBody checks the orbit chart emits one draggable grip per body.
func TestGripsAddressEveryBody(t *testing.T) {
	d := Draggable[Pt]{Value: seed}.WithLeaf("start")
	paths := orbits(d, euler(d, 100, 12), verlet(d, 100, 12))
	if len(paths.Grips) != len(seed) {
		t.Fatalf("got %d grips for %d bodies", len(paths.Grips), len(seed))
	}
	for i, g := range paths.Grips {
		if g.Ref.Index != i || g.Ref.Leaf != "start" {
			t.Errorf("grip %d addresses %s:%d, want start:%d", i, g.Ref.Leaf, g.Ref.Index, i)
		}
	}
}
