package amdahl

import (
	"math"
	"strings"
	"testing"
)

// TestModelPure confirms the model is a pure function of (p, n).
func TestModelPure(t *testing.T) {
	a := model(95, 32)
	b := model(95, 32)
	if a.SpeedupAtN != b.SpeedupAtN || a.Ceiling != b.Ceiling {
		t.Fatalf("model not pure")
	}
}

// TestCeilingIsInverseSerial is the headline: the ceiling is 1/(1−p), independent of
// core count. 95% parallel → 20×, 99% → 100×.
func TestCeilingIsInverseSerial(t *testing.T) {
	cases := []struct {
		p    int
		want float64
	}{
		{90, 10}, {95, 20}, {99, 100}, {50, 2},
	}
	for _, c := range cases {
		if got := model(c.p, 8).Ceiling; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("p=%d%%: ceiling %.2f×, want %.0f×", c.p, got, c.want)
		}
	}
}

// TestSpeedupNeverExceedsCeiling is Amdahl's whole point: no matter how many cores,
// the speedup stays under 1/(1−p). Check across the full curve.
func TestSpeedupNeverExceedsCeiling(t *testing.T) {
	m := model(95, 1)
	for i, s := range m.Amdahl {
		if s > m.Ceiling+1e-9 {
			t.Errorf("Amdahl speedup %.3f at %d cores exceeds the ceiling %.3f", s, i+1, m.Ceiling)
		}
	}
	// and it approaches the ceiling from below as cores grow (monotone increasing).
	for i := 1; i < len(m.Amdahl); i++ {
		if m.Amdahl[i] < m.Amdahl[i-1]-1e-12 {
			t.Errorf("Amdahl curve decreased at %d cores — must be monotone up toward the ceiling", i+1)
		}
	}
}

// TestFormula checks speedup(n) = 1/((1−p)+p/n) against a hand computation at the
// default: 95% parallel, 32 cores → 1/(0.05 + 0.95/32) = 12.5×.
func TestFormula(t *testing.T) {
	got := model(95, 32).SpeedupAtN
	want := 1 / (0.05 + 0.95/32)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("speedup at 95%%/32 = %.4f, want %.4f", got, want)
	}
	if math.Abs(got-12.5) > 0.2 {
		t.Errorf("speedup at 95%%/32 = %.2f, expected ≈12.5", got)
	}
}

// TestDiminishingReturns is the flattening: doubling cores past the knee buys less
// and less. From 32→64 cores at 95%, the extra speedup is far under 2×.
func TestDiminishingReturns(t *testing.T) {
	at32 := model(95, 32).SpeedupAtN
	at64 := model(95, 64).SpeedupAtN
	gain := at64 - at32
	if gain <= 0 {
		t.Fatalf("more cores should still help a little (%.2f → %.2f)", at32, at64)
	}
	// doubling cores should NOT double speedup near the ceiling.
	if at64 > 1.5*at32 {
		t.Errorf("doubling cores 32→64 gave %.2f→%.2f (>1.5×) — expected diminishing returns near the ceiling", at32, at64)
	}
}

// TestGustafsonIsLinear: Gustafson's speedup grows linearly with cores (it has no
// ceiling), so at high core counts it far exceeds Amdahl.
func TestGustafsonIsLinear(t *testing.T) {
	m := model(95, 1)
	// linear: value at 2n ≈ 2× value at n (minus the constant serial part).
	last := m.Gustafson[maxCores-1]
	if last <= m.Ceiling {
		t.Errorf("Gustafson at %d cores (%.1f) should exceed Amdahl's ceiling (%.1f) — it's unbounded", maxCores, last, m.Ceiling)
	}
	// and it exceeds Amdahl everywhere past the first core.
	if m.Gustafson[maxCores-1] <= m.Amdahl[maxCores-1] {
		t.Error("Gustafson should exceed Amdahl at high core counts")
	}
}

// TestEfficiencyCollapses: efficiency (speedup/cores) falls as cores grow — the
// "cores wasted" story.
func TestEfficiencyCollapses(t *testing.T) {
	e8 := model(95, 8).EfficiencyAtN
	e128 := model(95, 128).EfficiencyAtN
	if !(e8 > e128) {
		t.Errorf("efficiency should collapse with more cores: %.2f (8) vs %.2f (128)", e8, e128)
	}
}

// TestReadoutRender confirms the number panel actually renders. Without a Render
// method the engine has a value it can't display and the whole cell shows nothing
// in the browser — check green and headless --json both pass, but half the notebook
// is blank. This asserts the four cards and their values reach the output.
func TestReadoutRender(t *testing.T) {
	data := readout(model(95, 32)).Render().Data
	for _, want := range []string{"ceiling (n → ∞)", "20.0×", "speedup at 32 cores", "12.5×", "efficiency", "cores wasted"} {
		if !strings.Contains(data, want) {
			t.Errorf("readout missing %q — the number panel would render blank", want)
		}
	}
}

// TestCurvesRender confirms the chart is SVG with both curves, the ceiling, and the
// marker.
func TestCurvesRender(t *testing.T) {
	data := curves(model(95, 32)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("curves not SVG")
	}
	for _, want := range []string{"ceiling 20.0×", "Amdahl", "Gustafson", "at 32 cores"} {
		if !strings.Contains(data, want) {
			t.Errorf("chart missing %q", want)
		}
	}
}
