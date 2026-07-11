package engine

import (
	"context"
	"sync/atomic"
	"testing"
)

// TestPureCellCached: a pure cell is not re-executed when its inputs are
// unchanged across waves — the version-keyed cache serves the prior result.
func TestPureCellCached(t *testing.T) {
	var runs int64
	// pure cell over leaf x.
	pc := fnNode{
		id: "pc", in: []Symbol{"x"}, out: []Symbol{"y"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			atomic.AddInt64(&runs, 1)
			return Outputs{"y": in["x"].(int) * 2}, nil
		},
	}
	// A second leaf z, independent of pc, to trigger waves without changing x.
	echo := fnNode{
		id: "echo", in: []Symbol{"z"}, out: []Symbol{"e"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"e": in["z"]}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{pc, echo},
		Leaves: []LeafID{"x", "z"},
		Levels: [][]CellID{{"pc", "echo"}},
	}
	head := NewHead()
	head.Set("x", 5)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// First wave: pc runs (miss).
	rt.Set(context.Background(), "x", 5)
	// Subsequent waves change only z; x is unchanged, so pc should be a cache
	// hit and not re-execute.
	for i := 0; i < 5; i++ {
		rt.Set(context.Background(), "z", i)
	}

	if got := atomic.LoadInt64(&runs); got > 1 {
		t.Errorf("pure cell with unchanged input ran %d times; expected 1 (cached thereafter)", got)
	}
}

// TestImpureCellNeverCached: an impure cell runs every wave, even with
// unchanged inputs — its output is not a function of its inputs alone.
func TestImpureCellNeverCached(t *testing.T) {
	var runs int64
	impure := fnNode{
		id: "clock", in: []Symbol{"x"}, out: []Symbol{"t"}, pure: false,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			atomic.AddInt64(&runs, 1)
			return Outputs{"t": in["x"]}, nil
		},
	}
	other := fnNode{
		id: "other", in: []Symbol{"z"}, out: []Symbol{"o"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"o": in["z"]}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{impure, other},
		Leaves: []LeafID{"x", "z"},
		Levels: [][]CellID{{"clock", "other"}},
	}
	head := NewHead()
	head.Set("x", 1)
	rt := NewRuntime(cfg, head, NewMemoStore())

	const waves = 5
	for i := 0; i < waves; i++ {
		rt.Set(context.Background(), "z", i) // x never changes
	}

	if got := atomic.LoadInt64(&runs); got != waves {
		t.Errorf("impure cell ran %d times; expected %d (never cached)", got, waves)
	}
}
