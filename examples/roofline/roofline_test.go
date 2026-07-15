package roofline

import (
	"math"
	"strings"
	"testing"
)

// TestRooflinePure confirms the model is a pure function of its inputs.
func TestRooflinePure(t *testing.T) {
	a := roofline(1000, 200, 25)
	b := roofline(1000, 200, 25)
	if a.Attainable != b.Attainable {
		t.Fatalf("not pure")
	}
}

// TestRidgeIsPeakOverBandwidth pins the ridge point: intensity where the bandwidth
// slope meets the peak plateau, = peak/bandwidth. 1000/200 = 5.0 FLOP/byte.
func TestRidgeIsPeakOverBandwidth(t *testing.T) {
	if got := roofline(1000, 200, 25).Ridge; math.Abs(got-5.0) > 1e-9 {
		t.Errorf("ridge = %.3f, want 5.0 (peak/bandwidth)", got)
	}
}

// TestAttainableIsMinOfCeilings is the model: attainable = min(peak, intensity×bw).
// Left of the ridge → bandwidth slope; right → the flat peak.
func TestAttainableIsMinOfCeilings(t *testing.T) {
	cases := []struct {
		ai   int
		want float64
	}{
		{25, 50},     // 0.25 FLOP/byte × 200 = 50 (memory-bound)
		{100, 200},   // 1.0 × 200 = 200 (memory-bound)
		{500, 1000},  // 5.0 = ridge → hits peak
		{2000, 1000}, // 20 → clamped at peak (compute-bound)
	}
	for _, c := range cases {
		if got := roofline(1000, 200, c.ai).Attainable; math.Abs(got-c.want) > 1e-6 {
			t.Errorf("ai=%d: attainable %.1f, want %.1f", c.ai, got, c.want)
		}
	}
}

// TestBoundFlipsAtRidge is the teaching claim: below the ridge the kernel is
// memory-bound, above it compute-bound, and the flip is exactly at peak/bandwidth.
func TestBoundFlipsAtRidge(t *testing.T) {
	// ridge = 5.0 → ai 490 (4.9) memory, 510 (5.1) compute.
	if !roofline(1000, 200, 490).MemoryBound {
		t.Error("intensity 4.9 (< ridge 5.0) should be memory-bound")
	}
	if roofline(1000, 200, 510).MemoryBound {
		t.Error("intensity 5.1 (> ridge 5.0) should be compute-bound")
	}
}

// TestFasterCoresDontHelpMemoryBound is the core lesson, quantified: for a
// memory-bound kernel, doubling peak compute leaves attainable performance
// UNCHANGED — the cores were already waiting on memory.
func TestFasterCoresDontHelpMemoryBound(t *testing.T) {
	// intensity 0.25, bw 200 → memory-bound regardless of peak.
	base := roofline(1000, 200, 25)
	fasterCores := roofline(2000, 200, 25) // 2× peak
	if !base.MemoryBound {
		t.Fatal("precondition: should be memory-bound")
	}
	if math.Abs(base.Attainable-fasterCores.Attainable) > 1e-9 {
		t.Errorf("doubling peak changed a memory-bound kernel's performance: %.1f → %.1f — it shouldn't", base.Attainable, fasterCores.Attainable)
	}
	// but raising INTENSITY does help (up the slope).
	higherAI := roofline(1000, 200, 100) // 0.25 → 1.0 FLOP/byte
	if higherAI.Attainable <= base.Attainable {
		t.Errorf("raising intensity should raise attainable: %.1f → %.1f", base.Attainable, higherAI.Attainable)
	}
}

// TestFasterCoresHelpComputeBound: the mirror — for a compute-bound kernel, more
// peak DOES raise attainable performance.
func TestFasterCoresHelpComputeBound(t *testing.T) {
	base := roofline(1000, 200, 2000) // intensity 20 > ridge → compute-bound
	faster := roofline(2000, 200, 2000)
	if base.MemoryBound {
		t.Fatal("precondition: should be compute-bound")
	}
	if !(faster.Attainable > base.Attainable) {
		t.Errorf("faster cores should help a compute-bound kernel: %.1f → %.1f", base.Attainable, faster.Attainable)
	}
}

// TestNeverExceedsRoof: attainable is always ≤ both ceilings.
func TestNeverExceedsRoof(t *testing.T) {
	for _, ai := range []int{5, 50, 500, 5000} {
		r := roofline(1000, 200, ai)
		if r.Attainable > r.Peak+1e-9 || r.Attainable > r.Intensity*r.Bandwidth+1e-9 {
			t.Errorf("ai=%d: attainable %.1f exceeds a ceiling (peak %.0f, bw-ceil %.1f)", ai, r.Attainable, r.Peak, r.Intensity*r.Bandwidth)
		}
	}
}

// TestPlotRenders confirms the chart is SVG with the roofline, ridge, and verdict-
// colored kernel dot.
func TestPlotRenders(t *testing.T) {
	data := plot(roofline(1000, 200, 25)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("plot not SVG")
	}
	for _, want := range []string{"ridge 5.0", "memory-bound", "compute-bound", "50 GFLOP/s"} {
		if !strings.Contains(data, want) {
			t.Errorf("plot missing %q", want)
		}
	}
}
