package engine

import (
	"context"
	"testing"
)

// BenchmarkWaveDiamond measures the pure scheduler overhead of one wave over a
// small diamond (a -> {b,c} -> d) with trivial cells. This isolates the
// engine's per-edit cost — snapshot, level walk, goroutine fan-out, event emit
// — from analysis and rendering. It is the runtime floor that KC3 (slider →
// repaint) builds on; KC3 itself is measured end-to-end through the server in
// M4.
func BenchmarkWaveDiamond(b *testing.B) {
	mk := func(id CellID, in, out Symbol) fnNode {
		return fnNode{
			id: id, in: []Symbol{in}, out: []Symbol{out}, pure: true,
			run: func(_ context.Context, m Inputs) (Outputs, error) {
				return Outputs{out: m[in].(int) + 1}, nil
			},
		}
	}
	a := fnNode{id: "a", in: []Symbol{"x"}, out: []Symbol{"av"}, pure: true,
		run: func(_ context.Context, m Inputs) (Outputs, error) {
			return Outputs{"av": m["x"].(int)}, nil
		}}
	bcell := mk("b", "av", "bv")
	ccell := mk("c", "av", "cv")
	d := fnNode{id: "d", in: []Symbol{"bv", "cv"}, out: []Symbol{"dv"}, pure: true,
		run: func(_ context.Context, m Inputs) (Outputs, error) {
			return Outputs{"dv": m["bv"].(int) + m["cv"].(int)}, nil
		}}

	cfg := Config{
		Nodes:  []Node{a, bcell, ccell, d},
		Leaves: []LeafID{"x"},
		Levels: [][]CellID{{"a"}, {"b", "c"}, {"d"}},
	}
	rt := NewRuntime(cfg, NewHead(), NewMemoStore())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Set(ctx, "x", i)
	}
}
