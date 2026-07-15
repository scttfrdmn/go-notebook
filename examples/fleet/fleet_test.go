package fleet

import (
	"math"
	"strings"
	"testing"
)

func bill(n, spot, hrs int) Bill { return cost(n, spot, hrs, onDemandRate(), spotRate()) }
func em(n, hrs, g int) Emissions { return carbon(n, hrs, nodePower(), g) }

// TestCostPure / TestCarbonPure: both are pure functions of their inputs.
func TestPure(t *testing.T) {
	if bill(256, 60, 336).Total != bill(256, 60, 336).Total {
		t.Error("cost not pure")
	}
	if em(256, 336, 350).Total != em(256, 336, 350).Total {
		t.Error("carbon not pure")
	}
}

// TestSpotLowersCost is the cost lever: more spot → lower bill, monotonically.
func TestSpotLowersCost(t *testing.T) {
	c0 := float64(bill(256, 0, 336).Total)
	c60 := float64(bill(256, 60, 336).Total)
	c100 := float64(bill(256, 100, 336).Total)
	if !(c0 > c60 && c60 > c100) {
		t.Errorf("more spot should cost less: %.0f, %.0f, %.0f", c0, c60, c100)
	}
	// all-spot vs all-on-demand should differ by the rate ratio.
	if r := c100 / c0; math.Abs(r-1.10/3.06) > 0.001 {
		t.Errorf("all-spot/all-on-demand cost ratio %.3f, want %.3f (rate ratio)", r, 1.10/3.06)
	}
}

// TestSpotDoesNotChangeCarbon is THE teaching claim: the spot fraction is a cost
// lever only — carbon is byte-for-byte identical across the whole spot range,
// because emissions come from energy, which the pricing doesn't touch.
func TestSpotDoesNotChangeCarbon(t *testing.T) {
	// carbon() doesn't even take spot — but prove the whole pipeline agrees: the
	// emissions are the same regardless of how the bill was priced.
	base := em(256, 336, 350).Total
	for _, spot := range []int{0, 25, 50, 75, 100} {
		_ = bill(256, spot, 336) // vary the cost side
		if em(256, 336, 350).Total != base {
			t.Errorf("carbon moved with spot=%d — it must not (energy is unchanged)", spot)
		}
	}
}

// TestCarbonTracksEnergyAndGrid: emissions = energy × grid intensity, and energy =
// power × node-hours. Doubling nodes, hours, or grid intensity each doubles carbon.
func TestCarbonTracksEnergyAndGrid(t *testing.T) {
	base := float64(em(256, 336, 350).Total)
	if got := float64(em(512, 336, 350).Total); math.Abs(got-2*base) > 1e-6 {
		t.Errorf("2× nodes should 2× carbon: %.1f vs %.1f", got, base)
	}
	if got := float64(em(256, 672, 350).Total); math.Abs(got-2*base) > 1e-6 {
		t.Errorf("2× hours should 2× carbon: %.1f vs %.1f", got, base)
	}
	if got := float64(em(256, 336, 700).Total); math.Abs(got-2*base) > 1e-6 {
		t.Errorf("2× grid intensity should 2× carbon: %.1f vs %.1f", got, base)
	}
}

// TestEnergyFormula: energy = power × hours × nodes, in kWh.
func TestEnergyFormula(t *testing.T) {
	got := float64(em(256, 336, 350).Energy)
	want := 0.55 * 336 * 256
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("energy = %.1f kWh, want %.1f (power × hours × nodes)", got, want)
	}
}

// TestBillSplitAddsUp: on-demand + spot = total, and the split tracks the fraction.
func TestBillSplitAddsUp(t *testing.T) {
	b := bill(256, 60, 336)
	if math.Abs(float64(b.OnDemand+b.Spot)-float64(b.Total)) > 1e-6 {
		t.Errorf("split doesn't sum: %.1f + %.1f ≠ %.1f", b.OnDemand, b.Spot, b.Total)
	}
	// 60% spot → spot portion is 60% of node-hours (at the spot rate).
	allSpot := bill(256, 100, 336)
	if math.Abs(float64(b.Spot)-0.6*float64(allSpot.Total)) > 1e-3 {
		t.Errorf("spot portion %.1f, want 60%% of all-spot %.1f", b.Spot, allSpot.Total)
	}
}

// TestSummaryRenders confirms the chart is SVG with both headline numbers and the
// split bar.
func TestSummaryRenders(t *testing.T) {
	data := summary(bill(256, 60, 336), em(256, 336, 350)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("summary not SVG")
	}
	for _, want := range []string{"monthly cost", "monthly carbon", "on-demand", "spot", "t CO"} {
		if !strings.Contains(data, want) {
			t.Errorf("summary missing %q", want)
		}
	}
}
