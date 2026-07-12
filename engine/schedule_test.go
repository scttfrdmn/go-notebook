package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fnNode is a test Node backed by a closure. Production cells are generated;
// this lets tests build arbitrary graph shapes.
type fnNode struct {
	id   CellID
	in   []Symbol
	out  []Symbol
	pure bool
	run  func(ctx context.Context, in Inputs) (Outputs, error)
}

func (n fnNode) ID() CellID    { return n.id }
func (n fnNode) In() []Symbol  { return n.in }
func (n fnNode) Out() []Symbol { return n.out }
func (n fnNode) Pure() bool    { return n.pure }
func (n fnNode) Run(ctx context.Context, in Inputs) (Outputs, error) {
	return n.run(ctx, in)
}

// TestGlitchFreedom is the correctness bug the whole scheduler exists to
// prevent, written before the scheduler works.
//
// Diamond: a -> {b, c} -> d. The leaf `x` feeds a; a feeds both b and c; d
// consumes b and c. b is deliberately slow. We stamp each wave's value of a
// with its epoch, and assert that whenever d runs, the b-value and c-value it
// sees carry the SAME epoch. A scheduler that reads a shared mutable head
// (rather than an immutable per-wave snapshot) can let d observe b from an old
// epoch and c from a new one — a glitch, a number the user briefly sees that
// was never true.
func TestGlitchFreedom(t *testing.T) {
	var mismatches int64

	// a stamps the current x (the leaf) — its output is the epoch-bearing value.
	a := fnNode{
		id: "a", in: []Symbol{"x"}, out: []Symbol{"av"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"av": in["x"].(int)}, nil
		},
	}
	// b is slow: it gives a newer wave time to start and race.
	b := fnNode{
		id: "b", in: []Symbol{"av"}, out: []Symbol{"bv"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			time.Sleep(5 * time.Millisecond)
			return Outputs{"bv": in["av"].(int)}, nil
		},
	}
	c := fnNode{
		id: "c", in: []Symbol{"av"}, out: []Symbol{"cv"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"cv": in["av"].(int)}, nil
		},
	}
	// d asserts its two inputs agree. If they carry different epochs, that is a
	// glitch.
	d := fnNode{
		id: "d", in: []Symbol{"bv", "cv"}, out: []Symbol{"dv"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			if in["bv"].(int) != in["cv"].(int) {
				atomic.AddInt64(&mismatches, 1)
			}
			return Outputs{"dv": 0}, nil
		},
	}

	cfg := Config{
		Nodes:  []Node{a, b, c, d},
		Leaves: []LeafID{"x"},
		Levels: [][]CellID{{"a"}, {"b", "c"}, {"d"}},
	}
	head := NewHead()
	head.Set("x", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// Fire edits CONCURRENTLY so waves overlap: while a slow b for epoch N is
	// running, the edit for epoch N+1 starts. A scheduler that reads a shared
	// mutable value space (rather than an isolated per-wave one) will let d
	// observe b from one epoch and c from another. Only per-wave isolation
	// prevents it — which is what this asserts, under -race, with overlap.
	var wg sync.WaitGroup
	for i := 1; i <= 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rt.Set(context.Background(), "x", n)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt64(&mismatches); got != 0 {
		t.Fatalf("glitch detected: d observed mismatched epochs %d times", got)
	}
}

// TestSupersede: fire many edits concurrently; the scheduler must coalesce so
// that exactly one wave settles and the rest are reported stale. This is
// drag-coalescing — 300 drag events, one settled recompute — and it is free
// given epoch-checking before commit.
func TestSupersede(t *testing.T) {
	const edits = 100
	var settled int64

	// running closes when the first wave's sink cell begins — proving that wave
	// is registered and in-flight. The sink then blocks on release, so the wave
	// stays open while the later edits arrive; a superseded wave's ctx is
	// cancelled and it bails. This makes overlap deterministic. With instant
	// cells a wave could finish before the next concurrent edit bumped the epoch,
	// so nothing was ever in-flight to supersede — a flake on fast runners that
	// hoped goroutine scheduling would interleave. We no longer hope.
	running := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	leaf := fnNode{
		id: "double", in: []Symbol{"n"}, out: []Symbol{"d"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"d": in["n"].(int) * 2}, nil
		},
	}
	// A terminal cell counts how many waves reach a committed StateDone.
	sink := fnNode{
		id: "sink", in: []Symbol{"d"}, out: []Symbol{"s"},
		run: func(ctx context.Context, in Inputs) (Outputs, error) {
			once.Do(func() { close(running) })
			select {
			case <-release:
			case <-ctx.Done(): // superseded: abandon promptly
				return nil, ctx.Err()
			}
			return Outputs{"s": in["d"]}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{leaf, sink},
		Leaves: []LeafID{"n"},
		Levels: [][]CellID{{"double"}, {"sink"}},
	}
	head := NewHead()
	head.Set("n", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	sub := rt.Subscribe()
	go func() {
		for ev := range sub {
			if ev.Cell == "sink" && ev.State == StateDone {
				atomic.AddInt64(&settled, 1)
			}
		}
	}()

	// Prime the pump: fire the first edit and wait until its sink cell is
	// actually running, so its wave is registered and mid-compute. Track its Set
	// so we can wait for it to return (it will be superseded and bail).
	primeDone := make(chan struct{})
	go func() { rt.Set(context.Background(), "n", 1); close(primeDone) }()
	<-running

	// Flood the rest. Each bumps the epoch (head.Set, synchronous at Set entry)
	// and cancels older in-flight waves. A superseded wave's sink bails on
	// ctx.Done immediately — only the newest, un-superseded wave stays blocked on
	// release, so exactly it will commit once released.
	var wg sync.WaitGroup
	for i := 2; i <= edits; i++ {
		wg.Add(1)
		go func(n int) { defer wg.Done(); rt.Set(context.Background(), "n", n) }(i)
	}

	// Wait on the observed condition — every edit applied — so the primed wave is
	// provably superseded before we release. The final epoch is edits+1 (the +1
	// is the head.Set("n", 0) at setup). No duration is involved: a real hang
	// fails at waitFor's deadline instead of a sleep masking it.
	waitFor(t, func() bool { return head.Epoch() >= Epoch(edits+1) }, "all edits applied")
	close(release) // unblock the one surviving wave; superseded ones already bailed

	// Every Set goroutine returns once its wave settles or is superseded. When
	// they have all returned, no more sink-done events will be emitted — the
	// storm is quiescent, observed, not slept for.
	wg.Wait()
	<-primeDone

	// The surviving wave's sink-done event is emitted synchronously before its
	// Set returns, but the counting goroutine drains asynchronously; wait for it
	// to observe at least the one guaranteed settle. `settled` only grows and no
	// new events can arrive now, so once it's >= 1 the count is final.
	waitFor(t, func() bool { return atomic.LoadInt64(&settled) >= 1 }, "the surviving wave to settle")

	// Far fewer than `edits` settled (most superseded) — coalescing happened.
	got := atomic.LoadInt64(&settled)
	if got == edits {
		t.Errorf("no coalescing: all %d edits settled; expected supersession", edits)
	}
}
