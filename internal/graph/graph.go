// Package graph is the notebook intermediate representation: a plain-data
// dependency graph derived from a Go package.
//
// The IR is deliberately free of any dependency on go/types (or any analysis
// machinery). A [Graph] is a value that can be constructed, serialized,
// checked, and planned over without loading a compiler. This is the seam that
// lets the source analyzer be swapped later (go/types now, a headless gopls
// later) without touching the graph algorithms or the engine that consumes
// them.
//
// The wiring rule the IR encodes, in one sentence: a cell's named result feeds
// any cell that takes a parameter of the same name and type.
package graph

// CellID identifies a cell. It is the cell's function name, which is unique
// within a notebook file.
type CellID string

// Symbol is a named result or parameter — the unit of dataflow. An edge exists
// between two cells when one produces a Symbol another consumes.
type Symbol string

// ParamKind classifies how a parameter is satisfied.
type ParamKind int

const (
	// Wired is an ordinary edge: the parameter matches a Symbol produced by
	// some other cell's result.
	Wired ParamKind = iota

	// Injected marks a context.Context parameter. It is supplied by the
	// runtime, not by an upstream cell, and so is never an edge.
	Injected

	// Delayed marks a Prev[T] parameter: a self-edge read from the PREVIOUS
	// epoch rather than the current one.
	//
	// Nothing in this milestone produces a Delayed parameter — folds are
	// deferred. The kind exists now so that every graph algorithm (cycle
	// detection, topological ordering, the dirty closure) already knows to
	// skip delayed edges. Retrofitting that later would touch every algorithm;
	// establishing it now costs a handful of lines.
	Delayed
)

// String renders a ParamKind for diagnostics and golden output.
func (k ParamKind) String() string {
	switch k {
	case Wired:
		return "wired"
	case Injected:
		return "injected"
	case Delayed:
		return "delayed"
	default:
		return "unknown"
	}
}

// Position is a source location. It mirrors the fields of token.Position that
// diagnostics need, without importing go/token — the IR stays plain data.
type Position struct {
	Filename string `json:"filename"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

// Param is a single cell parameter.
type Param struct {
	Name Symbol    `json:"name"`
	Type string    `json:"type"` // rendered type string, for diagnostics and codegen
	Kind ParamKind `json:"kind"`
	Pos  Position  `json:"pos"`
}

// Result is a single named cell result.
type Result struct {
	Name Symbol `json:"name"`
	Type string `json:"type"`
	// Underlying is the result's underlying basic kind ("int", "float64",
	// "bool", "string", …) when it is a basic-kinded type — even through a named
	// type like PerHour over float64 — or "" for composite/interface types. It
	// is filled by the analyzer from go/types, so codegen never has to guess an
	// underlying kind from the rendered type string.
	Underlying string `json:"underlying,omitempty"`
	// IsError marks a trailing error result. It is not an edge: it is the
	// failure channel that blocks downstream cells rather than feeding them.
	IsError bool     `json:"isError"`
	Pos     Position `json:"pos"`
}

// Cell is one notebook cell: a top-level documented function.
//
// A cell may have multiple named results. Nothing in the IR or its algorithms
// assumes a single result per cell — multi-output cells (seamOrder, sim) are a
// first-class case, not a later addition.
type Cell struct {
	ID    CellID   `json:"id"`
	Pos   Position `json:"pos"`
	Doc   string   `json:"doc"`   // full doc comment text
	Label string   `json:"label"` // first sentence of Doc, or the function name
	// Source is the cell's verbatim source — its doc comment through the closing
	// brace — so the view can show "a cell is a function," read-only. It is
	// derived from the file the cell was parsed from; presentation only, never
	// parsed or executed.
	Source     string            `json:"source,omitempty"`
	Directives map[string]string `json:"directives,omitempty"`
	Params     []Param           `json:"params"`
	Results    []Result          `json:"results"`
	// Pure is derived (never declared): false if the cell transitively touches
	// time, randomness, or I/O. Impure cells are never cached.
	Pure bool `json:"pure"`
	// IsLeaf marks an editable input: a cell whose value the user (or a timer,
	// or a grip) writes directly. Leaf-ness is decided by the TYPE, never by a
	// directive — a comment must not decide whether something is an input. A
	// cell is a leaf iff it produces a single value that is widget-capable
	// (Bounds/Options/Reconcile) or, when parameterless, a scalar basic kind.
	// The directive only refines how the control looks, never whether it is one.
	IsLeaf bool `json:"isLeaf"`
	// WASMable reports whether the cell can run under GOOS=js GOARCH=wasm — it
	// transitively touches no net/os/cgo. Distinct from Pure (time and rand are
	// impure but WASM-able); derived by [analyze.WASMability], never declared.
	WASMable bool `json:"wasmable"`
}

// Graph is the whole notebook: its cells, the producer index that realizes the
// wiring rule, and source order for default layout.
type Graph struct {
	Cells    map[CellID]*Cell  `json:"cells"`
	Producer map[Symbol]CellID `json:"producer"` // exactly one producer per Symbol (enforced by Check)
	Order    []CellID          `json:"order"`    // source order, for default layout
	// Helpers are top-level funcs in the notebook file that are NOT cells
	// because they name no result (the wiring rule: no named result → no edge
	// → not a cell). Recorded, in source order, so tooling can show them: a
	// cell that vanishes because the author forgot to name its result is then a
	// one-glance diagnosis rather than a mystery.
	Helpers []CellID `json:"helpers,omitempty"`
}

// New returns an empty Graph with initialized maps.
func New() *Graph {
	return &Graph{
		Cells:    make(map[CellID]*Cell),
		Producer: make(map[Symbol]CellID),
		Order:    nil,
	}
}

// Add appends a cell to the graph in source order. It does not populate the
// producer index or validate anything — call [Graph.Index] and [Graph.Check]
// for that. Add is the low-level building block used by the analyzer and by
// tests that construct graphs by hand.
func (g *Graph) Add(c *Cell) {
	g.Cells[c.ID] = c
	g.Order = append(g.Order, c.ID)
}

// wiredParams returns the cell's parameters that are ordinary edges — i.e.
// neither Injected (context) nor Delayed (a previous-epoch self-edge). These
// are the parameters that must have a producer and that form the forward
// dependency edges the graph algorithms walk.
func (c *Cell) wiredParams() []Param {
	var out []Param
	for _, p := range c.Params {
		if p.Kind == Wired {
			out = append(out, p)
		}
	}
	return out
}

// dataResults returns the cell's results that are edges — every named result
// except a trailing error (the failure channel) and any unnamed result (which
// names no dataflow symbol and so cannot be wired).
func (c *Cell) dataResults() []Result {
	var out []Result
	for _, r := range c.Results {
		if r.IsError || r.Name == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}
