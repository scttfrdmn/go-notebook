package reliability

import (
	"math"
	"strings"
	"testing"
)

// TestSystemPure confirms the composition is a pure function of the four sliders.
func TestSystemPure(t *testing.T) {
	a := system(35, 25, 35, 1)
	b := system(35, 25, 35, 1)
	if a.Total != b.Total {
		t.Fatalf("system not pure: %v vs %v", a.Total, b.Total)
	}
}

// TestNinesRoundTrip pins the unit: n nines ↔ a = 1 − 10^(−n). 30 (×10) → 3.0 nines
// → 0.999, and ninesOf inverts it. The whole readout hangs on this being right.
func TestNinesRoundTrip(t *testing.T) {
	cases := []struct {
		tenths int
		want   float64
	}{
		{10, 0.9}, {20, 0.99}, {30, 0.999}, {40, 0.9999},
	}
	for _, c := range cases {
		if got := fromNines(c.tenths); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("fromNines(%d) = %.10f, want %.10f", c.tenths, got, c.want)
		}
	}
}

// TestSeriesIsWorseThanWorstPart is the first teaching claim: availabilities in
// series multiply, so the system is strictly worse than its weakest stage. Three
// equal three-nines stages must compose below three nines.
func TestSeriesIsWorseThanWorstPart(t *testing.T) {
	sys := system(30, 30, 30, 1) // all 3.0 nines, no redundancy
	worst := math.Min(sys.LB.A, math.Min(sys.App.A, sys.DB.A))
	if sys.Total >= worst {
		t.Errorf("series total %.6f is not worse than the worst part %.6f — the multiply is broken", sys.Total, worst)
	}
	// 0.999³ ≈ 0.997, i.e. below three nines.
	if n := -math.Log10(1 - sys.Total); n >= 3.0 {
		t.Errorf("three three-nines stages composed to %.2f nines; series must drop below 3", n)
	}
}

// TestRedundancyRaisesToThePower is the second teaching claim: N parallel replicas
// raise the tier's UNavailability to the Nth power. Two three-nines replicas → a
// six-nines tier.
func TestRedundancyRaisesToThePower(t *testing.T) {
	one := system(50, 30, 50, 1).App.A // single app replica at 3.0 nines
	two := system(50, 30, 50, 2).App.A // two in parallel
	unavailOne := 1 - one
	unavailTwo := 1 - two
	// unavailability should square: (1e-3)² = 1e-6.
	if math.Abs(unavailTwo-unavailOne*unavailOne) > 1e-12 {
		t.Errorf("2-way redundancy did not square the unavailability: %.3e vs %.3e²", unavailTwo, unavailOne)
	}
	// which is a jump of ~3 nines on the tier.
	jump := (-math.Log10(unavailTwo)) - (-math.Log10(unavailOne))
	if jump < 2.5 {
		t.Errorf("2 replicas of a 3-nines server jumped only %.1f nines, expected ~3", jump)
	}
}

// TestRedundancyHelpsUntilBottleneckMoves: adding replicas raises the system until
// the app tier is no longer the floor — then it stops mattering. Guards the honest
// nuance the demo shows (3→4 replicas barely moves a system floored by the DB).
func TestRedundancyDiminishes(t *testing.T) {
	// app the weak link (2.0 nines), LB/DB strong (4.0). More replicas help, then plateau.
	sys1 := system(40, 20, 40, 1).Total
	sys2 := system(40, 20, 40, 2).Total
	sys5 := system(40, 20, 40, 5).Total
	if !(sys2 > sys1) {
		t.Errorf("a 2nd replica did not help: %.6f → %.6f", sys1, sys2)
	}
	// by 5 replicas the app tier is far past the LB/DB floor, so system ≈ LB×DB.
	floor := fromNines(40) * fromNines(40)
	if math.Abs(sys5-floor) > 1e-6 {
		t.Errorf("with 5 replicas the system %.6f should approach the LB×DB floor %.6f", sys5, floor)
	}
}

// TestDowntimeUnits sanity-checks the human-facing downtime: three nines ≈ 8.77 h/yr.
func TestDowntimeUnits(t *testing.T) {
	got := downtime(0.999)
	if !strings.Contains(got, "hours") {
		t.Errorf("three nines downtime = %q, expected hours", got)
	}
}

// TestDiagramRendersTopology: the block diagram renders SVG, shows all three stages,
// and draws one app-server box per replica (the parallel stack).
func TestDiagramRendersTopology(t *testing.T) {
	for _, n := range []int{1, 3} {
		data := diagram(system(35, 25, 35, n)).Render().Data
		if !strings.HasPrefix(data, "<svg") {
			t.Fatalf("n=%d: not SVG", n)
		}
		for _, want := range []string{"load balancer", "database", "app server", "system:"} {
			if !strings.Contains(data, want) {
				t.Errorf("n=%d: diagram missing %q", n, want)
			}
		}
		if got := strings.Count(data, ">app server<"); got != n {
			t.Errorf("n=%d: drew %d app-server boxes, want %d (one per replica)", n, got, n)
		}
	}
}
