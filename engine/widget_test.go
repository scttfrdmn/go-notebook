package engine

import "testing"

// unit is a domain type that satisfies Bounded structurally — no import of this
// package is needed for a value to be a slider.
type unit float64

func (unit) Bounds() (float64, float64) { return 0, 1 }

// rng is a Reconciler that clamps a saved value into its bounds.
type rng struct{ lo, hi float64 }

func (r rng) Reconcile(saved any) any {
	v, ok := saved.(float64)
	if !ok {
		return saved
	}
	if v < r.lo {
		return r.lo
	}
	if v > r.hi {
		return r.hi
	}
	return v
}

// markdown satisfies Renderable structurally.
type markdown string

func (m markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }

// TestCapabilityProbing confirms discovery is by structural probe, not a type
// switch: any value with the method shape is discovered, with no registration.
func TestBoundedProbe(t *testing.T) {
	if b, ok := AsBounded(unit(0.5)); !ok {
		t.Error("unit should be Bounded")
	} else if lo, hi := b.Bounds(); lo != 0 || hi != 1 {
		t.Errorf("bounds = %v,%v want 0,1", lo, hi)
	}
	if _, ok := AsBounded(42); ok {
		t.Error("a plain int should not be Bounded")
	}
}

func TestReconcilerClamps(t *testing.T) {
	r, ok := AsReconciler(rng{lo: 0, hi: 150})
	if !ok {
		t.Fatal("rng should be a Reconciler")
	}
	if got := r.Reconcile(200.0); got != 150.0 {
		t.Errorf("clamp 200 into [0,150] = %v, want 150", got)
	}
	if got := r.Reconcile(75.0); got != 75.0 {
		t.Errorf("75 within [0,150] should be unchanged, got %v", got)
	}
}

func TestRenderableProbe(t *testing.T) {
	if r, ok := AsRendered(markdown("# hi")); !ok {
		t.Error("markdown should be Renderable")
	} else if r.MIME != "text/markdown" || r.Data != "# hi" {
		t.Errorf("rendered = %+v", r)
	}
	if _, ok := AsRendered(42); ok {
		t.Error("a plain int should not be Renderable")
	}
}
