package engine

import (
	"context"
	"fmt"
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
	// versions holds the current version of each symbol's value, for cache keys.
	// A version bumps only when the value actually changes (propagation
	// pruning), so an identical recompute neither invalidates the cache nor
	// wakes the subtree.
	versions map[Symbol]uint64
	lastVals map[Symbol]any
	vmu      sync.Mutex
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
	return r
}

// Subscribe returns a channel of events for one consumer. Each subscriber gets
// its own channel; the engine never blocks on a slow consumer beyond the
// channel buffer. engine/server subscribes here — the engine itself never
// imports a transport.
func (r *Runtime) Subscribe() <-chan Event {
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

	// Record this as the newest epoch. A wave for an older epoch that observes
	// current > its own epoch is stale and must not commit.
	r.mu.Lock()
	if epoch > r.current {
		r.current = epoch
	}
	r.mu.Unlock()

	snap, snapEpoch := r.head.Snapshot()
	r.runWave(ctx, snapEpoch, snap)
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
func (r *Runtime) runWave(ctx context.Context, epoch Epoch, snap map[LeafID]any) {
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
			r.emit(r.doneEvent(epoch, res))
		}
	}
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

		wg.Add(1)
		go func() {
			defer wg.Done()
			res := r.runNode(ctx, node, in)
			if res.err == nil && node.Pure() {
				r.cache.Put(r.cacheKey(node.ID(), node.In()), res.out)
			}
			results[i] = res
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

// doneEvent builds the StateDone event for a completed cell, attaching rendered
// output when the cell's (single) value is Renderable.
func (r *Runtime) doneEvent(epoch Epoch, res levelResult) Event {
	ev := Event{Epoch: epoch, Cell: res.id, State: StateDone}
	// Attach a rendered blob if any output value is Renderable. A cell with a
	// single output is the common view case.
	for _, v := range res.out {
		if rendered, ok := AsRendered(v); ok {
			rc := rendered
			ev.Out = &rc
			break
		}
	}
	return ev
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
