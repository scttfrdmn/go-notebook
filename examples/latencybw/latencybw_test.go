package latencybw

import (
	"math"
	"strings"
	"testing"
)

// near / fat are the two archetype links the notebook contrasts.
func near() Link { return linkA(1, 100) }    // 1 ms, 100 Mbit/s
func fat() Link  { return linkB(300, 1000) } // 300 ms, 1000 Mbit/s

// TestTransferTimeFormula: time = latency + size/bandwidth, computed in units. Check a
// hand value: 1 ms latency + 100 KB over 100 Mbit/s.
func TestTransferTimeFormula(t *testing.T) {
	l := linkA(1, 100)
	got := float64(l.timeFor(Kilobytes(100)))
	want := 1.0/1000 + (100*1024)/(100*1e6/8) // 0.001 + 0.008192
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("transfer time = %.6f, want %.6f", got, want)
	}
}

// TestSmallTransferIsLatencyBound is half the teaching claim: for a small transfer the
// near link (low latency) wins, even though the fat link has 10× the bandwidth — you
// pay the latency and you're done before bandwidth matters.
func TestSmallTransferIsLatencyBound(t *testing.T) {
	a, b := near(), fat()
	small := Kilobytes(10)
	if !(a.timeFor(small) < b.timeFor(small)) {
		t.Errorf("small transfer should favor the low-latency link: A=%.4fs B=%.4fs",
			float64(a.timeFor(small)), float64(b.timeFor(small)))
	}
}

// TestLargeTransferIsBandwidthBound is the other half: for a large transfer the fat
// pipe wins, its bandwidth having paid off the latency many times over.
func TestLargeTransferIsBandwidthBound(t *testing.T) {
	a, b := near(), fat()
	large := Kilobytes(1e6) // 1 GB
	if !(b.timeFor(large) < a.timeFor(large)) {
		t.Errorf("large transfer should favor the fat pipe: A=%.4fs B=%.4fs",
			float64(a.timeFor(large)), float64(b.timeFor(large)))
	}
}

// TestCrossoverIsWhereTheyTie is the heart of the lesson: the crossover size is exactly
// where the two links take equal time — verify both links agree there (to rounding),
// and that it sits between the latency-bound and bandwidth-bound examples above.
func TestCrossoverIsWhereTheyTie(t *testing.T) {
	a, b := near(), fat()
	x := crossoverKB(a, b)
	if x <= 0 {
		t.Fatalf("expected a positive crossover, got %.3f", x)
	}
	ta := float64(a.timeFor(Kilobytes(x)))
	tb := float64(b.timeFor(Kilobytes(x)))
	if math.Abs(ta-tb) > 1e-6 {
		t.Errorf("at the crossover the links must tie: A=%.6fs B=%.6fs (size=%.1f KB)", ta, tb, x)
	}
	if !(x > 10 && x < 1e6) {
		t.Errorf("crossover %.1f KB should sit between the small (10 KB, A wins) and large (1 GB, B wins) cases", x)
	}
}

// TestWinnerFlipsAcrossCrossover: just below the crossover the near link wins, just
// above it the fat pipe wins. The whole point — slide the size, the winner changes.
func TestWinnerFlipsAcrossCrossover(t *testing.T) {
	a, b := near(), fat()
	x := crossoverKB(a, b)
	below := Kilobytes(x * 0.5)
	above := Kilobytes(x * 2)
	if !(a.timeFor(below) < b.timeFor(below)) {
		t.Errorf("below crossover, near link should win")
	}
	if !(b.timeFor(above) < a.timeFor(above)) {
		t.Errorf("above crossover, fat pipe should win")
	}
}

// TestNoCrossoverWhenOneDominates: if a link is both lower-latency AND higher-bandwidth
// it wins at every size — there is no crossover (crossoverKB returns 0). Guards against
// reporting a spurious crossing.
func TestNoCrossoverWhenOneDominates(t *testing.T) {
	a := linkA(1, 1000) // strictly better: lower latency, higher bandwidth
	b := linkB(300, 100)
	if x := crossoverKB(a, b); x != 0 {
		t.Errorf("a strictly-dominant link should have no crossover, got %.3f KB", x)
	}
}

// TestUnitsDoNotCross is the fleet-style guard: the unit conversions are the only
// bridges between the type universes, and each maps to the physically-correct base
// unit. If these drift, every number is wrong.
func TestUnitsDoNotCross(t *testing.T) {
	if Milliseconds(1000).seconds() != 1.0 {
		t.Error("1000 ms should be 1 s")
	}
	if Megabits(8).bytesPerSec() != 1e6 {
		t.Error("8 Mbit/s should be 1e6 bytes/s")
	}
	if Kilobytes(1).bytes() != 1024 {
		t.Error("1 KB should be 1024 bytes")
	}
}

// TestViewsRender: the chart is SVG with its orienting labels; the verdict readout is
// HTML (else the client hides it — the pid/Readout lesson).
func TestViewsRender(t *testing.T) {
	c := compare(near(), fat(), 2000)

	ch := curveChart(c).Render()
	if !strings.HasPrefix(ch.Data, "<svg") {
		t.Fatal("chart not SVG")
	}
	for _, w := range []string{"Link A", "Link B", "latency-bound", "crossover"} {
		if !strings.Contains(ch.Data, w) {
			t.Errorf("chart missing %q", w)
		}
	}

	vd := verdict(c).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html (else the client hides it)", vd.MIME)
	}
	for _, w := range []string{"transfer size", "winner", "crossover"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
