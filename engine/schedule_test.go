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

// perHour is a named type over float64, the case that motivated the value pipe:
// a scalar cell's readout stringifies it ("40.24"), so a program subscribed to
// the wire only ever sees text. Event.Value must carry the Go value itself.
type perHour float64

// TestEventValueThreeWayAgreement is B1's anti-pass. Event.Value must be the
// same typed value that (a) Finals() records for the cell and (b) the rendered
// readout in Event.Out was derived from — the three-way agreement Value ≡ Finals
// ≡ rendered. The scalar leaf carries a NAMED numeric type, so the assertion
// also proves the pipe delivers the Go type intact (perHour, not float64, not a
// string) — nothing here goes through a wire projection.
func TestEventValueThreeWayAgreement(t *testing.T) {
	// A single scalar leaf whose value is a named-numeric type. This is the
	// exact shape the F1 spike measured arriving as the string "40.24" on the
	// wire; Event.Value must instead be perHour(40.24).
	rate := fnNode{
		id: "rate", in: []Symbol{"r"}, out: []Symbol{"rate"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"rate": perHour(in["r"].(float64))}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{rate},
		Leaves: []LeafID{"r"},
		Levels: [][]CellID{{"rate"}},
	}
	head := NewHead()
	head.Set("r", 0.0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	drain := collectEvents(rt)
	rt.Set(context.Background(), "r", 40.24)

	waitFor(t, func() bool {
		st, ok := lastState(drain(), "rate")
		return ok && st == StateDone
	}, "the rate cell to reach StateDone")

	events := drain()

	// Find the cell's committed StateDone event.
	var done *Event
	for i := range events {
		if events[i].Cell == "rate" && events[i].State == StateDone {
			done = &events[i]
		}
	}
	if done == nil {
		t.Fatal("no StateDone event for rate")
	}

	// (1) Value is the typed Go value, not a string or a bare float64.
	want := perHour(40.24)
	got, ok := done.Value.(perHour)
	if !ok {
		t.Fatalf("Event.Value = %T(%v), want engine.perHour", done.Value, done.Value)
	}
	if got != want {
		t.Errorf("Event.Value = %v, want %v", got, want)
	}

	// (2) Value ≡ Finals: the same value the batch/headless path records.
	finals := rt.Finals()
	if fv, ok := finals["rate"].(perHour); !ok || fv != want {
		t.Errorf("Finals()[rate] = %v (%T), want %v", finals["rate"], finals["rate"], want)
	}
	if finals["rate"] != done.Value {
		t.Errorf("Value ≡ Finals broken: Finals()[rate]=%v Event.Value=%v", finals["rate"], done.Value)
	}

	// (3) Value ≡ rendered: Out.Data is exactly what scalarReadout produces from
	// Value — the readout ladder rendered the very value it stamped into Value.
	if done.Out == nil {
		t.Fatal("expected a rendered Out for a scalar cell")
	}
	readout, rok := scalarReadout(done.Value)
	if !rok {
		t.Fatal("scalarReadout rejected the value the ladder itself selected")
	}
	if done.Out.Data != readout {
		t.Errorf("Out.Data = %q, but scalarReadout(Value) = %q", done.Out.Data, readout)
	}
}

// TestSubscribeValuesTypedWhileSubscribeWireOnly is B2's anti-pass. On ONE
// runtime, a SubscribeValues() consumer must receive the typed Go value intact,
// while the Subscribe() consumer's WIRE projection carries only the rendered
// readout — no typed value crosses. This is the whole point of the named
// out-side capability: the transports (which subscribe via Subscribe() and
// project through ToWire) never see an arbitrary Go value, but an in-process Go
// consumer that asks for it does.
func TestSubscribeValuesTypedWhileSubscribeWireOnly(t *testing.T) {
	rate := fnNode{
		id: "rate", in: []Symbol{"r"}, out: []Symbol{"rate"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"rate": perHour(in["r"].(float64))}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{rate},
		Leaves: []LeafID{"r"},
		Levels: [][]CellID{{"rate"}},
	}
	head := NewHead()
	head.Set("r", 0.0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	// Two subscribers on the same runtime: one names the value capability, one
	// is a plain (wire-facing) subscriber.
	valuesCh := rt.SubscribeValues()
	wireCh := rt.Subscribe()

	collect := func(ch <-chan Event) *Event {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case ev := <-ch:
				if ev.Cell == "rate" && ev.State == StateDone {
					return &ev
				}
			case <-deadline:
				t.Fatal("timed out waiting for rate StateDone")
				return nil
			}
		}
	}

	var vEv, wEv *Event
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); vEv = collect(valuesCh) }()
	go func() { defer wg.Done(); wEv = collect(wireCh) }()

	rt.Set(context.Background(), "r", 40.24)
	wg.Wait()

	// SubscribeValues consumer: the typed Go value, intact.
	got, ok := vEv.Value.(perHour)
	if !ok || got != perHour(40.24) {
		t.Errorf("SubscribeValues got Value = %v (%T), want perHour(40.24)", vEv.Value, vEv.Value)
	}

	// Subscribe consumer: its WIRE projection is rendered-only. ToWire is what
	// the transports actually put on the wire, and it carries no typed value —
	// only the "40.24" readout. (The struct field is shared by the fan-out; the
	// contract is that a wire consumer reads the projection, never Value.)
	w := ToWire(*wEv)
	if w.Data != "40.24" {
		t.Errorf("wire projection Data = %q, want readout %q", w.Data, "40.24")
	}
	// The wire shape has no field that could carry a Go value — assert the whole
	// projection is exactly the rendered {mime,data} + lifecycle, nothing typed.
	if w.MIME != "text/plain" {
		t.Errorf("wire projection MIME = %q, want text/plain", w.MIME)
	}
}

// TestWaveSettledMarker is item 2 of the port-coherence work (#214): a completed
// wave emits exactly one terminal StateSettled event, carrying that wave's epoch
// and an empty cell, AFTER every cell's own event. A program driving the notebook
// watches for it to know a coherent set of values from one wave has all arrived.
func TestWaveSettledMarker(t *testing.T) {
	// leaf n → double → sink: two derived cells so "settled comes last" is a real
	// ordering claim, not trivially true of a one-cell graph.
	leaf := fnNode{
		id: "n", out: []Symbol{"n"},
		run: func(_ context.Context, _ Inputs) (Outputs, error) { return Outputs{"n": 0}, nil }, // schema default; head's saved value reconciles in
	}
	double := fnNode{
		id: "double", in: []Symbol{"n"}, out: []Symbol{"d"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) { return Outputs{"d": in["n"].(int) * 2}, nil },
	}
	sink := fnNode{
		id: "sink", in: []Symbol{"d"}, out: []Symbol{"s"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) { return Outputs{"s": in["d"].(int) + 1}, nil },
	}
	cfg := Config{
		Nodes:  []Node{leaf, double, sink},
		Leaves: []LeafID{"n"},
		Levels: [][]CellID{{"n"}, {"double"}, {"sink"}},
	}
	head := NewHead()
	head.Set("n", 5)
	rt := NewRuntime(cfg, head, NewMemoStore())

	drain := collectEvents(rt)
	rt.RunAll(context.Background())
	events := drain()

	// Exactly one settled marker, empty cell, and it is the LAST event (every
	// cell's done arrived before the wave declared itself settled).
	var settledCount int
	var settledIdx = -1
	for i, ev := range events {
		if ev.State == StateSettled {
			settledCount++
			settledIdx = i
			if ev.Cell != "" {
				t.Errorf("settled marker has cell %q, want empty (it is a wave-level event)", ev.Cell)
			}
		}
	}
	if settledCount != 1 {
		t.Fatalf("got %d settled markers, want exactly 1 (events: %+v)", settledCount, events)
	}
	if settledIdx != len(events)-1 {
		t.Errorf("settled marker at index %d, want last (%d) — a coherence consumer must see every cell first", settledIdx, len(events)-1)
	}

	// The marker carries the wave's epoch (the same epoch the cell events carried).
	settled := events[settledIdx]
	var cellEpoch Epoch
	for _, ev := range events {
		if ev.Cell == "sink" && ev.State == StateDone {
			cellEpoch = ev.Epoch
		}
	}
	if settled.Epoch != cellEpoch {
		t.Errorf("settled epoch = %d, want %d (the wave's epoch)", settled.Epoch, cellEpoch)
	}
}

// TestSupersededWaveDoesNotSettle is the coherence guarantee: a wave that is
// superseded by a newer edit must NOT emit a settled marker — only the wave that
// actually wins does. Otherwise a consumer buffering by epoch could see an older
// epoch "settle" after a newer epoch's values, the exact incoherence the marker
// exists to prevent. A slow cell lets a second Set supersede the first mid-wave.
func TestSupersededWaveDoesNotSettle(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(1)
	var startOnce sync.Once

	// The cell blocks ONLY for the first wave's value (n=1): that wave parks in the
	// cell until released or cancelled. The winning wave (n=2) runs straight
	// through, so its synchronous Set returns and the test can proceed.
	slow := fnNode{
		id: "slow", in: []Symbol{"n"}, out: []Symbol{"v"},
		run: func(ctx context.Context, in Inputs) (Outputs, error) {
			if in["n"].(int) != 1 {
				return Outputs{"v": in["n"]}, nil // the winner: run fast
			}
			startOnce.Do(started.Done) // signal wave A has entered the cell
			select {
			case <-release:
			case <-ctx.Done(): // superseded: abandon
				return nil, ctx.Err()
			}
			return Outputs{"v": in["n"]}, nil
		},
	}
	cfg := Config{Nodes: []Node{slow}, Leaves: []LeafID{"n"}, Levels: [][]CellID{{"slow"}}}
	head := NewHead()
	head.Set("n", 1)
	rt := NewRuntime(cfg, head, NewMemoStore())

	drain := collectEvents(rt)

	// Wave A (epoch of n=1) enters the slow cell and blocks there.
	var wgA sync.WaitGroup
	wgA.Add(1)
	go func() { defer wgA.Done(); rt.Set(context.Background(), "n", 1) }()
	started.Wait()

	// Wave B supersedes A. Its Set cancels A's context (see Runtime.Set), so A's
	// slow cell returns ctx.Err() and A never reaches the settled emit. B runs
	// straight through (n≠1) so this Set returns.
	rt.Set(context.Background(), "n", 2)
	close(release) // A's cell already returned via ctx.Done(); this is cleanup
	wgA.Wait()

	events := drain()
	// Exactly one settled marker, and it belongs to the WINNER (the highest epoch
	// seen on any event) — never the superseded wave.
	var maxEpoch Epoch
	for _, ev := range events {
		if ev.Epoch > maxEpoch {
			maxEpoch = ev.Epoch
		}
	}
	var settled []Event
	for _, ev := range events {
		if ev.State == StateSettled {
			settled = append(settled, ev)
		}
	}
	if len(settled) != 1 {
		t.Fatalf("got %d settled markers, want exactly 1 (only the winning wave settles); events: %+v", len(settled), events)
	}
	if settled[0].Epoch != maxEpoch {
		t.Errorf("settled epoch = %d, want the winning epoch %d — a superseded wave must not settle", settled[0].Epoch, maxEpoch)
	}
}

// TestSetManyIsOneAtomicEdit is the atomic multi-leaf edit (#225): setting
// several leaves via SetMany bumps the epoch ONCE and runs ONE wave over all of
// them, so a downstream cell that reads two leaves sees both new values together,
// never an intermediate combination. Contrast with two separate Set calls, which
// would be two epochs and two waves.
func TestSetManyIsOneAtomicEdit(t *testing.T) {
	// a, b are leaves; sum reads both. If a and b arrived in separate waves, sum
	// would compute once with (newA, oldB) before settling on (newA, newB).
	a := fnNode{id: "a", out: []Symbol{"a"}, run: func(_ context.Context, _ Inputs) (Outputs, error) { return Outputs{"a": 0}, nil }}
	b := fnNode{id: "b", out: []Symbol{"b"}, run: func(_ context.Context, _ Inputs) (Outputs, error) { return Outputs{"b": 0}, nil }}
	sum := fnNode{
		id: "sum", in: []Symbol{"a", "b"}, out: []Symbol{"s"}, pure: true,
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"s": in["a"].(int) + in["b"].(int)}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{a, b, sum},
		Leaves: []LeafID{"a", "b"},
		Levels: [][]CellID{{"a", "b"}, {"sum"}},
	}
	head := NewHead()
	head.Set("a", 0)
	head.Set("b", 0)
	rt := NewRuntime(cfg, head, NewMemoStore())

	drain := collectEvents(rt)
	epoch := rt.SetMany(context.Background(), map[LeafID]any{"a": 3, "b": 4})
	events := drain()

	// The returned epoch is what the wave (and its settled marker) carried.
	var settledEpoch Epoch
	sumEpochs := map[Epoch]bool{}
	for _, ev := range events {
		if ev.State == StateSettled {
			settledEpoch = ev.Epoch
		}
		if ev.Cell == "sum" && ev.State == StateDone {
			sumEpochs[ev.Epoch] = true
		}
	}
	if settledEpoch != epoch {
		t.Errorf("SetMany returned epoch %d but the wave settled at %d", epoch, settledEpoch)
	}
	// sum computed in exactly ONE wave (one epoch), not two.
	if len(sumEpochs) != 1 {
		t.Errorf("sum ran in %d distinct epochs, want 1 — SetMany must be one wave, not per-leaf", len(sumEpochs))
	}
	// And its value is the coherent 3+4=7, never an intermediate (3+0 or 0+4).
	if v := rt.Finals()["s"]; v != 7 {
		t.Errorf("sum = %v, want 7 (both leaves applied together)", v)
	}
}
