package engine_test

import (
	"fmt"

	"github.com/scttfrdmn/go-notebook/engine"
)

// A widget selection arrives from a transport as decoded JSON — numbers as
// float64, arrays as []any. CoerceWire homogenizes that wire shape into the
// clean Go value a cell's Reconcile expects (here, a []string of labels), or
// reports false when the shape is a genuine mismatch. It is the one place
// untrusted client input crosses into the engine.
func ExampleCoerceWire() {
	// What a Multi-select's labels look like after JSON decoding on the /set path.
	selection := []any{"City", "Duplo"}

	clean, ok := engine.CoerceWire(selection)
	fmt.Printf("%v %T ok=%v\n", clean, clean, ok)

	// A mixed-kind array is a real client/leaf mismatch and fails loud, never
	// silently dropped.
	_, ok = engine.CoerceWire([]any{"City", 3.0})
	fmt.Printf("mixed ok=%v\n", ok)

	// Output:
	// [City Duplo] []string ok=true
	// mixed ok=false
}

// Head is the single mutation chokepoint: every leaf edit — a slider, a timer, a
// grip drag — goes through Set, which records the value and bumps the epoch. A
// wave reads only from an immutable Snapshot, never the live map, which is what
// makes propagation glitch-free.
func ExampleHead() {
	h := engine.NewHead()

	e1 := h.Set("servers", 80)
	e2 := h.Set("servers", 120) // an edit bumps the epoch

	v, ok := h.Get("servers")
	fmt.Printf("servers=%v ok=%v  epoch %d→%d\n", v, ok, e1, e2)

	// Output:
	// servers=120 ok=true  epoch 1→2
}

// AsRendered is the structural probe the runtime uses to decide how to draw a
// cell's output: any value whose type has a Render() Rendered method is drawn as
// its MIME-tagged content. The notebook declares nothing — the method is the
// whole contract.
func ExampleAsRendered() {
	// A notebook's own type, with a Render method the engine discovers by shape.
	out := chart{title: "load"}

	r, ok := engine.AsRendered(out)
	fmt.Printf("%q ok=%v\n", r.MIME, ok)

	// A plain value with no Render method is not renderable (it falls to the
	// scalar-readout rung instead).
	_, ok = engine.AsRendered(42)
	fmt.Printf("scalar ok=%v\n", ok)

	// Output:
	// "image/svg+xml" ok=true
	// scalar ok=false
}

// chart is a stand-in for a notebook's own renderable type — the engine finds it
// by the Render() method's shape, never by importing this package.
type chart struct{ title string }

func (c chart) Render() engine.Rendered {
	return engine.Rendered{MIME: "image/svg+xml", Data: "<svg><!-- " + c.title + " --></svg>"}
}
