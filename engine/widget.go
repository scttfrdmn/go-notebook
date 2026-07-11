package engine

// Widget discovery is capability probing, never a type switch. Adding a new
// control kind later (Multi, Select, Table, Draggable) means adding one probe
// here, not editing a switch statement in several places. The registry grows
// with the small, nearly-closed set of presentation categories, not with the
// user's domain types.

// Bounded is the input capability this milestone supports: a value that
// declares a numeric range renders as a ranged control (a slider). A type
// satisfies it structurally — no import of this package is required for a
// domain type to be a slider.
type Bounded interface {
	Bounds() (lo, hi float64)
}

// Optioned is declared but unused this milestone. It exists so the set of
// capability probes is a list that grows by one entry per control kind, rather
// than a special case bolted on later.
type Optioned interface {
	Options() []string
}

// Reconciler merges a saved selection into a freshly computed schema. When a
// cell recomputes a widget's bounds/options, the head still holds the user's
// selection; reconciliation is per-widget-kind, not universal:
//
//   - a range clamps its saved selection into the new bounds,
//   - a multi-select filters out options that no longer exist,
//   - a draggable resets on an arity change.
//
// Range is the only implementation this milestone; the interface exists so the
// others are additive.
type Reconciler interface {
	Reconcile(saved any) any
}

// AsBounded probes v for the Bounded capability. A cell whose output is Bounded
// is an input leaf (a control), regardless of whether it has parameters — which
// is what makes data-derived bounds (a cell computing its own range from data)
// additive rather than a redesign.
func AsBounded(v any) (Bounded, bool) {
	b, ok := v.(Bounded)
	return b, ok
}

// AsReconciler probes v for the Reconciler capability.
func AsReconciler(v any) (Reconciler, bool) {
	r, ok := v.(Reconciler)
	return r, ok
}
