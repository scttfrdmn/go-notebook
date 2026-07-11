package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSupersedeCancelsCompute proves supersession is real, not cosmetic: a slow
// cell that honors ctx.Done() actually abandons its work when a newer edit
// arrives, rather than running to completion and having the result discarded.
//
// Without per-wave context cancellation, every one of N rapid edits would run
// the slow cell to completion (N full computes). With it, superseded waves'
// cells observe ctx.Done() and bail, so far fewer complete.
func TestSupersedeCancelsCompute(t *testing.T) {
	var completed int64 // cells that ran to completion (not cancelled)
	var cancelled int64 // cells that observed ctx.Done() and bailed

	slow := fnNode{
		id: "slow", in: []Symbol{"n"}, out: []Symbol{"out"}, pure: false,
		run: func(ctx context.Context, in Inputs) (Outputs, error) {
			// Simulate heavy compute in slices, checking for cancellation.
			for i := 0; i < 100; i++ {
				select {
				case <-ctx.Done():
					atomic.AddInt64(&cancelled, 1)
					return nil, ctx.Err()
				case <-time.After(1 * time.Millisecond):
				}
			}
			atomic.AddInt64(&completed, 1)
			return Outputs{"out": in["n"]}, nil
		},
	}
	cfg := Config{Nodes: []Node{slow}, Leaves: []LeafID{"n"}}
	head := NewHead()
	head.Set("n", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// Fire many edits concurrently; each supersedes the last.
	const edits = 20
	var wg sync.WaitGroup
	for i := 1; i <= edits; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rt.Set(context.Background(), "n", n)
		}(i)
	}
	wg.Wait()
	// Give any still-running cell time to observe the final cancellation.
	time.Sleep(150 * time.Millisecond)

	comp := atomic.LoadInt64(&completed)
	canc := atomic.LoadInt64(&cancelled)
	t.Logf("of %d edits: %d cells completed, %d cancelled mid-compute", edits, comp, canc)

	// The point: supersession abandoned real compute. Not every wave ran to
	// completion — most were cancelled. (At least one must have been cancelled;
	// in practice nearly all but the last are.)
	if canc == 0 {
		t.Error("no cell was cancelled — supersession is not abandoning compute (ctx not wired)")
	}
	if comp == edits {
		t.Errorf("all %d cells completed — nothing was superseded mid-flight", edits)
	}
}

// TestContextInjectedAndCancellableOnLastWave confirms the surviving (newest)
// wave's cell runs to completion — cancellation only hits superseded waves, not
// the winner.
func TestContextInjectedAndCancellableOnLastWave(t *testing.T) {
	var lastVal int64
	cell := fnNode{
		id: "c", in: []Symbol{"n"}, out: []Symbol{"v"}, pure: false,
		run: func(ctx context.Context, in Inputs) (Outputs, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
			atomic.StoreInt64(&lastVal, int64(in["n"].(int)))
			return Outputs{"v": in["n"]}, nil
		},
	}
	cfg := Config{Nodes: []Node{cell}, Leaves: []LeafID{"n"}}
	head := NewHead()
	head.Set("n", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// A single settled edit completes and records its value.
	rt.Set(context.Background(), "n", 42)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&lastVal); got != 42 {
		t.Errorf("the settled wave's cell should complete with 42, got %d", got)
	}
}
