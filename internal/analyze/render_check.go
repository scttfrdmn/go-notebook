package analyze

import (
	"fmt"
	"go/token"
	"go/types"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// checkRenderShape validates that any cell result whose type has a Render()
// method returns the shape the engine's reflection probe requires: a struct
// with string fields named MIME and Data. It closes the one silent-failure hole
// the importless design opens.
//
// Because a notebook imports nothing from this project, a renderable cell
// defines its OWN Rendered-shaped struct and the engine discovers it by
// reflection (any Render() returning a struct with string MIME/Data). Reflection
// can't be enforced by the compiler, so a typo — `Mime` instead of `MIME`, or
// `Data []byte` — would make the cell silently not render. This check restores
// enforcement at check-time: a Render() method with the wrong shape is a
// diagnostic, not a blank cell.
//
// It only fires for a type that HAS a Render() method (an intent to render);
// a plain scalar with no Render() is fine and renders as a default readout.
func checkRenderShape(fset *token.FileSet, cellID graph.CellID, cellPos graph.Position, results *types.Tuple) []graph.Diagnostic {
	var diags []graph.Diagnostic
	for i := 0; i < results.Len(); i++ {
		rt := results.At(i).Type()
		method := renderMethod(rt)
		if method == nil {
			continue // no Render() method → not trying to render; fine
		}
		if d, ok := renderShapeDiagnostic(cellID, cellPos, method); ok {
			diags = append(diags, d)
		}
	}
	return diags
}

// renderMethod returns the Render method of a type (value or pointer receiver),
// or nil if it has none.
func renderMethod(t types.Type) *types.Func {
	// Check both the type and its pointer, since Render may have a pointer
	// receiver while the cell returns a value (or vice versa).
	for _, cand := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(cand)
		for i := 0; i < ms.Len(); i++ {
			if fn, ok := ms.At(i).Obj().(*types.Func); ok && fn.Name() == "Render" {
				return fn
			}
		}
	}
	return nil
}

// renderShapeDiagnostic checks a Render method's signature and returns a
// diagnostic (and true) if it does not match the required shape:
// func() T where T is a struct with string fields MIME and Data.
func renderShapeDiagnostic(cellID graph.CellID, cellPos graph.Position, method *types.Func) (graph.Diagnostic, bool) {
	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return graph.Diagnostic{}, false
	}
	bad := func(reason, hint string) (graph.Diagnostic, bool) {
		return graph.Diagnostic{
			Pos:      cellPos,
			Severity: graph.Error,
			Msg:      fmt.Sprintf("cell %q has a Render() method, but %s; it will not render.", cellID, reason),
			Hint:     hint,
		}, true
	}

	if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return bad("Render must take no arguments and return one value",
			"the render probe calls value.Render() and reads MIME and Data from the result")
	}
	res := sig.Results().At(0).Type()
	st, ok := underlyingStruct(res)
	if !ok {
		return bad("Render must return a struct",
			"return a struct with string fields MIME and Data (e.g. Rendered{MIME, Data string})")
	}
	mime, data := structStringField(st, "MIME"), structStringField(st, "Data")
	if !mime {
		return bad("its Render() result has no string field named MIME",
			`did you mean "MIME"? (the field name is case-sensitive: MIME, not Mime)`)
	}
	if !data {
		return bad("its Render() result has no string field named Data",
			"the render probe needs a string Data field alongside MIME")
	}
	return graph.Diagnostic{}, false
}

// underlyingStruct returns the struct underlying t (dereferencing a pointer and
// unwrapping a named type), and whether t is struct-shaped.
func underlyingStruct(t types.Type) (*types.Struct, bool) {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	return st, ok
}

// structStringField reports whether the struct has an exported field of the
// given name with an underlying string type.
func structStringField(st *types.Struct, name string) bool {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() != name {
			continue
		}
		basic, ok := f.Type().Underlying().(*types.Basic)
		return ok && basic.Kind() == types.String
	}
	return false
}
