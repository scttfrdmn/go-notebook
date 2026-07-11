package engine

import (
	"context"
	"testing"
)

// TestLeafOverride is the fix for the "sliders were silently inert" bug: a leaf
// cell's body is only the default. When the head holds a value for the leaf's
// symbol, that value flows downstream instead of the cell's hardcoded default.
func TestLeafOverride(t *testing.T) {
	// servers() default 80; downstream doubles it.
	servers := fnNode{
		id: "servers", in: nil, out: []Symbol{"c"}, pure: true,
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			return Outputs{"c": 80}, nil // the DEFAULT
		},
	}
	double := fnNode{
		id: "double", in: []Symbol{"c"}, out: []Symbol{"d"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"d": in["c"].(int) * 2}, nil
		},
	}
	cfg := Config{Nodes: []Node{servers, double}, Leaves: []LeafID{"c"}}

	// No head value: the default (80) flows, d = 160.
	head := NewHead()
	rt := NewRuntime(cfg, head, NewMemoStore())
	rt.RunAll(context.Background())
	if got := rt.Finals()["d"]; got != 160 {
		t.Errorf("with no head value, d = %v, want 160 (default 80 doubled)", got)
	}

	// Set the leaf: the head value (120) overrides the default, d = 240.
	rt.Set(context.Background(), "c", 120)
	if got := rt.Finals()["d"]; got != 240 {
		t.Errorf("after Set c=120, d = %v, want 240 (120 doubled) — leaf override failed", got)
	}
	// And the leaf cell itself reports the overridden value, not its default.
	if got := rt.Finals()["c"]; got != 120 {
		t.Errorf("leaf c = %v, want 120 (overridden), not its default 80", got)
	}
}

// TestFinalsAfterWave confirms Finals captures every cell's latest value, the
// basis of --json batch output.
func TestFinalsAfterWave(t *testing.T) {
	a := fnNode{
		id: "a", in: []Symbol{"x"}, out: []Symbol{"y"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"y": in["x"].(int) + 1}, nil
		},
	}
	cfg := Config{Nodes: []Node{a}, Leaves: []LeafID{"x"}}
	head := NewHead()
	head.Set("x", 41)
	rt := NewRuntime(cfg, head, NewMemoStore())
	rt.RunAll(context.Background())

	finals := rt.Finals()
	if finals["y"] != 42 {
		t.Errorf("finals[y] = %v, want 42", finals["y"])
	}
}

// TestSerialExecution confirms --serial runs cells one at a time. We can't
// directly observe goroutine count, but we can confirm correctness is
// unaffected: the same graph produces the same result serially.
func TestSerialExecution(t *testing.T) {
	mk := func(id CellID, in, out Symbol) fnNode {
		return fnNode{
			id: id, in: []Symbol{in}, out: []Symbol{out}, pure: true,
			run: func(_ context.Context, m Inputs) (Outputs, error) {
				return Outputs{out: m[in].(int) + 1}, nil
			},
		}
	}
	nodes := []Node{
		fnNode{id: "root", in: []Symbol{"x"}, out: []Symbol{"r"}, pure: true,
			run: func(_ context.Context, m Inputs) (Outputs, error) {
				return Outputs{"r": m["x"].(int)}, nil
			}},
		mk("b", "r", "bv"),
		mk("c", "r", "cv"),
	}
	cfg := Config{Nodes: nodes, Leaves: []LeafID{"x"}, Serial: true}
	head := NewHead()
	head.Set("x", 10)
	rt := NewRuntime(cfg, head, NewMemoStore())
	rt.RunAll(context.Background())

	f := rt.Finals()
	if f["bv"] != 11 || f["cv"] != 11 {
		t.Errorf("serial run: bv=%v cv=%v, want 11,11", f["bv"], f["cv"])
	}
}
