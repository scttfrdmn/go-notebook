package engine

import (
	"path/filepath"
	"testing"
)

// TestHeadSetBumpsEpoch confirms every Set advances the epoch — the signal a
// wave is tagged with.
func TestHeadSetBumpsEpoch(t *testing.T) {
	h := NewHead()
	if h.Epoch() != 0 {
		t.Fatalf("fresh head epoch = %d, want 0", h.Epoch())
	}
	e1 := h.Set("x", 1)
	e2 := h.Set("x", 2)
	if e1 != 1 || e2 != 2 {
		t.Errorf("Set epochs = %d, %d; want 1, 2", e1, e2)
	}
}

// TestSnapshotIsIsolated confirms a snapshot is an independent copy: mutating
// the head after snapshotting does not change the snapshot. This is the
// property the scheduler relies on for glitch-freedom.
func TestSnapshotIsIsolated(t *testing.T) {
	h := NewHead()
	h.Set("x", 1)
	snap, _ := h.Snapshot()
	h.Set("x", 2)
	if snap["x"] != 1 {
		t.Errorf("snapshot changed after a later Set: got %v, want 1", snap["x"])
	}
}

// TestHeadPersistRestore confirms leaf values survive a "restart" — reopening
// the head from disk restores the values. This is what makes process restart a
// non-event: the only state is a few leaf values.
func TestHeadPersistRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "head.json")

	h1, err := OpenHead(path)
	if err != nil {
		t.Fatal(err)
	}
	h1.Set("servers", 120)
	h1.Set("rate", 1400)

	// Reopen — simulating a process restart.
	h2, err := OpenHead(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := h2.Get("servers"); !ok || int(v.(float64)) != 120 {
		t.Errorf("restored servers = %v (ok=%v), want 120", v, ok)
	}
	if v, ok := h2.Get("rate"); !ok || int(v.(float64)) != 1400 {
		t.Errorf("restored rate = %v (ok=%v), want 1400", v, ok)
	}
}

// TestOpenHeadMissingFile confirms a missing file yields an empty head, not an
// error — a fresh notebook has no persisted state yet.
func TestOpenHeadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	h, err := OpenHead(path)
	if err != nil {
		t.Fatalf("OpenHead on missing file should not error: %v", err)
	}
	if h.Epoch() != 0 {
		t.Errorf("fresh head epoch = %d, want 0", h.Epoch())
	}
}
