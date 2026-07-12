package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// collectEvents subscribes and returns a function that drains and returns all
// events seen so far. Used to assert cell states within a wave.
func collectEvents(rt *Runtime) (drain func() []Event) {
	sub := rt.Subscribe()
	var mu sync.Mutex
	var events []Event
	go func() {
		for ev := range sub {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}()
	return func() []Event {
		time.Sleep(20 * time.Millisecond) // let events drain
		mu.Lock()
		defer mu.Unlock()
		cp := make([]Event, len(events))
		copy(cp, events)
		return cp
	}
}

// lastState returns the final state reported for a cell.
func lastState(events []Event, id CellID) (State, bool) {
	var st State
	var found bool
	for _, ev := range events {
		if ev.Cell == id {
			st = ev.State
			found = true
		}
	}
	return st, found
}

// lastEvent returns the final event reported for a cell.
func lastEvent(events []Event, id CellID) (Event, bool) {
	var last Event
	var found bool
	for _, ev := range events {
		if ev.Cell == id {
			last = ev
			found = true
		}
	}
	return last, found
}

// TestErrorBlocksDownstream: a cell that returns an error blocks its downstream,
// which report StateBlocked rather than being fed a wrong value. This is the
// runtime tier of the same two-tier model the toolchain uses (a broken cell
// doesn't stop the unrelated cells; it blocks only what depends on it).
func TestErrorBlocksDownstream(t *testing.T) {
	// a (leaf x) -> bad -> down;  also  a -> indep (independent of bad).
	bad := fnNode{
		id: "bad", in: []Symbol{"x"}, out: []Symbol{"b"},
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			return nil, errors.New("boom")
		},
	}
	down := fnNode{
		id: "down", in: []Symbol{"b"}, out: []Symbol{"d"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"d": in["b"]}, nil
		},
	}
	indep := fnNode{
		id: "indep", in: []Symbol{"x"}, out: []Symbol{"i"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"i": in["x"].(int) + 1}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{bad, down, indep},
		Leaves: []LeafID{"x"},
		Levels: [][]CellID{{"bad", "indep"}, {"down"}},
	}
	head := NewHead()
	rt := NewRuntime(cfg, head, NewMemoStore())
	drain := collectEvents(rt)

	rt.Set(context.Background(), "x", 1)
	events := drain()

	if st, _ := lastState(events, "bad"); st != StateError {
		t.Errorf("bad should be StateError, got %v", st)
	}
	if st, _ := lastState(events, "down"); st != StateBlocked {
		t.Errorf("down depends on bad; should be StateBlocked, got %v", st)
	}
	// The independent cell still runs to completion — a broken cell does not
	// stop unrelated work.
	if st, _ := lastState(events, "indep"); st != StateDone {
		t.Errorf("indep is independent of bad; should be StateDone, got %v", st)
	}
}

// TestPanicRecoveredAsError: a cell that panics becomes a typed error on that
// node (never process death), and blocks its downstream like any other error.
func TestPanicRecoveredAsError(t *testing.T) {
	boom := fnNode{
		id: "boom", in: []Symbol{"x"}, out: []Symbol{"b"},
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			panic("kaboom")
		},
	}
	down := fnNode{
		id: "down", in: []Symbol{"b"}, out: []Symbol{"d"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"d": in["b"]}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{boom, down},
		Leaves: []LeafID{"x"},
		Levels: [][]CellID{{"boom"}, {"down"}},
	}
	rt := NewRuntime(cfg, NewHead(), NewMemoStore())
	drain := collectEvents(rt)

	rt.Set(context.Background(), "x", 1) // must not crash the process
	events := drain()

	st, found := lastState(events, "boom")
	if !found || st != StateError {
		t.Errorf("panicking cell should be StateError, got %v (found=%v)", st, found)
	}
	if st, _ := lastState(events, "down"); st != StateBlocked {
		t.Errorf("downstream of a panic should be StateBlocked, got %v", st)
	}
}
