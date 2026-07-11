package analyze

import "go/types"

// isLeafCell decides whether a cell is an editable input leaf, from its TYPE —
// never from a directive. A comment must not decide whether something is an
// input (that was the //notebook:nocache mistake, and it must not recur for the
// most important structural property in the design).
//
// The rule (spec §4.3, "a cell is a leaf iff its output implements a widget
// capability", generalized to cover the plainest degradation-ladder rung):
//
//   - parameterless + output widget-capable  → leaf (e.g. slaTarget Probability
//     has Bounds())
//   - parameterless + output a scalar basic   → leaf (bool checkbox, int64/float
//     text field, string) — the bare `func alpha() float64` rung
//   - has parameters + output widget-capable   → leaf (data-derived schema, e.g.
//     priceRange(rows) Range[USD] computes its bounds from data)
//   - otherwise                                → not a leaf (a computed root like
//     sets() []Set is derived, not editable)
//
// A directive (//notebook:slider) only refines how the control renders; delete
// every directive and every control is still present, just plainer. That is the
// degradation ladder, and it depends on this being type-driven.
func isLeafCell(sig *types.Signature) bool {
	// A leaf produces exactly one data value.
	results := sig.Results()
	var only types.Type
	count := 0
	for i := 0; i < results.Len(); i++ {
		t := results.At(i).Type()
		if isErrorType(t) {
			continue
		}
		count++
		only = t
	}
	if count != 1 {
		return false
	}

	// A renderable output is a VIEW, never an input — even though its type may
	// be a scalar (e.g. notes() Markdown, where Markdown is a string with a
	// Render() method). The Render() method is the tell: the cell exists to be
	// displayed, not edited.
	if hasMethod(only, "Render") {
		return false
	}

	if sig.Params().Len() == 0 {
		// A root control: an input-widget capability (Bounds/Options for a
		// ranged/select control), or a plain scalar the view renders as a text
		// field / checkbox — the plainest degradation rung.
		return isInputWidget(only) || isScalarBasic(only)
	}
	// A cell WITH parameters is a leaf only in the data-derived-schema case: its
	// output is a stateful selection widget (a Reconciler — Range/Multi/
	// Draggable) whose bounds/options track the data while the head holds the
	// user's selection. A parameterized value that merely carries Bounds()
	// (e.g. waitProbability(a,c) Probability) is a COMPUTED result, not an
	// input — so Reconcile, not Bounds, is the gate here.
	return hasMethod(only, "Reconcile")
}

// isInputWidget reports whether a type is an input control by capability:
// Bounds() (ranged) or Options() (select). Reconcile alone does not make a
// parameterless cell an input (it is about merging a saved selection), but a
// parameterless Reconciler is unusual; Bounds/Options are the root-control
// capabilities.
func isInputWidget(t types.Type) bool {
	return hasMethod(t, "Bounds") || hasMethod(t, "Options")
}

// hasMethod reports whether t (or *t) has a method of the given name.
func hasMethod(t types.Type, name string) bool {
	for _, cand := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(cand)
		for i := 0; i < ms.Len(); i++ {
			if ms.At(i).Obj().Name() == name {
				return true
			}
		}
	}
	return false
}

// basicKind returns the name of t's underlying basic kind ("int", "float64",
// "bool", "string", …) for a basic-kinded type (including through a named type
// like PerHour over float64), or "" for composite/interface types. Codegen uses
// this instead of guessing from the rendered type string.
func basicKind(t types.Type) string {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return ""
	}
	return basic.Name()
}

// isScalarBasic reports whether t's underlying type is a basic scalar kind the
// view can render as a default control — a named type over bool/int/float/string
// (e.g. PerHour over float64, or a bare bool) counts. Composite types (slices,
// structs, maps) are computed roots, not editable inputs.
func isScalarBasic(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	info := basic.Info()
	return info&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
}
