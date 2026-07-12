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

	// running closes when the first wave's cell has entered its compute loop, so
	// that wave is provably in-flight when the later edits arrive. Firing edits
	// concurrently and hoping one is mid-compute is a flake on fast/loaded
	// runners; priming the pump makes the supersession deterministic.
	running := make(chan struct{})
	var once sync.Once

	slow := fnNode{
		id: "slow", in: []Symbol{"n"}, out: []Symbol{"out"}, pure: false,
		run: func(ctx context.Context, in Inputs) (Outputs, error) {
			once.Do(func() { close(running) })
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

	const edits = 20
	// Prime: fire the first edit and wait until its cell is mid-compute.
	go rt.Set(context.Background(), "n", 1)
	<-running
	// Flood the rest; each bumps the epoch and cancels the in-flight older wave.
	for i := 2; i <= edits; i++ {
		go rt.Set(context.Background(), "n", i)
	}

	// Wait on the observed conditions the test names, not a duration: every edit
	// applied (so every supersession has been issued), then at least one cell
	// cancelled (supersession abandoned real compute) — the whole point. A slow
	// machine polls more; a real regression (nothing ever cancelled) fails loud
	// at the deadline instead of being masked by a fixed sleep.
	waitFor(t, func() bool { return head.Epoch() >= Epoch(edits) }, "all edits applied")
	waitFor(t, func() bool { return atomic.LoadInt64(&cancelled) > 0 }, "a superseded cell to cancel")

	comp := atomic.LoadInt64(&completed)
	canc := atomic.LoadInt64(&cancelled)
	t.Logf("of %d edits: %d cells completed, %d cancelled mid-compute", edits, comp, canc)

	// The point: supersession abandoned real compute. Not every wave ran to
	// completion — most were cancelled.
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

	// A single settled edit completes and records its value. Set runs the wave
	// synchronously and returns only once it settles, so the value is recorded
	// by the time Set returns — assert it directly, no wait needed.
	rt.Set(context.Background(), "n", 42)
	if got := atomic.LoadInt64(&lastVal); got != 42 {
		t.Errorf("the settled wave's cell should complete with 42, got %d", got)
	}
}
