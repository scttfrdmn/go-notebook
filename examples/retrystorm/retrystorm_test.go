package retrystorm

import (
	"strings"
	"testing"
)

// TestStormIsPure: storm is a pure function of its inputs (fixed-horizon-pure, no
// RNG) — same params ⇒ byte-identical trace, so scrubbing re-runs from t=0 and the
// phase loop is stable.
func TestStormIsPure(t *testing.T) {
	a := storm(150, 85, 100)
	b := storm(150, 85, 100)
	for i := range a.Good {
		if a.Good[i] != b.Good[i] || a.Offered[i] != b.Offered[i] {
			t.Fatalf("storm not pure at tick %d", i)
		}
	}
}

// TestNoRetriesNoHysteresis is the control, and it draws the precise line the notebook
// teaches: a raw overload (retry=0) can push goodput down while the load is high, but
// it is NOT metastable — it recovers at the same load it degraded at, so there is no
// hysteresis gap. The *retries* are what turn a recoverable overload into a trap. (An
// earlier draft of this test asserted "no collapse at all" — the probe showed the
// overload cliff exists without retries; only the hysteresis is retry-borne.)
func TestNoRetriesNoHysteresis(t *testing.T) {
	r := storm(200, 0, 100) // heavy overload, but no retry feedback
	gap := r.GapUp - r.GapDown
	if gap > 5 {
		t.Errorf("with retry=0 there should be no hysteresis (recover ≈ collapse load): up=%.0f down=%.0f gap=%.0f",
			r.GapUp, r.GapDown, gap)
	}
	// goodput still peaks near capacity — the server isn't broken, it's just saturated.
	if r.PeakGood < capacity*0.9 {
		t.Errorf("peak goodput %.1f, want ≈capacity %.0f", r.PeakGood, capacity)
	}
}

// TestRetriesCauseMetastableCollapse is THE teaching claim: crank the retry gain and
// the same load that a no-retry system rides out instead tips the system into
// collapse — AND it stays collapsed on the way down (hysteresis: recovers at a much
// lower offered load than it collapsed at).
func TestRetriesCauseMetastableCollapse(t *testing.T) {
	r := storm(150, 85, 100) // default: retries on, breaker off
	if !r.Collapsed {
		t.Fatal("with retries and a load past capacity, the system should collapse")
	}
	if r.GapUp <= 0 || r.GapDown <= 0 {
		t.Fatalf("expected both a collapse point (up) and a recovery point (down), got up=%.0f down=%.0f",
			r.GapUp, r.GapDown)
	}
	// Hysteresis: it collapses at a HIGH offered load going up, but stays collapsed
	// until a MUCH LOWER offered load coming down. The gap is the trap.
	if r.GapUp <= r.GapDown {
		t.Errorf("no hysteresis: collapse@%.0f (up) should exceed recover@%.0f (down)", r.GapUp, r.GapDown)
	}
	if gap := r.GapUp - r.GapDown; gap < 30 {
		t.Errorf("hysteresis gap %.0f too narrow at default params — the trap should be wide", gap)
	}
}

// TestMoreRetriesWidensTheTrap: retry % is the gain on the loop, so raising it widens
// the hysteresis gap (recovery happens at an even lower load). Monotone in the right
// direction.
func TestMoreRetriesWidensTheTrap(t *testing.T) {
	lo := storm(150, 60, 100)
	hi := storm(150, 95, 100)
	gapLo := lo.GapUp - lo.GapDown
	gapHi := hi.GapUp - hi.GapDown
	if !(gapHi > gapLo) {
		t.Errorf("more retries should widen the trap: gap@60%%=%.0f, gap@95%%=%.0f", gapLo, gapHi)
	}
}

// TestBreakerClosesTheLoop is the fix, stated honestly: arming the circuit breaker
// does not stop the tip-over (it still collapses under peak load), but it lets the
// system RECOVER — the recovery point jumps back up, closing the hysteresis gap.
func TestBreakerClosesTheLoop(t *testing.T) {
	off := storm(150, 85, 100) // breaker disabled
	on := storm(150, 85, 50)   // breaker trips at 50% failure rate

	if on.GapDown <= off.GapDown {
		t.Errorf("the breaker should raise the recovery point (climb back out sooner): off=%.0f on=%.0f",
			off.GapDown, on.GapDown)
	}
	offGap := off.GapUp - off.GapDown
	onGap := on.GapUp - on.GapDown
	if !(onGap < offGap) {
		t.Errorf("the breaker should shrink the hysteresis gap: off=%.0f on=%.0f", offGap, onGap)
	}
}

// TestLoadProfileIsATriangle: deterministic 0 → peak → 0, peaking at the midpoint.
func TestLoadProfileIsATriangle(t *testing.T) {
	p := loadProfile(150)
	if p[0] > 1 {
		t.Errorf("profile should start near 0, got %.1f", p[0])
	}
	mid := p[ticks/2]
	if mid < 140 {
		t.Errorf("profile should peak near 150 at the midpoint, got %.1f", mid)
	}
	if p[ticks-1] > 5 {
		t.Errorf("profile should return near 0, got %.1f", p[ticks-1])
	}
}

// TestViewsRender: both linked views are SVG and carry their orienting labels; the
// verdict readout is HTML (else the client hides it — the pid lesson).
func TestViewsRender(t *testing.T) {
	run := storm(150, 85, 100)

	tl := timeline(run).Render()
	if !strings.HasPrefix(tl.Data, "<svg") {
		t.Fatal("timeline not SVG")
	}
	for _, w := range []string{"offered", "goodput", "capacity"} {
		if !strings.Contains(tl.Data, w) {
			t.Errorf("timeline missing %q", w)
		}
	}

	ph := phase(run).Render()
	if !strings.HasPrefix(ph.Data, "<svg") {
		t.Fatal("phase not SVG")
	}
	for _, w := range []string{"load rising", "load falling", "hysteresis"} {
		if !strings.Contains(ph.Data, w) {
			t.Errorf("phase loop missing %q", w)
		}
	}

	vd := verdict(run).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html (else the client hides it)", vd.MIME)
	}
	for _, w := range []string{"outcome", "hysteresis gap", "METASTABLE"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
