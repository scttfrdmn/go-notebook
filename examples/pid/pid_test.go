package pid

import (
	"strings"
	"testing"
)

// meanTail / jitterTail summarize the settled region (last third) of a trace: the
// average level, and the average tick-to-tick swing. The teaching claims are claims
// about this region — where the transient is over and the failure mode (if any) is
// what's left.
func meanTail(xs []float64) float64 {
	s, n := 0.0, 0
	for t := len(xs) * 2 / 3; t < len(xs); t++ {
		s += xs[t]
		n++
	}
	return s / float64(n)
}

func jitterTail(xs []float64) float64 {
	s, n := 0.0, 0
	for t := len(xs) * 2 / 3; t < len(xs); t++ {
		d := xs[t] - xs[t-1]
		if d < 0 {
			d = -d
		}
		s += d
		n++
	}
	return s / float64(n)
}

// TestPure: simulate is a pure function of its inputs — the fixed-horizon-pure
// contract. Same gains + load ⇒ byte-identical trace (the LCG noise is seeded by a
// constant, not the clock), so scrubbing a slider re-runs from t=0 exactly.
func TestPure(t *testing.T) {
	a := simulate(60, 30, 45, 45)
	b := simulate(60, 30, 45, 45)
	if len(a.Depth) != len(b.Depth) {
		t.Fatalf("trace length differs: %d vs %d", len(a.Depth), len(b.Depth))
	}
	for i := range a.Depth {
		if a.Depth[i] != b.Depth[i] || a.Reps[i] != b.Reps[i] {
			t.Fatalf("simulate not pure at tick %d: depth %v/%v reps %v/%v",
				i, a.Depth[i], b.Depth[i], a.Reps[i], b.Reps[i])
		}
	}
}

// TestDefaultIsWellTuned: the shipped default (Kp=0.60, Ki=0.30, Kd=0.45) is a clean
// step response — modest overshoot, quick settle, on target, calm replicas. This is
// the "good tuning" the notebook holds up as the target; if it regresses, the whole
// teaching frame is off.
func TestDefaultIsWellTuned(t *testing.T) {
	r := simulate(60, 30, 45, 45)
	if os := r.Overshoot / target * 100; os > 15 {
		t.Errorf("default overshoot %.0f%%, want ≤15%% (a clean response)", os)
	}
	if r.SettleTick < 0 || r.SettleTick > 10 {
		t.Errorf("default settles at tick %d, want a quick settle (0..10)", r.SettleTick)
	}
	if m := meanTail(r.Depth); m < target-1 || m > target+1 {
		t.Errorf("default settled mean %.1f, want ≈%.0f (on target)", m, target)
	}
	if j := jitterTail(r.Reps); j > 2 {
		t.Errorf("default replica jitter %.2f, want calm (≤2) — no thrashing at good gains", j)
	}
}

// TestNoKiDroops is the integral lesson, and it must point the RIGHT way: with a load
// increase and Ki=0, proportional control can only hold the queue where Kp·error
// covers the extra arrivals — which is ABOVE the target (droop), and it never closes.
// The prose says "above"; an earlier draft said "below" and this test is why it's
// right now.
func TestNoKiDroops(t *testing.T) {
	r := simulate(60, 0, 45, 45)
	m := meanTail(r.Depth)
	if m <= target+2 {
		t.Errorf("no-Ki settled mean %.1f, want well ABOVE target %.0f (droop)", m, target)
	}
	if r.SettleTick >= 0 {
		t.Errorf("no-Ki should never settle to the ±5%% band (it droops off-target), got settle=%d", r.SettleTick)
	}
	// The integral is what closes it: restoring Ki pulls the same load back to target.
	withKi := simulate(60, 30, 45, 45)
	if mm := meanTail(withKi.Depth); mm >= m {
		t.Errorf("adding Ki should reduce the offset: with-Ki mean %.1f not below no-Ki mean %.1f", mm, m)
	}
}

// TestHighKpRings is the proportional lesson: crank Kp and the loop overshoots hard
// and oscillates instead of settling.
func TestHighKpRings(t *testing.T) {
	def := simulate(60, 30, 45, 45)
	hi := simulate(180, 30, 10, 45)
	if hi.Overshoot <= def.Overshoot {
		t.Errorf("high-Kp overshoot %.1f should exceed default %.1f", hi.Overshoot, def.Overshoot)
	}
	if hi.SettleTick >= 0 {
		t.Errorf("high-Kp should ring, not settle; got settle=%d", hi.SettleTick)
	}
}

// TestHighKdThrashesRepsBeforeQueue is the derivative lesson, and the reason the
// replica trace is on the plot at all: as Kd climbs, the derivative differentiates
// the sensor noise and the REPLICA count saws — and it saws MORE than the queue does,
// so the autoscaler thrashes nodes while the queue dashboard still looks calm. This
// is D's own failure mode, distinct from Kp's ringing.
func TestHighKdThrashesRepsBeforeQueue(t *testing.T) {
	calm := simulate(60, 30, 45, 45) // default Kd
	flap := simulate(60, 30, 60, 45) // higher Kd, still pre-destabilization

	if jitterTail(flap.Reps) <= jitterTail(calm.Reps) {
		t.Errorf("raising Kd should increase replica jitter: %.2f (Kd=0.60) vs %.2f (Kd=0.45)",
			jitterTail(flap.Reps), jitterTail(calm.Reps))
	}
	// The teaching crux: replicas thrash MORE than the queue moves — the flap is in
	// what the controller DOES, not (yet) in what the queue IS.
	if jitterTail(flap.Reps) <= jitterTail(flap.Depth) {
		t.Errorf("high-Kd replica jitter %.2f should exceed queue jitter %.2f (thrash the controller, not the queue)",
			jitterTail(flap.Reps), jitterTail(flap.Depth))
	}
}

// TestNoiseIsDeterministic: the sensor-noise LCG is seeded by a constant, so the
// sequence is reproducible — the pull that keeps simulate() pure.
func TestNoiseIsDeterministic(t *testing.T) {
	a := lcgNoise(8, 1.0)
	b := lcgNoise(8, 1.0)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("lcgNoise not deterministic at %d: %v vs %v", i, a[i], b[i])
		}
		if a[i] < -1.0 || a[i] > 1.0 {
			t.Errorf("noise[%d]=%v out of [-amp,amp]", i, a[i])
		}
	}
}

// TestChartPlotsBothTraces: the response chart is SVG and draws both the queue-depth
// series and the replica series (the amber line the Kd lesson depends on).
func TestChartPlotsBothTraces(t *testing.T) {
	data := response(simulate(60, 30, 45, 45)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("response chart is not SVG")
	}
	for _, want := range []string{"queue depth", "replicas", "target"} {
		if !strings.Contains(data, want) {
			t.Errorf("chart missing %q label", want)
		}
	}
}

// TestMetricsRender is the "reaches the page" guard for the fix in this notebook: a
// Readout with no Render() carries no MIME and the client leaves the cell HIDDEN —
// the cards would compute and reach no one. Assert Readout renders HTML with the three
// headline numbers and their captions.
func TestMetricsRender(t *testing.T) {
	out := metrics(simulate(60, 30, 45, 45)).Render()
	if out.MIME != "text/html" {
		t.Fatalf("metrics MIME = %q, want text/html (else the client hides it)", out.MIME)
	}
	for _, want := range []string{"overshoot", "settling time", "steady-state error", "raise Ki"} {
		if !strings.Contains(out.Data, want) {
			t.Errorf("metrics card missing %q", want)
		}
	}
}
