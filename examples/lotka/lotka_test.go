package lotka

import (
	"math"
	"strings"
	"testing"
)

// dflt builds a trajectory at the notebook's default parameters.
func dflt() Trajectory {
	return trajectory(110, 40, 100, 30, Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}, 200)
}

// TestTrajectoryPure confirms the run is a pure function of its inputs — same
// params + start + span → identical trajectory — so every slider scrubs exactly.
func TestTrajectoryPure(t *testing.T) {
	a := dflt()
	b := dflt()
	if len(a.Prey) != len(b.Prey) {
		t.Fatalf("length differs")
	}
	for i := range a.Prey {
		if a.Prey[i] != b.Prey[i] || a.Pred[i] != b.Pred[i] {
			t.Fatalf("trajectory not pure at %d", i)
		}
	}
}

// TestOscillates is the core physics claim: both populations rise and crash (a real
// cycle), not settle to a flat line or blow up. Guards a measurable period.
func TestOscillates(t *testing.T) {
	run := dflt()
	if p := run.periodEstimate(); p <= 0 {
		t.Errorf("no oscillation detected (period=%v); Lotka-Volterra should cycle", p)
	}
	preyMax, predMax := run.peaks()
	preyMin := math.Inf(1)
	for _, v := range run.Prey {
		preyMin = math.Min(preyMin, v)
	}
	// A real oscillation swings; a settled or dead run wouldn't.
	if preyMax < 2*preyMin {
		t.Errorf("prey barely oscillates: min %.2f max %.2f", preyMin, preyMax)
	}
	if predMax <= 0 {
		t.Errorf("predators died out entirely (max %.2f)", predMax)
	}
}

// TestOrbitCloses is the conservation claim — the phase portrait's loop. After a
// whole number of periods the state should return near where it started. We run to
// the first prey peak and confirm the state comes back close to that peak later,
// i.e. the orbit is closed rather than spiraling in or out.
func TestOrbitCloses(t *testing.T) {
	run := dflt()
	// find the first two prey peaks; the state at each should be similar.
	var peaks []int
	for i := 1; i < len(run.Prey)-1; i++ {
		if run.Prey[i] > run.Prey[i-1] && run.Prey[i] >= run.Prey[i+1] {
			peaks = append(peaks, i)
		}
	}
	if len(peaks) < 2 {
		t.Fatalf("need at least two peaks to test closure, got %d", len(peaks))
	}
	p0, p1 := peaks[0], peaks[1]
	// prey and predator values one period apart should be close (RK4 on a
	// conservative system drifts only slightly).
	dPrey := math.Abs(run.Prey[p0] - run.Prey[p1])
	dPred := math.Abs(run.Pred[p0] - run.Pred[p1])
	if dPrey > 0.5 || dPred > 0.5 {
		t.Errorf("orbit does not close: prey Δ=%.3f pred Δ=%.3f over one period (should be ~0)", dPrey, dPred)
	}
}

// TestParametersBite confirms changing a rate changes the dynamics — a different
// predator death rate γ gives a materially different trajectory.
func TestParametersBite(t *testing.T) {
	a := trajectory(110, 40, 100, 30, Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}, 200)
	b := trajectory(110, 40, 160, 30, Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}, 200)
	var diff float64
	for i := range a.Prey {
		diff += math.Abs(a.Prey[i] - b.Prey[i])
	}
	if diff/float64(len(a.Prey)) < 0.5 {
		t.Errorf("changing γ barely changed the trajectory (mean |Δprey|=%.3f)", diff/float64(len(a.Prey)))
	}
}

// TestLinkedViews confirms both views are projections of the SAME trajectory: the
// series' arrays are the run's arrays, and the portrait plots the same points with
// the draggable start grip attached. This is the reactive-graph fork made testable.
func TestLinkedViews(t *testing.T) {
	ic := Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}.WithLeaf("ic")
	run := trajectory(110, 40, 100, 30, ic, 200)

	s := series(run)
	if len(s.Prey) != len(run.Prey) || len(s.Pred) != len(run.Pred) {
		t.Error("series is not the trajectory's own arrays")
	}
	p := portrait(run, ic)
	if len(p.Prey) != len(run.Prey) {
		t.Error("portrait is not the trajectory's own arrays")
	}
	// the phase grip addresses the start leaf, index 0.
	if p.Grip.Ref.Leaf != "ic" || p.Grip.Ref.Index != 0 {
		t.Errorf("phase grip addresses %s:%d, want ic:0", p.Grip.Ref.Leaf, p.Grip.Ref.Index)
	}
	if p.Grip.At != (Pt{X: 10, Y: 5}) {
		t.Errorf("grip sits at %v, want the start point", p.Grip.At)
	}
}

// TestChartsRenderSVG confirms both views render as SVG the client can inject.
func TestChartsRenderSVG(t *testing.T) {
	run := dflt()
	ic := Draggable[Pt]{Value: []Pt{{X: 10, Y: 5}}}.WithLeaf("ic")
	for name, data := range map[string]string{
		"series":   series(run).Render().Data,
		"portrait": portrait(run, ic).Render().Data,
	} {
		if !strings.HasPrefix(data, "<svg") {
			t.Errorf("%s did not render an SVG", name)
		}
	}
	// only the portrait carries a grip.
	if !strings.Contains(portrait(run, ic).Render().Data, "data-grip") {
		t.Error("portrait is missing its draggable grip")
	}
}
