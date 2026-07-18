//go:build js && wasm

package wasm

import (
	"context"
	"reflect"
	"syscall/js"
	"testing"

	"github.com/scttfrdmn/go-notebook/engine"
)

// fnNode is a test Node backed by a closure (the wasm package can't see the
// engine package's own test node). Enough to build a one-cell graph.
type fnNode struct {
	id  engine.CellID
	in  []engine.Symbol
	out []engine.Symbol
	run func(ctx context.Context, in engine.Inputs) (engine.Outputs, error)
}

func (n fnNode) ID() engine.CellID   { return n.id }
func (n fnNode) In() []engine.Symbol { return n.in }
func (n fnNode) Out() []engine.Symbol {
	return n.out
}
func (n fnNode) Pure() bool { return false }
func (n fnNode) Run(ctx context.Context, in engine.Inputs) (engine.Outputs, error) {
	return n.run(ctx, in)
}

// newTestPort builds a port over a trivial runtime: one leaf `x` (a scalar
// cell) whose value the head holds. Enough to exercise set/subscribe/values
// without a generated notebook.
func newTestPort(t *testing.T) (*port, *engine.Runtime) {
	t.Helper()
	x := fnNode{
		id: "x", out: []engine.Symbol{"x"},
		run: func(_ context.Context, in engine.Inputs) (engine.Outputs, error) {
			if v, ok := in["x"]; ok {
				return engine.Outputs{"x": v}, nil
			}
			return engine.Outputs{"x": 1.0}, nil // default
		},
	}
	rt := engine.NewRuntime(engine.Config{
		Nodes:  []engine.Node{x},
		Leaves: []engine.LeafID{"x"},
	}, engine.NewHead(), engine.NewMemoStore())
	meta := []engine.CellMeta{{ID: "x", Leaf: "x"}}
	return &port{rt: rt, meta: meta}, rt
}

// TestPortSubscribeFanoutAndUnsub pins the OUT half of the named port: an event
// reaches every live subscriber, and the returned unsubscribe function stops
// delivery to just that one. This is the contract cmd/notebook/wasm_ui.go and a
// foreign host page both depend on. pump is driven synchronously (close the
// channel so it drains and returns) — deterministic, no goroutine scheduling
// under Node's single thread.
func TestPortSubscribeFanoutAndUnsub(t *testing.T) {
	p, _ := newTestPort(t)

	countA, countB := 0, 0
	fnA := js.FuncOf(func(_ js.Value, _ []js.Value) any { countA++; return nil })
	fnB := js.FuncOf(func(_ js.Value, _ []js.Value) any { countB++; return nil })

	unsubA := p.subscribe(fnA.Value).(js.Func)
	p.subscribe(fnB.Value)

	// First wave: one event, both subscribers live → each invoked once.
	ch1 := make(chan engine.Event, 1)
	ch1 <- engine.Event{Epoch: 1, Cell: "x", State: engine.StateDone}
	close(ch1)
	p.pump(ch1)
	if countA != 1 || countB != 1 {
		t.Fatalf("after first event: countA=%d countB=%d, want 1,1", countA, countB)
	}

	// A unsubscribes; a second event reaches only B.
	unsubA.Invoke()
	ch2 := make(chan engine.Event, 1)
	ch2 <- engine.Event{Epoch: 2, Cell: "x", State: engine.StateDone}
	close(ch2)
	p.pump(ch2)
	if countA != 1 {
		t.Errorf("unsubscribed callback still received events: countA = %d, want 1", countA)
	}
	if countB != 2 {
		t.Errorf("live callback missed an event: countB = %d, want 2", countB)
	}
}

// TestPortValuesReflectsSet pins the IN→snapshot round trip: after a leaf is
// Set and the wave settles, notebook.values() reports the new value. This is the
// pull form of the value channel — the seed a control starts at, with no
// separate seed channel to poll (the ritual the F1 spike flagged).
func TestPortValuesReflectsSet(t *testing.T) {
	p, rt := newTestPort(t)
	rt.RunAll(context.Background()) // first wave: x = default 1.0
	if got := p.values()["x"]; got != 1.0 {
		t.Fatalf("values()[x] after first wave = %v, want 1.0 (the default seed)", got)
	}
	rt.Set(context.Background(), "x", 42.0)
	if got := p.values()["x"]; got != 42.0 {
		t.Errorf("values()[x] after Set = %v, want 42.0 (the pull channel didn't reflect the edit)", got)
	}
}

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

// TestFromJSObject pins the object branch that lets a host set a composite leaf
// over the wasm port: a plain JS object (a content-addressed handle
// {Source, Rows, Schema}, or a Table row) must become map[string]any and
// homogenize through engine.CoerceWire's coerceMap exactly as the SSE path's
// JSON decode does. Without it a host's set(dataLeaf, handle) stringified to
// "<object>" over wasm while composing fine over SSE/CLI — the IN-side
// asymmetry the SQ2 survey (#100) measured. This is the wasm end of that fix.
func TestFromJSObject(t *testing.T) {
	o := js.Global().Get("Object").New()
	o.Set("Source", "trips.csv")
	o.Set("Rows", 7)
	o.Set("Schema", 123456)

	m, ok := fromJS(o).(map[string]any)
	if !ok {
		t.Fatalf("fromJS(object) = %T, want map[string]any (a handle would stringify without this)", fromJS(o))
	}
	// The whole point: it survives CoerceWire's coerceMap into the same clean map
	// the server yields, so a Rel handle reaches a leaf's Reconcile unchanged.
	norm, cok := engine.CoerceWire(m)
	if !cok {
		t.Fatalf("CoerceWire rejected the handle map from fromJS; set(dataLeaf, handle) would die in the coercer")
	}
	want := map[string]any{"Source": "trips.csv", "Rows": float64(7), "Schema": float64(123456)}
	if !reflect.DeepEqual(norm, want) {
		t.Fatalf("CoerceWire(fromJS(handle)) = %#v, want %#v", norm, want)
	}
}
