package aimd

import (
	"strings"
	"testing"
)

const (
	multiplicative = 0
	additive       = 1
)

// TestSingleIsPure: the fold is a pure function of its inputs — deterministic loss, no
// RNG, so the sawtooth is byte-identical across runs and scrubs exactly.
func TestSingleIsPure(t *testing.T) {
	a := single(50, 1, multiplicative)
	b := single(50, 1, multiplicative)
	for i := range a.Cwnd {
		if a.Cwnd[i] != b.Cwnd[i] {
			t.Fatalf("single not pure at tick %d: %v vs %v", i, a.Cwnd[i], b.Cwnd[i])
		}
	}
}

// TestSawtoothShape: the window climbs additively until it overruns the pipe, then
// drops multiplicatively — the defining sawtooth. Assert there IS a drop (a loss), that
// the drop is a cut not a climb, and that between drops the window only increases.
func TestSawtoothShape(t *testing.T) {
	f := single(50, 1, multiplicative)
	if f.Losses < 2 {
		t.Fatalf("expected multiple sawtooth teeth over the horizon, got %d losses", f.Losses)
	}
	drops, climbs := 0, 0
	for i := 1; i < len(f.Cwnd); i++ {
		switch {
		case f.Cwnd[i] < f.Cwnd[i-1]:
			drops++
			// a multiplicative cut of β=0.5 roughly halves; assert it's a real cut.
			if f.Cwnd[i] > f.Cwnd[i-1]*0.75 {
				t.Errorf("drop at %d too shallow for β=0.5: %.1f → %.1f", i, f.Cwnd[i-1], f.Cwnd[i])
			}
		case f.Cwnd[i] > f.Cwnd[i-1]:
			climbs++
		}
	}
	if drops != f.Losses {
		t.Errorf("every loss should be one downward tooth: %d drops vs %d losses", drops, f.Losses)
	}
	if climbs < drops {
		t.Error("the window should spend most ticks climbing between teeth")
	}
}

// TestGentlerBetaRaisesUtilization: a gentler decrease (higher β) keeps the window
// nearer capacity, so utilization rises monotonically with β. The β tuning lever.
func TestGentlerBetaRaisesUtilization(t *testing.T) {
	u30 := single(30, 1, multiplicative).Utilization
	u50 := single(50, 1, multiplicative).Utilization
	u90 := single(90, 1, multiplicative).Utilization
	if !(u30 < u50 && u50 < u90) {
		t.Errorf("utilization should rise with β: β30=%.3f β50=%.3f β90=%.3f", u30, u50, u90)
	}
	if u90 >= 1.0 {
		t.Errorf("even a gentle sawtooth can't hit 100%% utilization, got %.3f", u90)
	}
}

// TestMultiplicativeConvergesToFair is the deep claim: two flows started far apart
// converge to an equal share under multiplicative decrease — the gap collapses toward
// zero, and it shrinks over time (not just small at the end by luck).
func TestMultiplicativeConvergesToFair(t *testing.T) {
	p := pair(50, 1, multiplicative)
	if p.FinalGap > 1.0 {
		t.Errorf("multiplicative decrease should converge to fair (gap→0), final gap %.2f", p.FinalGap)
	}
	// the gap early in the run should be much larger than at the end.
	early := p.Gap[len(p.Gap)/10]
	if !(early > p.FinalGap+2) {
		t.Errorf("the fairness gap should shrink over time: early %.2f, final %.2f", early, p.FinalGap)
	}
}

// TestAdditiveStaysUnfair is the counterexample that proves WHY decrease must be
// multiplicative: additive decrease subtracts a constant from both flows, leaving their
// difference untouched — so two flows started unequal STAY unequal. If this ever
// "converges," the Chiu–Jain teaching is a lie and the toggle proves nothing.
func TestAdditiveStaysUnfair(t *testing.T) {
	p := pair(50, 1, additive)
	if p.FinalGap <= 1.0 {
		t.Errorf("additive decrease must NOT converge to fair — gap should persist, got %.2f", p.FinalGap)
	}
	// and it should be dramatically less fair than multiplicative on the same start.
	mult := pair(50, 1, multiplicative)
	if !(p.FinalGap > mult.FinalGap+5) {
		t.Errorf("additive (%.2f) should be far less fair than multiplicative (%.2f)", p.FinalGap, mult.FinalGap)
	}
}

// TestViewsRender: both charts are SVG with the capacity line and their labels; the
// verdict readout is HTML (else the client hides it — the pid/Readout lesson).
func TestViewsRender(t *testing.T) {
	f := single(50, 1, multiplicative)
	p := pair(50, 1, multiplicative)

	saw := sawtooth(f).Render()
	if !strings.HasPrefix(saw.Data, "<svg") {
		t.Fatal("sawtooth not SVG")
	}
	for _, w := range []string{"sawtooth", "pipe capacity", "cwnd"} {
		if !strings.Contains(saw.Data, w) {
			t.Errorf("sawtooth missing %q", w)
		}
	}

	fair := fairness(p).Render()
	for _, w := range []string{"flow A", "flow B", "fair share"} {
		if !strings.Contains(fair.Data, w) {
			t.Errorf("fairness chart missing %q", w)
		}
	}

	vd := verdict(f, p).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html", vd.MIME)
	}
	for _, w := range []string{"utilization", "decrease rule", "fairness"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
