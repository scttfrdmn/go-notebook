// Package engine is the notebook runtime. Generated code imports it, so its
// API is public and versioned from the first commit.
//
// This milestone (M2) defines only the surface the generated registry needs to
// reference and compile against: the [Node] execution interface, the value
// types that flow across edges, and the per-cell metadata descriptor. The
// scheduler, head, cache, and event stream arrive in M3 — but the types here
// are the stable contract those layers and the generated code both depend on.
//
// Hard constraint: engine must never import net/http (nor any transport). It
// will emit events on a channel that engine/server subscribes to, which is what
// keeps headless, WASM, and batch modes free. Nothing in this package reaches
// for the network or a server.
package engine

import "context"

// CellID identifies a cell — the cell's function name, unique within a
// notebook.
type CellID string

// Symbol is a named result or parameter: the unit of dataflow. A leaf is
// identified by the symbol it produces.
type Symbol string

// LeafID identifies an input leaf by the symbol it produces. A leaf is a cell
// whose value the user (or, later, a timer or grip) writes directly.
type LeafID = Symbol

// Epoch counts edits. Each write to the head bumps the epoch; a wave carries
// its epoch so superseded results can be discarded before they commit.
type Epoch uint64

// Inputs maps each of a cell's parameter symbols to its current value.
type Inputs map[Symbol]any

// Outputs maps each of a cell's result symbols to the value it produced.
type Outputs map[Symbol]any

// Value is a symbol's current value plus a version. The cache keys on versions,
// so arbitrary Go values never have to be hashed: two runs with the same input
// versions produce the same output.
type Value struct {
	V       any
	Version uint64
}

// Node is the unit of execution. Generated cells are one implementation; an
// interpreted or remote executor can be another without the scheduler knowing.
// Keeping this an interface (not a struct) is the seam that lets alternate
// executors exist later.
type Node interface {
	// ID returns the cell's identifier.
	ID() CellID
	// In returns the input symbols the cell consumes (wired parameters only;
	// injected and delayed parameters are supplied by the runtime).
	In() []Symbol
	// Out returns the result symbols the cell produces.
	Out() []Symbol
	// Pure reports whether the cell is safe to cache: false if it transitively
	// touches time, randomness, or I/O. Derived by the toolchain, never
	// declared. A conservative false only costs a cache miss.
	Pure() bool
	// Run executes the cell against its inputs. The context is honored by cells
	// that ask for one; a panic inside Run is recovered by the scheduler (M3)
	// into a per-cell error state, so implementations need not.
	Run(ctx context.Context, in Inputs) (Outputs, error)
}

// CellMeta is the presentation metadata for a cell: everything the view needs
// that is not part of execution. It is flattened by codegen from doc comments
// and //notebook: directives, so nothing is parsed at runtime.
type CellMeta struct {
	// ID is the cell this metadata describes.
	ID CellID
	// Leaf is the symbol this cell produces when it is an input control (a leaf
	// the user edits). Empty for non-leaf cells. The head, the UI, and --set
	// all address a leaf by this symbol — a leaf is identified by the symbol it
	// produces, not by the cell's name.
	Leaf Symbol
	// Label is the cell's display label (the first sentence of its doc
	// comment, or its function name).
	Label string
	// Directives are the flattened //notebook:k=v pairs. A bare directive
	// token (e.g. "slider") is recorded with an empty value.
	Directives map[string]string
}
