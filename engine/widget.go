package engine

import (
	"encoding/json"
	"reflect"
)

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

// StampLeaf stamps a draggable widget with the leaf symbol it belongs to, if the
// value exposes the runtime seam WithLeaf(string) → same-type. It is the
// write-direction twin of the read probes (Render/Bounds/WidgetView): the
// notebook exposes a method of an agreed shape, the runtime calls it, and the
// runtime never names the widget's type. A grip is drawn by a cell that does not
// own its leaf (curvefit's editor draws handles for the ctrl leaf), so the
// leaf's identity must ride WITH the value across that cell boundary — this is
// how it gets there. WithLeaf has value semantics (returns a copy), so the
// stamped value flows downstream as an ordinary value with no hidden mutation.
//
// If v has no WithLeaf seam (every non-draggable widget), v is returned
// unchanged. The seam is for the RUNTIME only; a notebook that calls it is a
// smell (the runtime writes leaf identity, the notebook reads it via Grip).
func StampLeaf(v any, sym string) any {
	m := reflect.ValueOf(v).MethodByName("WithLeaf")
	if !m.IsValid() {
		return v
	}
	mt := m.Type()
	if mt.NumIn() != 1 || mt.NumOut() != 1 ||
		mt.In(0).Kind() != reflect.String || mt.Out(0) != reflect.TypeOf(v) {
		return v // wrong shape — not the stamping seam
	}
	return m.Call([]reflect.Value{reflect.ValueOf(sym)})[0].Interface()
}

// CoerceWire homogenizes a decoded-JSON selection into the clean Go primitive a
// widget's Reconcile expects, so a cell stays an ordinary Go function that never
// touches wire shapes. It is the general form of the scalar coercer — the write
// path is a human at human speed, so a little reflection here is free.
//
// A selection's Go type is not the widget's field type: a Multi[Theme]'s
// selection is []string (labels — the client picks by Label(), and the
// label→Theme mapping lives in the notebook's Reconcile, not here), a Range's is
// []float64 (endpoints), a Select's is a string. JSON decodes those as []any and
// json.Number; this collapses them to []string / []float64 / string / bool so a
// Reconcile(saved any) can assert a concrete type. It never guesses a domain
// type; it only removes the wire's any-boxing.
//
// ok is false when the value can't be homogenized to a single primitive shape
// (a mixed []any) — a real client/leaf mismatch the caller must surface, never
// pass through silently.
func CoerceWire(decoded any) (any, bool) {
	switch v := decoded.(type) {
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
		return nil, false
	case []any:
		return coerceSlice(v)
	default:
		return decoded, true // string, bool, float64 — already primitive
	}
}

// coerceSlice collapses a decoded []any into []string or []float64 when every
// element shares a shape, else reports failure (a mixed array is a mismatch).
// An empty selection is a valid []string (the common "nothing selected" case).
func coerceSlice(in []any) (any, bool) {
	if len(in) == 0 {
		return []string{}, true
	}
	strs := make([]string, 0, len(in))
	floats := make([]float64, 0, len(in))
	for _, e := range in {
		switch x := e.(type) {
		case string:
			strs = append(strs, x)
		case json.Number:
			if f, err := x.Float64(); err == nil {
				floats = append(floats, f)
			}
		case float64:
			floats = append(floats, x)
		}
	}
	if len(strs) == len(in) {
		return strs, true
	}
	if len(floats) == len(in) {
		return floats, true
	}
	return nil, false // mixed or unrecognized element types
}

// WidgetView is a widget's STATE on the wire — never its appearance. It carries
// what the client needs to render an interactive control and to know what a
// user's edit means: the current selection, the available choices/bounds, and
// hard constraints. It carries nothing about how the control LOOKS — no label,
// color, step, or layout. Kind (static, from the type, in CellMeta) decides
// which control; a //notebook: directive refines it; this view carries neither.
//
// This is the input analogue of Rendered (which is output, a picture). A widget
// is structured input state, so a Multi/Select/Range/Table value that is not
// Renderable still reaches the client — through this, not as a blob.
//
// Each widget KIND builds its own view explicitly (see the notebook's widget
// types). It is never a generic reflection of the widget struct: that would drag
// a Draggable's unexported leaf token or a Table's arbitrary row type onto the
// wire. Verbose-but-explicit is the point — the wire format is a decision.
type WidgetView struct {
	// Value is the current selection. Its permitted shapes are a CLOSED set —
	// adding one is a decision, not a fill-in, because this is the one field the
	// type does not constrain:
	//   - a JSON scalar (number/string/bool) — Range picks a number, Select a label
	//   - a []string of labels               — Multi's selected options
	//   - a []T of rows                       — Table's editable rows
	//   - a []Pt (or similar point list)      — Draggable's handle positions
	// It must stay flat, JSON-encodable STATE. A nested object describing
	// appearance or structure does not belong here; if a new widget needs a
	// shape not listed above, add it here deliberately and update this comment.
	Value any `json:"value"`
	// Options are the choosable labels for Select/Multi (nil otherwise).
	Options []string `json:"options,omitempty"`
	// Lo/Hi are the numeric bounds for Range. Pointers so "no bounds" (nil) is
	// distinct from a real [0,0] range — absent means absent, no separate flag.
	Lo *float64 `json:"lo,omitempty"`
	Hi *float64 `json:"hi,omitempty"`
	// Max is a selection-count cap for Multi. Pointer so "no cap" (nil) is
	// distinct from a cap of 0.
	Max *int `json:"max,omitempty"`
}

// Viewable is the capability a widget value has when it can state its own view.
// Probed structurally (like Renderable), because a notebook defines its OWN
// widget types and imports nothing from this package — so the match is by method
// shape across the zero-import boundary, not by a static interface.
//
// The method is WidgetView() WidgetView-shaped: no args, one struct result with
// the field shape above. A widget states its view explicitly in this method.
type Viewable interface {
	WidgetView() WidgetView
}
