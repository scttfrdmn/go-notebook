//go:build js && wasm

package wasm

import (
	"reflect"
	"syscall/js"
	"testing"

	"github.com/scttfrdmn/go-notebook/engine"
)

// TestFromJSArray pins the browser write path's one non-obvious conversion: a
// JS array (what a widget selection or a grip drag's flat point set arrives as)
// must become []any so it reaches engine.CoerceWire and homogenizes exactly
// like the SSE path's JSON decode does. This regressed silently once — the
// conversion was described in a commit but never landed, so a grip drag
// stringified and the write died in the coercer with no error (#91). A scalar
// test can't catch it; only an array exercises this branch.
func TestFromJSArray(t *testing.T) {
	// A grip drag's payload: a flat [x0,y0,x1,y1] float array.
	arr := js.Global().Get("Array").New()
	for _, f := range []float64{1.5, 2.5, 3.5, 4.5} {
		arr.Call("push", f)
	}

	got := fromJS(arr)
	slice, ok := got.([]any)
	if !ok {
		t.Fatalf("fromJS(JS array) = %T, want []any (the shape CoerceWire expects)", got)
	}

	// The whole point of []any: CoerceWire homogenizes it to []float64, the shape
	// a Draggable's Reconcile asserts. If fromJS stringified instead, this fails.
	norm, cok := engine.CoerceWire(slice)
	if !cok {
		t.Fatalf("CoerceWire(%v) reported failure; the drag selection would die silently", slice)
	}
	if want := []float64{1.5, 2.5, 3.5, 4.5}; !reflect.DeepEqual(norm, want) {
		t.Fatalf("CoerceWire(fromJS(array)) = %#v, want %#v", norm, want)
	}
}

// TestFromJSStringArray covers a Multi selection: a JS array of strings must
// reach CoerceWire as []any and homogenize to []string.
func TestFromJSStringArray(t *testing.T) {
	arr := js.Global().Get("Array").New()
	for _, s := range []string{"Duplo", "City"} {
		arr.Call("push", s)
	}

	slice, ok := fromJS(arr).([]any)
	if !ok {
		t.Fatalf("fromJS(JS string array) = %T, want []any", fromJS(arr))
	}
	norm, cok := engine.CoerceWire(slice)
	if !cok {
		t.Fatal("CoerceWire failed on a string selection")
	}
	if want := []string{"Duplo", "City"}; !reflect.DeepEqual(norm, want) {
		t.Fatalf("CoerceWire(fromJS(string array)) = %#v, want %#v", norm, want)
	}
}

// TestFromJSScalars pins the base cases the scalar controls rely on — these
// always worked, but the test documents that fromJS's contract covers them so a
// future refactor of the array branch can't quietly drop them.
func TestFromJSScalars(t *testing.T) {
	if got := fromJS(js.ValueOf(42.0)); got != 42.0 {
		t.Errorf("fromJS(number) = %v, want 42.0", got)
	}
	if got := fromJS(js.ValueOf(true)); got != true {
		t.Errorf("fromJS(bool) = %v, want true", got)
	}
	if got := fromJS(js.ValueOf("hi")); got != "hi" {
		t.Errorf("fromJS(string) = %v, want hi", got)
	}
}
