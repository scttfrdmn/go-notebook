package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Runtime executes the notebook's dependency graph reactively. It owns the head
// (the only mutable state), the cache, and the event stream, and it runs a wave
// per edit: an immutable snapshot of the head propagated through the dirty
// subgraph, with independent cells fanned out onto goroutines.
//
// The scheduler is the load-bearing piece of the whole design. Its one
// correctness obligation — glitch-freedom — is that no cell ever observes
// inputs from two different epochs. That falls out of reading only from an
// immutable per-wave snapshot; it cannot be retrofitted onto a scheduler that
// reads shared mutable state.
type Runtime struct {
	nodes map[CellID]Node
	// deps[c] are the cells c consumes from (producers of its inputs).
	deps map[CellID][]CellID
	// levels are topological levels: cells within a level are independent and
	// run concurrently; level i+1 depends only on levels <= i.
	levels [][]CellID
	// producer maps a symbol to the cell that produces it.
	producer map[Symbol]CellID
	// leaves are the input symbols written through the head.
	leaves map[LeafID]bool

	head  *Head
	cache Store

	mu      sync.Mutex
	subs    []chan Event
	current Epoch // the newest epoch that has begun a wave; used to supersede
	// waves holds the cancel func of each in-flight wave by epoch, so a newer
	// edit can cancel older waves' contexts — turning supersession from "discard
	// the result" into "abandon the compute" for cells that honor ctx.Done().
	waves map[Epoch]context.CancelFunc
	// versions holds the current version of each symbol's value, for cache keys.
	// A version bumps only when the value actually changes (propagation
	// pruning), so an identical recompute neither invalidates the cache nor
	// wakes the subtree.
	versions map[Symbol]uint64
	lastVals map[Symbol]any
	vmu      sync.Mutex

	// serial disables goroutine fan-out (cells in a level run sequentially).
	// This restores per-cell stdout for debugging, at the cost of parallelism.
	serial bool

	// finals holds the most recent committed value of every symbol, for
	// headless --json output. Guarded by vmu.
	finals map[Symbol]any
}

// Config describes the static graph shape the runtime executes. It is derived
// from the notebook graph (by the caller) and is independent of go/types — the
// runtime consumes plain data, just like the IR.
type Config struct {
	// Nodes are the executable cells.
	Nodes []Node
	// Leaves are the input symbols the user writes (slider roots, etc.).
	Leaves []LeafID
	// Levels are the precomputed topological levels over all cells, from the
	// graph's Plan. Cells within a level are independent.
	Levels [][]CellID
	// Serial disables goroutine fan-out: cells within a level run one at a
	// time. Used by --serial to restore per-cell stdout for debugging.
	Serial bool
}

// NewRuntime builds a runtime from a config, a head, and a cache.
func NewRuntime(cfg Config, head *Head, cache Store) *Runtime {
	r := &Runtime{
		nodes:    make(map[CellID]Node, len(cfg.Nodes)),
		deps:     make(map[CellID][]CellID),
		levels:   cfg.Levels,
		producer: make(map[Symbol]CellID),
		leaves:   make(map[LeafID]bool, len(cfg.Leaves)),
		head:     head,
		cache:    cache,
		versions: make(map[Symbol]uint64),
		lastVals: make(map[Symbol]any),
		finals:   make(map[Symbol]any),
		waves:    make(map[Epoch]context.CancelFunc),
		serial:   cfg.Serial,
	}
	for _, n := range cfg.Nodes {
		r.nodes[n.ID()] = n
		for _, out := range n.Out() {
			r.producer[out] = n.ID()
		}
	}
	for _, l := range cfg.Leaves {
		r.leaves[l] = true
	}
	// Resolve each cell's dependencies: the producers of its input symbols that
	// are themselves cells (not bare leaves).
	for _, n := range cfg.Nodes {
		var ds []CellID
		seen := map[CellID]bool{}
		for _, in := range n.In() {
			if p, ok := r.producer[in]; ok && p != n.ID() && !seen[p] {
				seen[p] = true
				ds = append(ds, p)
			}
		}
		r.deps[n.ID()] = ds
	}

	// Levels are supplied by the caller (tests) or computed from the nodes'
	// declared shape (the generated main, which stays topology-free).
	if r.levels == nil {
		r.levels = computeLevels(cfg.Nodes, r.producer)
	}
	return r
}

// Subscribe returns a channel of events for one consumer. Each subscriber gets
// its own channel; the engine never blocks on a slow consumer beyond the
// channel buffer. engine/server subscribes here — the engine itself never
// imports a transport.
//
// The events carry only the rendered projection you should read: Out ({mime,
// data}) and the lifecycle fields. This is the WIRE-SAFE contract — the SSE and
// WASM transports subscribe here and project each event through [ToWire], which
// ignores Event.Value, so no arbitrary Go value is ever marshalled. A consumer
// that wants the typed Go value must ask for it by name via [SubscribeValues].
func (r *Runtime) Subscribe() <-chan Event {
	return r.subscribe()
}

// SubscribeValues returns a channel like [Subscribe], but names the out-side
// capability of reading Event.Value — the cell's typed Go value for the wave,
// not its string readout. It is the symmetric partner of the input capability
// probes (Bounded/Optioned/Reconciler): inputs are probed for what they accept,
// this names what a consumer may read on the way out. It is for IN-PROCESS Go
// consumers only; the typed value never crosses a wire (see [Event.Value]). The
// fan-out is shared with [Subscribe] — the two differ by contract, not
// mechanism: only a SubscribeValues consumer is promised Value is populated.
func (r *Runtime) SubscribeValues() <-chan Event {
	return r.subscribe()
}

// subscribe registers one buffered channel with the emit fan-out. Both
// [Subscribe] and [SubscribeValues] go through here so there is exactly one
// fan-out; the methods differ only in the contract they document.
func (r *Runtime) subscribe() <-chan Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan Event, 256)
	r.subs = append(r.subs, ch)
	return ch
}

// emit sends an event to all subscribers, dropping it for any subscriber whose
// buffer is full rather than blocking the wave.
func (r *Runtime) emit(ev Event) {
	r.mu.Lock()
	subs := r.subs
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Set writes a leaf through the head (the single mutation chokepoint), bumping
// the epoch, then runs the resulting wave. It returns when the wave settles or
// is superseded by a newer edit.
func (r *Runtime) Set(ctx context.Context, leaf LeafID, v any) {
	epoch := r.head.Set(leaf, v)
	r.bumpVersions(Outputs{leaf: v})
	r.runEdit(ctx, epoch)
}

// SetMany writes several leaves as ONE atomic edit and runs a SINGLE wave over
// all of them. The values enter the head under one epoch (see [Head.SetMany]), so
// a host changing several related inputs together — principal, rate, term — gets
// one coherent recompute, never three waves with intermediate combinations a
// subscriber could observe. It is the batch form of [Set], sharing the same
// supersede-and-run tail; the returned epoch is the one the resulting wave (and
// its settled marker) carries, so a caller can correlate the edit with its result.
func (r *Runtime) SetMany(ctx context.Context, vals map[LeafID]any) Epoch {
	epoch := r.head.SetMany(vals)
	// Version every changed leaf, so pure cells reading any of them recompute —
	// the same reason Set bumps its one leaf (else a cached stale value serves).
	out := make(Outputs, len(vals))
	for leaf, v := range vals {
		out[leaf] = v // LeafID is an alias for Symbol
	}
	r.bumpVersions(out)
	r.runEdit(ctx, epoch)
	return epoch
}

// runEdit is the shared tail of Set/SetMany: record this as the newest epoch,
// cancel every older in-flight wave (so a superseded wave's ctx.Done()-honoring
// cells abandon their compute rather than finishing to a discarded result), then
// run the wave over a fresh head snapshot. Factored out so Set and SetMany cannot
// drift in how they supersede and schedule.
func (r *Runtime) runEdit(ctx context.Context, epoch Epoch) {
	r.mu.Lock()
	if epoch > r.current {
		r.current = epoch
	}
	for e, cancel := range r.waves {
		if e < epoch {
			cancel()
		}
	}
	r.mu.Unlock()

	snap, snapEpoch := r.head.Snapshot()
	r.runWave(ctx, snapEpoch, snap)
}

// RunAll executes a full wave over the whole graph at the current head state.
// It is used once at startup so every cell renders before any edit, and after a
// rebuild when the process restarts with a restored head.
func (r *Runtime) RunAll(ctx context.Context) {
	snap, epoch := r.head.Snapshot()
	r.runWave(ctx, epoch, snap)
}

// superseded reports whether a newer edit has arrived since this wave's epoch.
func (r *Runtime) superseded(epoch Epoch) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current > epoch
}

// runWave executes every cell for one wave against an immutable snapshot of the
// head. This is the heart of glitch-freedom: every cell in the wave reads leaf
// values from `snap` and cell outputs from `values`, a per-wave map seeded from
// the snapshot — never from shared mutable state. No cell can observe a value
// from a different epoch, because this wave has its own isolated value space.
func (r *Runtime) runWave(parent context.Context, epoch Epoch, snap map[LeafID]any) {
	// Derive a per-wave context, cancelled either when this call returns or when
	// a newer edit supersedes this epoch (see Set). A slow cell that honors
	// ctx.Done() abandons its work the moment it is superseded.
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	// If a newer wave already started before we registered, cancel immediately.
	if r.current > epoch {
		cancel()
	}
	r.waves[epoch] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.waves, epoch)
		r.mu.Unlock()
		cancel()
	}()

	// values is this wave's private symbol space: leaves first, then cell
	// outputs as they are produced. It is never shared with another wave.
	values := make(map[Symbol]any, len(snap))
	for k, v := range snap {
		values[k] = v
	}

	// blocked holds cells that must not run because an upstream failed.
	blocked := make(map[CellID]bool)

	for _, level := range r.levels {
		// Supersede check at each level boundary: a newer edit means this
		// wave's remaining work is wasted, so stop and let the newer wave win.
		if r.superseded(epoch) {
			r.markStale(epoch, level)
			return
		}

		results := r.runLevel(ctx, epoch, level, values, blocked)

		// Commit this level's results into the wave's value space. Committing
		// after the whole level completes keeps a level's cells independent.
		for _, res := range results {
			if res.blocked {
				blocked[res.id] = true
				r.emit(Event{Epoch: epoch, Cell: res.id, State: StateBlocked})
				continue
			}
			if res.err != nil {
				blocked[res.id] = true
				r.emit(Event{Epoch: epoch, Cell: res.id, State: StateError, Err: res.err.Error()})
				continue
			}
			for k, v := range res.out {
				values[k] = v
			}
			r.bumpVersions(res.out)
			r.recordFinals(res.out)
			r.emit(r.doneEvent(epoch, res))
		}
	}

	// The wave ran every level without being superseded: emit one terminal
	// wave-settled marker (empty Cell) so a program consumer knows a COHERENT set
	// of values — every cell that was going to update in this epoch — has arrived.
	// Re-check supersession first: a newer edit may have landed while the last
	// level ran, and in that case the newer wave will emit its own settled marker;
	// emitting here too would let a consumer buffering by epoch see an older epoch
	// settle after a newer one's values. A superseded wave returns above (markStale)
	// and never reaches here.
	if !r.superseded(epoch) {
		r.emit(Event{Epoch: epoch, State: StateSettled})
	}
}

// recordFinals stores the latest committed value of each produced symbol, for
// headless --json output.
func (r *Runtime) recordFinals(out Outputs) {
	r.vmu.Lock()
	defer r.vmu.Unlock()
	for k, v := range out {
		r.finals[k] = v
	}
}

// Finals returns a copy of the most recent committed value of every symbol,
// after a wave. It is the batch/headless output: run once, read the results.
func (r *Runtime) Finals() map[Symbol]any {
	r.vmu.Lock()
	defer r.vmu.Unlock()
	cp := make(map[Symbol]any, len(r.finals))
	for k, v := range r.finals {
		cp[k] = v
	}
	return cp
}

// leafSymbol returns a leaf cell's single output symbol and true, or ("",false)
// if the node is not a single-output leaf.
func (r *Runtime) leafSymbol(node Node) (Symbol, bool) {
	outs := node.Out()
	if len(outs) != 1 {
		return "", false // multi-output cells are not simple leaves
	}
	sym := outs[0]
	if !r.leaves[sym] {
		return "", false
	}
	return sym, true
}

// reconcileLeaf implements the §4.3 leaf rule: the cell body computes the
// SCHEMA; the head holds the user's SELECTION; the leaf's value is the schema
// reconciled against the selection. This is the leaf path finally built — it was
// under-specified since M5, and only a widget (whose schema and selection are
// different things) revealed it. For a scalar leaf the two ARE the same thing,
// so reconcile is the identity and a slider behaves exactly as before.
//
// One rule, applied uniformly — never special-cased by kind, because that is how
// the single Head.Set chokepoint would quietly fork:
//
//   - schema not a Reconciler (a scalar, or a widget with no Reconcile) → the
//     saved selection replaces the schema wholesale (identity: a slider's saved
//     float IS its value).
//   - schema is a Reconciler → schema.Reconcile(saved) merges them per widget
//     kind (Range clamps, Multi filters, Select falls back, Draggable resets).
//
// schema is the freshly-run body output; saved is the head value for this leaf
// (absent on first run — then the schema, i.e. the default, stands).
func (r *Runtime) reconcileLeaf(schema any, saved any, hasSaved bool) any {
	if !hasSaved {
		return schema // never edited: the cell body's default stands
	}
	if rec, ok := AsReconciler(schema); ok {
		return rec.Reconcile(saved)
	}
	return saved // scalar / non-reconciling widget: the selection is the value
}

// levelResult is one cell's outcome within a level.
type levelResult struct {
	id      CellID
	out     Outputs
	err     error
	blocked bool
}

// runLevel runs every cell in a level concurrently (the design's parallel
// fan-out) against the wave's current value space, and returns their results.
// Reads happen from `values`, which is not mutated during the level, so the
// concurrent reads are race-free; writes are collected and committed by the
// caller after the level completes.
func (r *Runtime) runLevel(ctx context.Context, epoch Epoch, level []CellID, values map[Symbol]any, blocked map[CellID]bool) []levelResult {
	results := make([]levelResult, len(level))
	var wg sync.WaitGroup
	for i, id := range level {
		i, id := i, id
		node := r.nodes[id]
		if node == nil {
			results[i] = levelResult{id: id, blocked: true}
			continue
		}
		// If any dependency is blocked or failed, this cell is blocked.
		if r.dependsOnBlocked(id, blocked) {
			results[i] = levelResult{id: id, blocked: true}
			continue
		}

		// Leaf reconcile (§4.3): a leaf cell's body computes the SCHEMA; the head
		// holds the user's SELECTION; the leaf's value is the schema reconciled
		// against the selection. Run the body for the schema, then reconcile the
		// head value in. For a scalar leaf (slider, --set) the schema and
		// selection are the same thing, so reconcile is the identity and the
		// edited value flows downstream exactly as before. For a widget they
		// differ — the body's data-derived options/bounds are kept, the saved
		// selection is clamped/filtered/reset into them.
		//
		// The saved selection is read from `values` (seeded from the head
		// snapshot) BEFORE the body runs, since the body produces the same symbol.
		if sym, ok := r.leafSymbol(node); ok {
			saved, hasSaved := values[sym]
			in := make(Inputs, len(node.In()))
			for _, s := range node.In() {
				in[s] = values[s]
			}
			r.emit(Event{Epoch: epoch, Cell: id, State: StateRunning})
			res := r.runNode(ctx, node, in)
			if res.err == nil {
				if schema, ok := res.out[sym]; ok {
					// Stamp the leaf identity onto the value BEFORE reconcile, so a
					// draggable's grips (drawn downstream in a foreign cell) know which
					// leaf they write. A no-op for widgets without the WithLeaf seam.
					schema = StampLeaf(schema, string(sym))
					res.out = Outputs{sym: r.reconcileLeaf(schema, saved, hasSaved)}
				}
			}
			results[i] = res
			continue
		}

		// Pure cells consult the cache, keyed on input versions (never a value
		// hash). Impure cells — those that transitively touch time, rand, or
		// I/O — skip the cache entirely, because their output is not a function
		// of their inputs alone.
		if node.Pure() {
			key := r.cacheKey(id, node.In())
			if out, ok := r.cache.Get(key); ok {
				results[i] = levelResult{id: id, out: out}
				r.emit(Event{Epoch: epoch, Cell: id, State: StateRunning})
				continue
			}
		}

		r.emit(Event{Epoch: epoch, Cell: id, State: StateRunning})

		// Build this cell's inputs by reading (not writing) the shared value
		// space. Only the wired inputs the node declares are passed.
		in := make(Inputs, len(node.In()))
		for _, sym := range node.In() {
			in[sym] = values[sym]
		}

		run := func() {
			res := r.runNode(ctx, node, in)
			if res.err == nil && node.Pure() {
				r.cache.Put(r.cacheKey(node.ID(), node.In()), res.out)
			}
			results[i] = res
		}
		if r.serial {
			// Sequential: cells in a level run one at a time, so per-cell stdout
			// doesn't interleave. This is the --serial debugging mode.
			run()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			run()
		}()
	}
	wg.Wait()
	return results
}

// runNode executes a single cell, recovering a panic into an error state. This
// is the only recover site in the codebase — a cell panic becomes a typed error
// on that node, never process death.
func (r *Runtime) runNode(ctx context.Context, node Node, in Inputs) (res levelResult) {
	res.id = node.ID()
	defer func() {
		if p := recover(); p != nil {
			res.out = nil
			res.err = fmt.Errorf("cell %q panicked: %v", node.ID(), p)
		}
	}()
	out, err := node.Run(ctx, in)
	res.out = out
	res.err = err
	return res
}

// dependsOnBlocked reports whether any of a cell's dependencies is blocked or
// failed, so the failure propagates downstream as StateBlocked rather than a
// wrong value.
func (r *Runtime) dependsOnBlocked(id CellID, blocked map[CellID]bool) bool {
	for _, dep := range r.deps[id] {
		if blocked[dep] {
			return true
		}
	}
	return false
}

// markStale emits StateStale for the cells of a superseded wave's current level
// so subscribers know the wave was abandoned.
func (r *Runtime) markStale(epoch Epoch, level []CellID) {
	for _, id := range level {
		r.emit(Event{Epoch: epoch, Cell: id, State: StateStale})
	}
}

// doneEvent builds the StateDone event for a completed cell, attaching output
// per the display degradation ladder: a Renderable value carries its rich view;
// a bare scalar (a float64/int/bool/string, or a named type over one — e.g.
// utilization's rho) carries a text/plain readout so the value the graph
// computes is visible, not invisible, and updates as upstream leaves change. A
// value that is neither (a raw slice/struct with no Render) carries nothing and
// the transport leaves it hidden. This is the output-side mirror of the control
// ladder ("losing the view costs polish, never correctness", design.md:88) and
// the "caller-chosen default readout" render.go anticipates. No API change —
// Out was always the display seam; scalars simply stop being nil.
func (r *Runtime) doneEvent(epoch Epoch, res levelResult) Event {
	ev := Event{Epoch: epoch, Cell: res.id, State: StateDone}
	// A cell with a single output is the common view case. Order: a Renderable
	// carries its rich picture; a widget carries its structured STATE (so a
	// Multi/Select/Range value reaches the client, not nil); a bare scalar
	// carries a text readout. Widget state travels in the existing Out.Data as
	// JSON under a widget MIME — no new Event field, same capability-probe
	// discipline as Rendered.
	for _, v := range res.out {
		if rendered, ok := AsRendered(v); ok {
			rc := rendered
			ev.Out = &rc
			ev.Value = v
			break
		}
		if wv, ok := AsWidgetView(v); ok {
			if data, err := json.Marshal(wv); err == nil {
				ev.Out = &Rendered{MIME: WidgetMIME, Data: string(data)}
				ev.Value = v
				break
			}
		}
		if txt, ok := scalarReadout(v); ok {
			ev.Out = &Rendered{MIME: "text/plain", Data: txt}
			ev.Value = v
			break
		}
	}
	return ev
}

// WidgetMIME tags an Out whose Data is a JSON [WidgetView] — the client
// dispatches on the cell's static Kind (CellMeta) and reads this live state.
const WidgetMIME = "application/x-notebook-widget+json"

// scalarReadout formats v as a plain-text readout when it is a scalar — a basic
// kind or a named type over one (PerHour, Seconds, Probability). Composite kinds
// (slice, struct, map) return false: they have no obvious one-line form and are
// left to a Render() method or hidden. Uses reflect.Kind, so a `type USD float64`
// formats like its float64 underlying without the engine knowing the named type.
func scalarReadout(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return fmt.Sprintf("%v", rv.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'g', 6, 64), true
	case reflect.String:
		return rv.String(), true
	default:
		return "", false
	}
}

// bumpVersions increments the stored version of a produced symbol only when its
// value actually changed since last time (the structural `changed` probe). An
// identical recompute leaves the version untouched — which is propagation
// pruning: downstream cache keys don't change, so the subtree isn't woken.
func (r *Runtime) bumpVersions(out Outputs) {
	r.vmu.Lock()
	defer r.vmu.Unlock()
	for sym, v := range out {
		prev, seen := r.lastVals[sym]
		if !seen || changed(prev, v) {
			r.versions[sym]++
			r.lastVals[sym] = v
		}
	}
}

// cacheKey builds a version-based cache key for a cell given its input symbols.
func (r *Runtime) cacheKey(id CellID, in []Symbol) Key {
	r.vmu.Lock()
	defer r.vmu.Unlock()
	parts := make([]string, 0, len(in))
	sorted := append([]Symbol(nil), in...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, sym := range sorted {
		parts = append(parts, string(sym)+":"+strconv.FormatUint(r.versions[sym], 10))
	}
	return Key{Cell: id, Vers: strings.Join(parts, ",")}
}
