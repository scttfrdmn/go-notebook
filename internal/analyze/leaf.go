package analyze

import (
	"go/types"
	"strings"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

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
		// A root control: a plain scalar the view renders as a text field /
		// checkbox (the plainest degradation rung), or a widget — an input-widget
		// capability (Bounds/Options), or an editable widget struct with a Value
		// (a parameterless Table/Draggable like holdings() Table[Lot], which
		// supplies its own starting rows). A renderable output was already
		// excluded above, so a struct-with-Value here is an editable control.
		return isScalarBasic(only) || isInputWidget(only) || isSelectionWidget(only)
	}
	// A cell WITH parameters is a leaf when its output is a widget: a stateful
	// control (Range/Multi/Select/Draggable/Table) whose choices/bounds are
	// DATA-DERIVED — computed from the parameters each wave — while the head
	// holds the user's selection. themePicker(all []Set) Multi[Theme] computes
	// its options from the data and is a leaf; that is what makes data-derived
	// options work (a cell returning a widget is a leaf whether or not it has
	// parameters). The gate is widget-capability, NOT Reconcile alone — Multi and
	// Select expose Options(), not Reconcile(), yet are plainly inputs.
	//
	// Not every parameterized widget-capable value is an input, though: a
	// COMPUTED scalar that merely carries Bounds() (waitProbability(a,c)
	// Probability) is a result, not a control. So the gate is the SELECTION
	// capabilities — Options (select/multi), Reconcile (a stateful widget), or a
	// composite Value the user edits — not a bare Bounds() on a scalar.
	return isSelectionWidget(only)
}

// leafResultType returns a leaf cell's single non-error result type — the type
// the widget descriptor is derived from. Assumes isLeafCell already passed (so
// exactly one non-error result exists).
func leafResultType(sig *types.Signature) types.Type {
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		t := results.At(i).Type()
		if !isErrorType(t) {
			return t
		}
	}
	return nil
}

// widgetMeta derives a leaf's static control descriptor from its type. Kind is
// decided by CAPABILITY, never a type-name switch: the client dispatches on it,
// but the analyzer classifies by the same method-shape probing the leaf rule
// uses. A directive only refines rendering later; this is the type's own say.
//
//   - Options() + a slice Value field   → "multi"  (a multi-select)
//   - Options() + a scalar Value field  → "select" (a single choice)
//   - Bounds()                          → "range"
//   - a slice Value field + Grip method → "draggable"
//   - a slice Value field of structs    → "table"  (with a column schema)
//   - underlying basic bool             → "bool"   (a checkbox)
//   - otherwise                         → "" (a scalar text/slider rung; nil meta)
//
// Only a Table carries Columns — a grid needs its row type's fields, which the
// runtime value cannot supply to the client. Everything else renders from the
// live [engine.WidgetView] the value produces.
func widgetMeta(t types.Type) *graph.WidgetMeta {
	if t == nil {
		return nil
	}
	switch {
	case hasMethod(t, "Options"):
		if isSliceValueWidget(t) {
			return &graph.WidgetMeta{Kind: "multi"}
		}
		return &graph.WidgetMeta{Kind: "select"}
	case hasMethod(t, "Bounds"):
		return &graph.WidgetMeta{Kind: "range"}
	case hasMethod(t, "Grip"):
		return &graph.WidgetMeta{Kind: "draggable"}
	case isSliceValueWidget(t):
		// A stateful editable collection with no Options/Grip is a table; its
		// grid needs the row type's columns, which are T's fields at codegen.
		return &graph.WidgetMeta{Kind: "table", Columns: tableColumns(t)}
	}
	if b, ok := t.Underlying().(*types.Basic); ok && b.Info()&types.IsBoolean != 0 {
		return &graph.WidgetMeta{Kind: "bool"}
	}
	return nil // a scalar rung (number/string) — the client's default control
}

// isSliceValueWidget reports whether t is a widget struct with a slice-typed
// exported Value field (Multi[T]{Value []T}, Draggable[T]{Value []T},
// Table[T]{Value []T}). Distinguishes multi from select and finds a table.
func isSliceValueWidget(t types.Type) bool {
	f, ok := valueField(t)
	return ok && isSliceType(f)
}

// valueField returns the type of an exported Value field on t's underlying
// struct, if any.
func valueField(t types.Type) (types.Type, bool) {
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	for i := 0; i < st.NumFields(); i++ {
		if f := st.Field(i); f.Exported() && f.Name() == "Value" {
			return f.Type(), true
		}
	}
	return nil, false
}

func isSliceType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Slice)
	return ok
}

// tableColumns derives the grid schema from a Table's row type: the fields of
// the element type of its Value slice. A grid cannot be rendered from the
// runtime value alone — it needs column names and coarse types, which are
// properties of the row type T, known here at codegen.
func tableColumns(t types.Type) []graph.WidgetColumn {
	f, ok := valueField(t)
	if !ok {
		return nil
	}
	sl, ok := f.Underlying().(*types.Slice)
	if !ok {
		return nil
	}
	st, ok := sl.Elem().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	var cols []graph.WidgetColumn
	for i := 0; i < st.NumFields(); i++ {
		fld := st.Field(i)
		if !fld.Exported() {
			continue
		}
		cols = append(cols, graph.WidgetColumn{Name: fld.Name(), Type: coarseType(fld.Type())})
	}
	return cols
}

// coarseType maps a field type to the client's cell-editor category.
func coarseType(t types.Type) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch {
		case b.Info()&types.IsBoolean != 0:
			return "bool"
		case b.Info()&types.IsNumeric != 0:
			return "number"
		case b.Info()&types.IsString != 0:
			return "string"
		}
	}
	// A named type over a basic (Date, USD, Ticker) — treat by its underlying,
	// else fall back to string for the editor.
	if strings.Contains(types.TypeString(t, nil), "int") ||
		strings.Contains(types.TypeString(t, nil), "float") {
		return "number"
	}
	return "string"
}

// isInputWidget reports whether a type is an input control by capability:
// Bounds() (ranged) or Options() (select). Reconcile alone does not make a
// parameterless cell an input (it is about merging a saved selection), but a
// parameterless Reconciler is unusual; Bounds/Options are the root-control
// capabilities.
func isInputWidget(t types.Type) bool {
	return hasMethod(t, "Bounds") || hasMethod(t, "Options")
}

// isSelectionWidget reports whether a parameterized cell's output is a
// data-derived selection widget — a stateful control the user edits — as
// opposed to a computed scalar that merely happens to carry Bounds().
//
// The distinguisher is STRUCT vs SCALAR. A widget is a struct holding both its
// data-derived schema and the user's selection: Range{Lo,Hi,From,To},
// Multi/Select{All,Value}, Table/Draggable{Value []T}. A computed result like
// waitProbability(a,c) Probability is a scalar named type over float64 that
// carries Bounds() for display but is not editable. So:
//
//   - Options()                 → select/multi (a choice widget), always a leaf
//   - a struct with Bounds()    → a Range widget (has a From/To selection), leaf
//   - a struct with a Value fld → Table/Draggable (editable collection), leaf
//   - a bare scalar with Bounds → a computed result, NOT a leaf
//
// Reconcile is not required (Multi/Select/Range don't implement it yet); the
// struct-with-schema shape is the real tell.
func isSelectionWidget(t types.Type) bool {
	if hasMethod(t, "Options") {
		return true
	}
	_, isStruct := t.Underlying().(*types.Struct)
	if !isStruct {
		return false // a scalar named type (Probability) is a computed result
	}
	// A widget struct: it reconciles a saved selection, carries a numeric range
	// (Bounds), or holds an editable Value. Any of these marks a stateful control
	// distinct from a computed scalar that merely carries Bounds().
	if hasMethod(t, "Reconcile") || hasMethod(t, "Bounds") {
		return true
	}
	_, hasValue := valueField(t)
	return hasValue
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
