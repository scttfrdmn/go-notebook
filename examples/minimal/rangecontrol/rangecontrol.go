//go:notebook
//
// rangecontrol — a from/to span (a two-handled slider).
//
// A type with a Bounds() (lo, hi float64) method renders as a range control on
// its own — no directive. From and To are the current selection; Reconcile clamps
// the saved selection into the current bounds across a recompute.
//
//	go tool notebook run ./examples/minimal/rangecontrol
//
// Demonstrates: Bounds() -> range, Reconcile clamps. See docs/reference-controls.html.

package rangecontrol

// The price window to include, in dollars.
func window() (span Range) { return Range{From: 20, To: 80, Lo: 0, Hi: 100} }

// The width of the selected window.
func width(span Range) (w float64) { return span.To - span.From }

// Range is a from/to span: Bounds() makes it a range control; From/To are the
// selection; Reconcile keeps it (clamped) across a wave.
type Range struct {
	From, To, Lo, Hi float64
}

func (r Range) Bounds() (lo, hi float64) { return r.Lo, r.Hi }

func (r Range) Reconcile(saved any) any {
	sel, ok := saved.([]float64)
	if !ok || len(sel) != 2 {
		return r
	}
	from, to := clamp(sel[0], r.Lo, r.Hi), clamp(sel[1], r.Lo, r.Hi)
	if from > to {
		from, to = to, from
	}
	return Range{From: from, To: to, Lo: r.Lo, Hi: r.Hi}
}

// clamp is an ordinary helper (unnamed return) — invisible to the graph.
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
