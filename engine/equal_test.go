package engine

import (
	"context"
	"sync/atomic"
	"testing"
)

// bigResult is a slice-backed type — not comparable with ==, so propagation
// pruning must fall to the Equal(any) rung. It reports equal when contents
// match, so a recompute producing the same contents should NOT wake downstream.
type bigResult struct {
	data []int
}

func (b bigResult) Equal(other any) bool {
	o, ok := other.(bigResult)
	if !ok || len(o.data) != len(b.data) {
		return false
	}
	for i := range b.data {
		if b.data[i] != o.data[i] {
			return false
		}
	}
	return true
}

// TestEqualPruningRung exercises the middle rung of the propagation ladder
// (== → Equal(any) → assume-changed). A slice-backed value falls past == to
// Equal; when a recompute yields an equal value, the downstream must not re-run.
//
// This is the rung that had zero corpus coverage — every example output was a
// non-comparable type with no Equal, so it fell straight to "assume changed."
func TestEqualPruningRung(t *testing.T) {
	var downstreamRuns int64

	// producer echoes leaf n into a bigResult, but ALWAYS the same contents
	// regardless of n (simulating a cell whose output is stable under an input
	// change — the case pruning exists for).
	producer := fnNode{
		id: "producer", in: []Symbol{"n"}, out: []Symbol{"big"}, pure: true,
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			return Outputs{"big": bigResult{data: []int{1, 2, 3}}}, nil
		},
	}
	consumer := fnNode{
		id: "consumer", in: []Symbol{"big"}, out: []Symbol{"c"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			atomic.AddInt64(&downstreamRuns, 1)
			return Outputs{"c": len(in["big"].(bigResult).data)}, nil
		},
	}
	cfg := Config{Nodes: []Node{producer, consumer}, Leaves: []LeafID{"n"}}
	head := NewHead()
	head.Set("n", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// First wave: consumer runs once (miss).
	rt.Set(context.Background(), "n", 1)
	first := atomic.LoadInt64(&downstreamRuns)

	// Change n several times. producer recomputes each time but yields an EQUAL
	// bigResult, so its version must not bump, so consumer must not re-run.
	for i := 2; i <= 6; i++ {
		rt.Set(context.Background(), "n", i)
	}
	after := atomic.LoadInt64(&downstreamRuns)

	if after != first {
		t.Errorf("consumer re-ran %d times after producer yielded Equal values; "+
			"the Equal pruning rung did not fire (want no additional runs)", after-first)
	}
}

// TestEqualRungDirect unit-tests the changed() probe across the ladder:
// comparable == , Equal(any), and the conservative fallback.
func TestChangedLadder(t *testing.T) {
	// comparable: == rung
	if changed(3, 3) {
		t.Error("identical comparable values should be unchanged (== rung)")
	}
	if !changed(3, 4) {
		t.Error("different comparable values should be changed")
	}
	// Equal(any) rung: slice-backed, equal contents
	a := bigResult{data: []int{1, 2, 3}}
	b := bigResult{data: []int{1, 2, 3}}
	if changed(a, b) {
		t.Error("Equal-implementing values with equal contents should be unchanged (Equal rung)")
	}
	if !changed(a, bigResult{data: []int{9}}) {
		t.Error("Equal-implementing values with different contents should be changed")
	}
	// conservative fallback: a non-comparable type with no Equal → assume changed
	type noEq struct{ s []int }
	if !changed(noEq{[]int{1}}, noEq{[]int{1}}) {
		t.Error("a non-comparable type without Equal must conservatively be treated as changed")
	}
}
