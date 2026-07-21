package analyze

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// checkCapabilityShapes validates the signatures of structural control-capability
// methods on a cell's result types. It is the sibling of [checkRenderShape] for
// the input side: a notebook imports nothing from this project, so a control is
// recognized by a method of the right SHAPE (Bounds/Options/Reconcile), probed by
// the engine via a runtime interface assertion (engine.Bounded/Optioned/
// Reconciler). Reflection-style assertion can't be enforced by the compiler, so a
// method named right but shaped wrong — `Bounds() (int, int)` instead of
// `Bounds() (float64, float64)` — fails the runtime probe and the control simply
// never appears, with no explanation. This restores check-time enforcement: a
// near-miss capability method is a diagnostic naming the actual vs. required
// signature, not a silently-missing control.
//
// Like checkRenderShape, it fires ONLY for a method that exists (an intent to be
// that control). A type with no Bounds method is simply not a range — that is not
// an error. Grip is deliberately NOT checked: its signature (Grip(i int) Ref) is
// notebook-defined and the engine asserts no static Grip interface, so there is
// no required shape to check against — presence alone selects the draggable kind.
func checkCapabilityShapes(cellID graph.CellID, cellPos graph.Position, results *types.Tuple) []graph.Diagnostic {
	var diags []graph.Diagnostic
	for i := 0; i < results.Len(); i++ {
		rt := results.At(i).Type()
		for _, cap := range capabilityChecks {
			method := methodNamed(rt, cap.name)
			if method == nil {
				continue // not trying to be this control; fine
			}
			if d, ok := cap.diagnose(cellID, cellPos, method); ok {
				diags = append(diags, d)
			}
		}
	}
	return diags
}

// capabilityCheck is one structural control capability the engine asserts by
// interface at runtime: the method name to look for and a signature validator.
type capabilityCheck struct {
	name     string
	diagnose func(cellID graph.CellID, pos graph.Position, m *types.Func) (graph.Diagnostic, bool)
}

// capabilityChecks are the input capabilities with an engine-enforced signature
// (engine/widget.go: Bounded, Optioned, Reconciler). Grip is absent on purpose
// (see the package doc above).
var capabilityChecks = []capabilityCheck{
	{"Bounds", diagnoseBounds},
	{"Options", diagnoseOptions},
	{"Reconcile", diagnoseReconcile},
}

// diagnoseBounds requires Bounds() (float64, float64) — a range control. The most
// common near-miss is Bounds() (int, int), which the reviewer flagged: detected by
// name, classified as a range, then silently dropped by the runtime float64 probe.
func diagnoseBounds(cellID graph.CellID, pos graph.Position, m *types.Func) (graph.Diagnostic, bool) {
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return graph.Diagnostic{}, false
	}
	if sig.Params().Len() == 0 && resultsAreBasic(sig, types.Float64, types.Float64) {
		return graph.Diagnostic{}, false // correct
	}
	return capabilityDiag(cellID, pos, got(m),
		"Bounds() (float64, float64)",
		"a range control needs two float64 bounds; the runtime probe reads them as float64 (use float64 even for an integer range)")
}

// diagnoseOptions requires Options() []string — a select or multi-select.
func diagnoseOptions(cellID graph.CellID, pos graph.Position, m *types.Func) (graph.Diagnostic, bool) {
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return graph.Diagnostic{}, false
	}
	if sig.Params().Len() == 0 && sig.Results().Len() == 1 && isStringSlice(sig.Results().At(0).Type()) {
		return graph.Diagnostic{}, false // correct
	}
	return capabilityDiag(cellID, pos, got(m),
		"Options() []string",
		"a select/multi-select reads its choices as []string; return the option labels as strings")
}

// diagnoseReconcile requires Reconcile(saved any) any — a stateful widget that
// survives a wave (Range/Multi/Select/Table/Draggable).
func diagnoseReconcile(cellID graph.CellID, pos graph.Position, m *types.Func) (graph.Diagnostic, bool) {
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return graph.Diagnostic{}, false
	}
	if sig.Params().Len() == 1 && isEmptyInterface(sig.Params().At(0).Type()) &&
		sig.Results().Len() == 1 && isEmptyInterface(sig.Results().At(0).Type()) {
		return graph.Diagnostic{}, false // correct
	}
	return capabilityDiag(cellID, pos, got(m),
		"Reconcile(saved any) any",
		"the engine passes the wire value as `any` and stores the returned `any`; use exactly `Reconcile(saved any) any`")
}

// capabilityDiag builds the shared near-miss diagnostic: the cell, the capability,
// the actual signature, and the required one.
func capabilityDiag(cellID graph.CellID, pos graph.Position, actual, want, hint string) (graph.Diagnostic, bool) {
	return graph.Diagnostic{
		Pos:      pos,
		Severity: graph.Error,
		Msg: fmt.Sprintf("cell %q declares %s, but a control requires %s; it will not render as a control.",
			cellID, actual, want),
		Hint: hint,
	}, true
}

// methodNamed returns the named method of a type (value or pointer receiver), or
// nil. Mirrors renderMethod for the capability names.
func methodNamed(t types.Type, name string) *types.Func {
	for _, cand := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(cand)
		for i := 0; i < ms.Len(); i++ {
			if fn, ok := ms.At(i).Obj().(*types.Func); ok && fn.Name() == name {
				return fn
			}
		}
	}
	return nil
}

// got renders a method's actual signature as `Name(params) results` for the
// diagnostic, e.g. "Bounds() (int, int)".
func got(m *types.Func) string {
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return m.Name() + "(?)"
	}
	return m.Name() + strings.TrimPrefix(types.TypeString(sig, nil), "func")
}

// resultsAreBasic reports whether sig returns exactly the given basic kinds.
func resultsAreBasic(sig *types.Signature, kinds ...types.BasicKind) bool {
	res := sig.Results()
	if res.Len() != len(kinds) {
		return false
	}
	for i, k := range kinds {
		b, ok := res.At(i).Type().Underlying().(*types.Basic)
		if !ok || b.Kind() != k {
			return false
		}
	}
	return true
}

// isStringSlice reports whether t is []string (through any named type).
func isStringSlice(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// isEmptyInterface reports whether t is the empty interface (any / interface{}).
func isEmptyInterface(t types.Type) bool {
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.NumMethods() == 0
}
