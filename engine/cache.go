package engine

import (
	"reflect"
	"sync"
)

// Key identifies a cached cell result by the cell and the versions of its
// inputs. Keying on versions rather than value hashes means arbitrary Go values
// never have to be hashed: same cell, same input versions ⇒ same output.
type Key struct {
	Cell CellID
	Vers string // input versions, order-stable, joined into a comparable key
}

// Store is the cache behind an interface from day one, so eviction (which
// becomes mandatory once folds generate unbounded tick keys) is an additive
// change: swap the implementation, leave the scheduler untouched.
type Store interface {
	Get(key Key) (Outputs, bool)
	Put(key Key, out Outputs)
}

// memoStore is the unbounded in-memory implementation. It is correct for
// stateless notebooks; a bounded/evicting Store replaces it when folds arrive.
type memoStore struct {
	mu sync.Mutex
	m  map[Key]Outputs
}

// NewMemoStore returns an unbounded in-memory Store.
func NewMemoStore() Store {
	return &memoStore{m: make(map[Key]Outputs)}
}

func (s *memoStore) Get(key Key) (Outputs, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, ok := s.m[key]
	return out, ok
}

func (s *memoStore) Put(key Key, out Outputs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = out
}

// changed reports whether a recomputed value differs from the old one, so a
// recompute that produces an identical value does not wake the subtree.
//
// It uses the same structural ladder as the capability probes: an Equal(any)
// method if present, else == if the type is comparable, else conservatively
// assume it changed. Being conservative here only costs extra recomputation,
// never correctness.
func changed(old, newv any) bool {
	if e, ok := newv.(interface{ Equal(any) bool }); ok {
		return !e.Equal(old)
	}
	if t := reflect.TypeOf(newv); t != nil && t.Comparable() {
		return old != newv
	}
	return true
}
