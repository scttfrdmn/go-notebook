package engine

import (
	"context"
	"testing"
)

// TestLeafProperty_EveryLeafAffectsDownstream is the property that would have
// caught the "sliders were silently inert" bug, stated as a property rather
// than a case: for every leaf, setting it to a value different from its default
// must change at least one downstream value. A leaf that can be edited without
// anything downstream moving is not reactive — it is a dead control.
//
// This generalizes to every widget kind added later: whatever the control, an
// edit must propagate. Instantiate it on real notebooks (via the built binary)
// and on any synthetic graph.
func TestLeafProperty_EveryLeafAffectsDownstream(t *testing.T) {
	// A graph where every leaf has a downstream: two leaves feed one sink.
	leafA := fnNode{
		id: "a", out: []Symbol{"x"}, pure: true,
		run: func(_ context.Context, _ Inputs) (Outputs, error) { return Outputs{"x": 1}, nil },
	}
	leafB := fnNode{
		id: "b", out: []Symbol{"y"}, pure: true,
		run: func(_ context.Context, _ Inputs) (Outputs, error) { return Outputs{"y": 10}, nil },
	}
	sink := fnNode{
		id: "sink", in: []Symbol{"x", "y"}, out: []Symbol{"s"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"s": in["x"].(int) + in["y"].(int)}, nil
		},
	}
	cfg := Config{Nodes: []Node{leafA, leafB, sink}, Leaves: []LeafID{"x", "y"}}

	leaves := map[LeafID]int{"x": 1, "y": 10} // leaf → its default value

	for leaf, def := range leaves {
		// Baseline: run with defaults, capture the sink.
		rt := NewRuntime(cfg, NewHead(), NewMemoStore())
		rt.RunAll(context.Background())
		before := rt.Finals()["s"]

		// Set this leaf to a value ≠ its default; the sink must change.
		rt.Set(context.Background(), leaf, def+1000)
		after := rt.Finals()["s"]

		if before == after {
			t.Errorf("leaf %q: editing it (%v→%v) did not change downstream sink (stayed %v) — the control is inert",
				leaf, def, def+1000, before)
		}
	}
}
