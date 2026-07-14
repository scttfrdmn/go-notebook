package surface

import (
	"math"
	"strings"
	"testing"
)

// TestSurfacePure confirms the heightfield is a pure function of the sliders — same
// inputs, same field — so the cell caches and scrubs exactly (the design property).
func TestSurfacePure(t *testing.T) {
	a := surface(40, 10, 3)
	b := surface(40, 10, 3)
	if len(a.Z) != len(b.Z) {
		t.Fatalf("length differs")
	}
	for i := range a.Z {
		if a.Z[i] != b.Z[i] {
			t.Fatalf("surface not pure at %d: %v vs %v", i, a.Z[i], b.Z[i])
		}
	}
}

// TestSurfaceShape checks the math produces a real surface: res×res samples, a
// non-trivial height range, and zero at the boundary (the falloff). Guards against
// a flat or degenerate field that would render as nothing.
func TestSurfaceShape(t *testing.T) {
	f := surface(40, 10, 3)
	if f.Res != 40 || len(f.Z) != 40*40 {
		t.Fatalf("res/len wrong: res=%d len=%d", f.Res, len(f.Z))
	}
	var lo, hi float64
	for _, z := range f.Z {
		lo, hi = math.Min(lo, z), math.Max(hi, z)
	}
	if hi-lo < 0.2 {
		t.Errorf("surface is nearly flat (range %.3f) — nothing to see", hi-lo)
	}
	// The corners are at radius > 1, so falloff zeroes them.
	if f.Z[0] != 0 {
		t.Errorf("corner should be zero (outside unit disk), got %v", f.Z[0])
	}
}

// TestAmplitudeScales confirms the amplitude slider actually changes the height —
// double the amplitude, roughly double the range.
func TestAmplitudeScales(t *testing.T) {
	rng := func(f Heightfield) float64 {
		var lo, hi float64
		for _, z := range f.Z {
			lo, hi = math.Min(lo, z), math.Max(hi, z)
		}
		return hi - lo
	}
	small := rng(surface(40, 5, 3))
	large := rng(surface(40, 20, 3))
	if large < 3*small {
		t.Errorf("4x amplitude gave range %.3f vs %.3f — slider barely bites", large, small)
	}
}

// TestSinApprox checks the local sin against math.Sin — the notebook uses a Taylor
// series (to keep the cell graph obviously pure), so verify it's actually accurate.
func TestSinApprox(t *testing.T) {
	for x := -pi; x <= pi; x += 0.1 {
		if d := math.Abs(sin(x) - math.Sin(x)); d > 1e-9 {
			t.Errorf("sin(%.2f) off by %g", x, d)
		}
	}
}

// TestSceneHTMLStructure is the escape-hatch wiring test: the rendered HTML must be
// text/html, carry the heightfield in the canvas dataset, and bootstrap WebGL from
// an onerror handler (since innerHTML-injected <script> does not run). If any of
// these regress, the GPU never sees the data.
func TestSceneHTMLStructure(t *testing.T) {
	r := scene(surface(8, 10, 3)).Render()
	if r.MIME != "text/html" {
		t.Fatalf("MIME = %q, want text/html", r.MIME)
	}
	for _, want := range []string{
		`<canvas`,      // the draw target
		`data-res="8"`, // the grid size for the mesh builder
		`data-z=`,      // the heightfield payload
		`onerror=`,     // the bootstrap trigger (script tags don't run via innerHTML)
		`getContext`,   // the WebGL init in the handler
		`drawArrays`,   // the actual draw call
	} {
		if !strings.Contains(r.Data, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

// TestEscapeAttr guards the attribute boundary directly: the data and the handler
// live in HTML attributes, so any quote/angle bracket in them must be escaped or a
// value could break out. This is the one place notebook-produced content meets an
// attribute, so test it in isolation.
func TestEscapeAttr(t *testing.T) {
	got := escapeAttr(`a"b'c<d>e&f`)
	for _, raw := range []string{`"`, `'`, `<`, `>`} {
		if strings.Contains(got, raw) {
			t.Errorf("escapeAttr left a raw %q in %q", raw, got)
		}
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("escapeAttr did not escape &: %q", got)
	}
}
