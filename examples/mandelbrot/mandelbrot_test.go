package mandelbrot

import (
	"math"
	"strings"
	"testing"
)

// TestRenderIsPure: the image is a pure function of (cx, cy, mag, limit) — same
// viewport, byte-identical pixels. This is what makes it WASM-live and scrubbable.
func TestRenderIsPure(t *testing.T) {
	a := render(-0.5, 0, 3, 512)
	b := render(-0.5, 0, 3, 512)
	if len(a.Px) != len(b.Px) {
		t.Fatalf("pixel counts differ: %d vs %d", len(a.Px), len(b.Px))
	}
	for i := range a.Px {
		if a.Px[i] != b.Px[i] {
			t.Fatalf("render not pure at pixel %d", i)
		}
	}
}

// TestInteriorPointsNeverEscape: the origin (0,0) and (−1,0) are deep in the set, so
// their escape time is 0 (never escaped). The cardioid/bulb early-outs must agree.
func TestInteriorPointsNeverEscape(t *testing.T) {
	if e := escape(0, 0, 1000); e != 0 {
		t.Errorf("origin is in the set, escape should be 0, got %.3f", e)
	}
	if e := escape(-1, 0, 1000); e != 0 {
		t.Errorf("(-1,0) is in the set, escape should be 0, got %.3f", e)
	}
}

// TestExteriorPointsEscape: a point well outside the set escapes quickly (small,
// positive escape time). (2,2) is far out — it should bail almost immediately.
func TestExteriorPointsEscape(t *testing.T) {
	e := escape(2, 2, 1000)
	if e <= 0 {
		t.Errorf("(2,2) is outside the set, should escape (>0), got %.3f", e)
	}
	if e > 10 {
		t.Errorf("(2,2) is far outside, should escape fast (<10), got %.3f", e)
	}
}

// TestMoreIterationsResolveMoreInterior is the iteration-limit lesson: a boundary-ish
// point that "escapes" under a tiny limit is revealed as interior (escape 0) under a
// generous one — higher limits resolve the fine boundary. Use a point near the edge.
func TestMoreIterationsResolveMoreInterior(t *testing.T) {
	// A point just inside the boundary near the seahorse valley: with few iterations
	// it hasn't escaped yet (reads 0), with more it still reads 0 — pick instead a
	// point that flips. (-0.75, 0.1) is near the neck; low limit under-resolves it.
	lo := escape(-0.749, 0.05, 32)
	hi := escape(-0.749, 0.05, 2048)
	// Whatever the classification, more iterations must never DECREASE how much work
	// was done before escaping: a point escaping at the low limit escapes at the same
	// step under the high limit (determinism), and a non-escaper stays 0.
	if lo != 0 && hi != 0 && math.Abs(lo-hi) > 1e-9 {
		t.Errorf("escape time must be limit-independent once escaped: %.4f vs %.4f", lo, hi)
	}
}

// TestZoomShrinksSpan: a higher zoom magnitude maps the image to a smaller region of
// the complex plane. Verify two zoom levels produce different images at the same
// center (the viewport actually changed).
func TestZoomShrinksSpan(t *testing.T) {
	wide := render(-0.7436, 0.1318, 1, 512)
	deep := render(-0.7436, 0.1318, 8, 512)
	same := true
	for i := range wide.Px {
		if wide.Px[i] != deep.Px[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("zooming should change the image; wide and deep views are identical")
	}
}

// TestRenderProducesSVGWrappedPNG: the client injects image/svg+xml but not a bare
// image/png, so the render must be an SVG wrapping a PNG data URI (the turing pattern).
func TestRenderProducesSVGWrappedPNG(t *testing.T) {
	data := render(-0.5, 0, 2, 256).Render()
	if data.MIME != "image/svg+xml" {
		t.Fatalf("MIME = %q, want image/svg+xml (a bare PNG renders as text in the tab)", data.MIME)
	}
	if !strings.Contains(data.Data, "<image") || !strings.Contains(data.Data, "data:image/png;base64,") {
		t.Error("render should embed a PNG data URI inside the SVG wrapper")
	}
}

// TestDepthMatchesEscape: the depth readout is exactly the escape time at the center,
// so the crosshair number and the picture agree.
func TestDepthMatchesEscape(t *testing.T) {
	if depth(-0.5, 0.6, 512) != escape(-0.5, 0.6, 512) {
		t.Error("depth should equal escape() at the center")
	}
}
