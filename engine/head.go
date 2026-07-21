package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Head is the only mutable state in the system: the current value of every
// input leaf, plus the epoch that increments on each write.
//
// Every leaf write goes through the single [Head.Set] chokepoint. Sliders call
// it today; timers, buttons, and grips are all just callers of it later. This
// is a hard architectural constraint — if writes scatter, those features cannot
// be added additively — so there is deliberately no other way to mutate a leaf.
//
// A wave reads leaf values only from an immutable [Head.Snapshot], never from
// the live map. That is what makes propagation glitch-free: no cell can observe
// a half-applied edit, because there is no shared mutable view to observe.
type Head struct {
	mu    sync.Mutex
	vals  map[LeafID]any
	epoch Epoch
	path  string // persisted here; empty means in-memory only
}

// NewHead returns an empty in-memory head. Use [OpenHead] to persist.
func NewHead() *Head {
	return &Head{vals: make(map[LeafID]any)}
}

// OpenHead returns a head backed by the file at path, restoring any previously
// persisted leaf values. A missing file is not an error — it yields an empty
// head that will create the file on the first [Head.Set]. This is what makes
// process restart a non-event: the only state is a few leaf values on disk.
func OpenHead(path string) (*Head, error) {
	h := &Head{vals: make(map[LeafID]any), path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return nil, fmt.Errorf("reading head %s: %w", path, err)
	}
	if len(data) == 0 {
		return h, nil
	}
	var stored map[LeafID]any
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parsing head %s: %w", path, err)
	}
	h.vals = stored
	return h, nil
}

// Set is the ONE place a leaf is written. It records the value, bumps the
// epoch, persists (if backed by a file), and returns the new epoch so the
// caller can start a wave tagged with it.
//
// Keeping this the single chokepoint is load-bearing: a timer writing a tick, a
// button incrementing a counter, and a grip dragging a control point are all
// just Set calls, so they need no new mutation path.
func (h *Head) Set(leaf LeafID, v any) Epoch {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vals[leaf] = v
	h.epoch++
	// Persistence best-effort: a write failure must not lose the in-memory
	// edit, and the head is reconstructable from the source anyway.
	h.persistLocked()
	return h.epoch
}

// SetMany writes several leaves as ONE edit: it applies every value, bumps the
// epoch exactly once, and returns that single epoch. This is what makes a
// multi-leaf edit atomic — the values enter under one epoch, so a single wave
// computes over all of them at once and no subscriber ever observes an
// intermediate combination (three sliders moved together, not three separate
// waves). It stays within the single-chokepoint discipline: every write is still
// a head write; SetMany is Set for a set of leaves, not a second mutation path.
// An empty map still bumps the epoch (a no-op edit is a wave, matching Set).
func (h *Head) SetMany(vals map[LeafID]any) Epoch {
	h.mu.Lock()
	defer h.mu.Unlock()
	for leaf, v := range vals {
		h.vals[leaf] = v
	}
	h.epoch++
	h.persistLocked()
	return h.epoch
}

// Snapshot returns an immutable copy of the current leaf values together with
// the epoch at which it was taken. A wave reads only from this copy, never from
// the live map — the guarantee that makes propagation glitch-free.
func (h *Head) Snapshot() (map[LeafID]any, Epoch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make(map[LeafID]any, len(h.vals))
	for k, v := range h.vals {
		cp[k] = v
	}
	return cp, h.epoch
}

// Epoch returns the current epoch.
func (h *Head) Epoch() Epoch {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.epoch
}

// Get returns the current value of a leaf and whether it is set. It is a
// convenience for callers that need a single value (e.g. reconciling a saved
// selection); waves use Snapshot instead.
func (h *Head) Get(leaf LeafID) (any, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.vals[leaf]
	return v, ok
}

// persistLocked writes the head to disk. The caller must hold h.mu. It is a
// no-op for an in-memory head (empty path).
func (h *Head) persistLocked() {
	if h.path == "" {
		return
	}
	data, err := json.Marshal(h.vals)
	if err != nil {
		return // a value that doesn't marshal is not persisted; in-memory wins
	}
	_ = os.WriteFile(h.path, data, 0o600)
}
