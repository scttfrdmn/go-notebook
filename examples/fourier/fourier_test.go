package fourier

import (
	"math"
	"strings"
	"testing"
)

func sh() Draggable[Pt] { return shape() }

// TestSpectrumPure confirms the DFT is a pure function of the shape.
func TestSpectrumPure(t *testing.T) {
	a := spectrum(sh())
	b := spectrum(sh())
	if len(a.Coeffs) != len(b.Coeffs) {
		t.Fatalf("length differs")
	}
	for i := range a.Coeffs {
		if a.Coeffs[i] != b.Coeffs[i] {
			t.Fatalf("spectrum not pure at %d", i)
		}
	}
}

// TestReconstructionConverges is the teaching claim: adding terms captures more of
// the shape's energy, monotonically toward 100%. If it didn't climb, the circles
// wouldn't be tracing the curve.
func TestReconstructionConverges(t *testing.T) {
	spec := spectrum(sh())
	energy := func(n int) float64 {
		kept := lowestFreqs(len(spec.Coeffs), n)
		var keptE, totalE float64
		for _, c := range spec.Coeffs {
			totalE += cabs(c) * cabs(c)
		}
		for _, f := range kept {
			e := cabs(spec.Coeffs[wrap(f, len(spec.Coeffs))])
			keptE += e * e
		}
		return keptE / totalE
	}
	e1, e8, e40 := energy(1), energy(8), energy(40)
	if !(e1 < e8 && e8 <= e40) {
		t.Errorf("energy did not climb with terms: %.3f, %.3f, %.3f", e1, e8, e40)
	}
	if e40 < 0.999 {
		t.Errorf("40 circles capture only %.1f%% of the energy, want ~100%%", e40*100)
	}
}

// TestReconstructionReversible is the purity payoff: the reconstruction at n terms is
// identical whether you arrived by adding or removing circles — there's no state, so
// Gibbs ringing is something you can summon AND dismiss.
func TestReconstructionReversible(t *testing.T) {
	spec := spectrum(sh())
	direct := reconstruction(spec, 8)
	_ = reconstruction(spec, 40) // "scrub up" first
	back := reconstruction(spec, 8)
	if len(direct.Curve) != len(back.Curve) {
		t.Fatalf("curve length differs")
	}
	for i := range direct.Curve {
		if direct.Curve[i] != back.Curve[i] {
			t.Fatalf("reconstruction at n=8 differs by path at point %d — not reversible", i)
		}
	}
}

// TestReconstructionTracesShape: with enough terms, the reconstruction passes close
// to every vertex of the target star (the circles really do trace the curve).
func TestReconstructionTracesShape(t *testing.T) {
	outline := sh()
	spec := spectrum(outline)
	recon := reconstruction(spec, 60)
	for vi, v := range outline.Value {
		best := math.Inf(1)
		for _, p := range recon.Curve {
			best = math.Min(best, math.Hypot(p.X-v.X, p.Y-v.Y))
		}
		if best > 3.0 { // within 3 units on a 100-unit plane
			t.Errorf("vertex %d (%.1f,%.1f) not traced: nearest recon point %.2f away", vi, v.X, v.Y, best)
		}
	}
}

// TestStarSpectralSymmetry: a 5-pointed star's spectrum is dominated by frequencies
// ≡ ±1 (mod 5) — the fundamental at ±1 and its harmonics. Bins that break the 5-fold
// symmetry should be near zero. This confirms the DFT captured real structure.
func TestStarSpectralSymmetry(t *testing.T) {
	spec := spectrum(sh())
	n := len(spec.Coeffs)
	fund := cabs(spec.Coeffs[wrap(1, n)])   // the ±1 fundamental — should be large
	broken := cabs(spec.Coeffs[wrap(2, n)]) // freq 2 breaks 5-fold symmetry — tiny
	if fund < 10*broken {
		t.Errorf("star fundamental (|c1|=%.3f) not dominant over asymmetric freq (|c2|=%.3f)", fund, broken)
	}
}

// TestViewRendersWithGrips: the chart renders SVG with the target, reconstruction,
// and one grip per polygon vertex.
func TestViewRendersWithGrips(t *testing.T) {
	outline := shape().WithLeaf("s")
	spec := spectrum(outline)
	data := view(outline, reconstruction(spec, 6)).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("view is not SVG")
	}
	if got := strings.Count(data, "data-grip"); got != len(outline.Value) {
		t.Errorf("%d grips, want %d (one per vertex)", got, len(outline.Value))
	}
}

// TestResampleClosed: resampling produces the requested count and stays within the
// polygon's bounding box (no wild interpolation).
func TestResampleClosed(t *testing.T) {
	pts := resample(star(5, 50, 50, 38, 16), samples)
	if len(pts) != samples {
		t.Fatalf("resample gave %d points, want %d", len(pts), samples)
	}
	for _, z := range pts {
		if real(z) < 10 || real(z) > 90 || imag(z) < 10 || imag(z) > 90 {
			t.Fatalf("resampled point %v outside the star's bounds", z)
		}
	}
}
