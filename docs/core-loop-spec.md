# Core Loop — Engineering Spec

*A buildable first milestone for the Go reactive notebook. Written to be handed to Claude Code.*

**Companion document:** `reactive-notebook-go.md` (the design). This spec assumes it.

---

## 0. What this milestone is for

The core loop exists to answer **one question**, fast:

> Is `edit a cell → see the chart move` pleasant in a compile-first notebook?

Everything in the design is downstream of that. If the edit loop is two seconds, the project is dead or needs `gopls` on day one — and it is far better to learn that in a weekend than after building widgets, folds, and a SQL typechecker on top of a loop that doesn't feel right.

So this milestone is deliberately narrow **in features** and deliberately complete **in structure**. The features it skips are cheap to add later *only if* the seams are cut now. Section 3 is the important one: it lists the abstractions that must exist from the first commit even where their only implementation is trivial, because retrofitting them is a rewrite.

**Success is a falsification, not a demo.** See the kill criteria in §7.

---

## 1. Scope

### In

- Derive the dependency graph from `go/types` (cells, edges, cycles, diagnostics)
- Generate a cell registry via `go build -overlay` (**never writes to the user's source tree**)
- Epoch'd scheduler: immutable head snapshot per wave, topological execution, goroutine fan-out on independent branches, supersede-on-new-epoch
- Head: `map[LeafID]any`, persisted to disk, restored on start
- One widget capability (`Bounded` → slider) and one output capability (`Render() Rendered`)
- HTTP + WebSocket server; browser renders SVG/markdown blobs and posts leaf edits
- `go tool notebook run <dir>` and `go tool notebook check <dir>`
- `examples/capacity` runs end to end

### Explicitly out — but not foreclosed

`Prev[T]` / `Tick` · timers and buttons · grips and direct manipulation · `Multi` / `Select` / `Table` / `Draggable` · SQL and `Rel[T]` · cache eviction · hot reload · WASM · remote build service · giri integration · per-cell diagnostics remapping for a browser IDE.

Each of these has a named seam in §3. **Do not implement them. Do not make them impossible.**

### Non-negotiable from commit one

- **Epoch'd snapshot propagation.** Glitch-freedom cannot be retrofitted; it is a scheduler rewrite.
- **Multi-output cells.** The design uses them (`seamOrder`, `sim`). Assuming one result per cell is a data-model rewrite.
- **`Delayed` as an edge kind.** Five lines now; touches every graph algorithm later.

---

## 1.5 Naming and distribution

**The project has no brand, and that is deliberate.**

| Thing | Name |
|---|---|
| File convention | `//go:notebook` |
| Cell directives | `//notebook:slider min=0 max=100` |
| Module | `github.com/scttfrdmn/go-notebook` |
| Command | `go tool notebook` (binary: `cmd/notebook`) |
| Runtime, imported only by generated code | `go-notebook/engine` |
| Optional author conveniences (`Unit`, `Percent`, …) | root package `notebook` |

The directive names *what the file is*, not what reads it. A `//go:notebook` file is a notebook whether this toolchain runs it, a future `gopls` understands it, or someone writes a competing runtime. It sits in the same register as `//go:generate` and `//go:embed`: a convention any tool may honor, not a vendor tag.

The consequence worth protecting: **a notebook file contains no mention of this project anywhere.** No import, no marker, no brand. That is the strongest form of the no-framework claim — the artifact does not know the framework exists. Do not add one.

### `go tool`, not `go notebook`

The Go toolchain has a fixed subcommand set and deliberately no `git`-style PATH dispatch, so `go notebook` is not available and never will be. Go 1.24's `tool` directives in `go.mod` are the supported path:

```
go get -tool github.com/scttfrdmn/go-notebook/cmd/notebook
go tool notebook run capacity.go
```

This is better than a standalone binary, not a consolation prize: the toolchain that reads a notebook becomes a **version-pinned dependency of the module the notebook lives in**, recorded in `go.mod` alongside everything else. Reproducibility again, as a side effect of a decision made for other reasons.

Zero-install trial still works:

```
go run github.com/scttfrdmn/go-notebook/cmd/notebook@latest run capacity.go
```

`cmd/notebook` must therefore be `go run`-able with no configuration and no state directory.

---

## 2. Repository layout

```
<module>/
  go.mod                        # go 1.24
  cmd/notebook/                      # CLI: run, check, build
    main.go
  internal/analyze/             # source → Graph. Swappable (go/types now, gopls later).
    analyzer.go                 # Analyzer interface
    types.go                    # TypesAnalyzer (the only impl for now)
    doc.go                      # doc comment → label; //notebook: → directives
    purity.go                   # callgraph → Pure flag
  internal/graph/               # the IR. No I/O, no go/types, fully unit-testable.
    graph.go                    # Graph, Cell, Param, Result, Symbol
    check.go                    # cycles (skipping Delayed), single-producer, arity
    plan.go                     # dirty closure, topological levels
  internal/gen/                 # Graph → registry source + overlay
    gen.go
    overlay.go                  # go build -overlay JSON
    posmap.go                   # generated line → (cell, offset). Carried, barely used yet.
  engine/                       # PUBLIC. Generated code imports this. Stable API.
    engine.go                   # Runtime, Node, Value, Inputs/Outputs
    head.go                     # leaf state, persistence, Set() chokepoint
    schedule.go                 # epochs, waves, fan-out, supersede
    cache.go                    # Store interface + memo impl
    widget.go                   # capability probes, Widget descriptor, Reconciler
    render.go                   # Renderable probe, Rendered
    event.go                    # Event stream out of the engine
  engine/server/                # PUBLIC. HTTP/WS. engine MUST NOT import this.
    server.go
    ui/                         # embedded static assets (embed.FS)
  notebook.go                   # PUBLIC root pkg. Optional author conveniences: Unit, Percent, …
  examples/capacity/            # the reference notebook (capacity.go, verbatim from design)
  testdata/
    graphs/                     # notebook .go → expected graph JSON (golden)
    cycles/                     # notebooks that must fail, with expected diagnostics
```

**Module boundaries that matter:**

- `internal/graph` has **no dependency on `go/types`**. The IR is plain data. This is what lets the gopls swap happen without touching the scheduler, and it is what makes the graph algorithms trivially testable.
- `engine` has **no dependency on `net/http`**. It emits `Event`s on a channel; `engine/server` subscribes. This is what lets headless, WASM, and batch modes exist later without an HTTP server in the binary.
- `engine` is **public** because generated code imports it. Treat its API as versioned from day one.

---

## 3. Foreclosure table

**This is the load-bearing section.** Each row is a feature deferred from this milestone, and the seam that must exist now so adding it later is additive rather than structural.

| Deferred | Seam required now | Cost now |
|---|---|---|
| `Prev[T]` folds | `ParamKind` enum with `Delayed` member; cycle checker **skips** `Delayed` edges; topo sort ignores them | ~5 lines |
| Timers, buttons | All leaf writes funnel through **one** `Head.Set(leaf, value)` that bumps the epoch. A timer is a goroutine calling `Set`. A button is a `Set` on a counter. | free (just don't scatter writes) |
| Grips / direct manipulation | `Rendered` carries a `Grips []Handle` field (empty for now); a leaf-write endpoint exists on the server | ~10 lines |
| `Multi`, `Select`, `Table`, `Draggable` | Widget discovery is **capability probing**, not a type switch. `Reconciler` interface exists with `Range` as sole impl. | ~30 lines |
| Cells that *return* widgets (data-derived bounds) | A cell is a **leaf iff its output implements a widget capability** — regardless of whether it has parameters. Run the cell for the schema, then `Reconcile` the head's saved value into it. | ~30 lines |
| SQL / `Rel[T]` | Nothing structural. `Node.Run` already takes `any` values; a handle is just a value. | free |
| Cache eviction | `cache.Store` is an **interface**; the memo impl is behind it. | ~15 lines |
| Alternate executors (interpreted, remote) | `Node` is an **interface**, not a struct. Generated cells are one impl. | ~10 lines |
| gopls / incremental analysis | `Analyzer` is an **interface** returning `*graph.Graph` + diagnostics | ~10 lines |
| Hot reload | Head is **persisted and reloadable**; process restart is a non-event | already required |
| WASM, headless, batch | `engine` never imports `net/http`; transport subscribes to an event channel | free (discipline) |
| Browser IDE diagnostics | `gen` emits a **position map** (generated line → cell + offset), unused for now | ~20 lines |

**Anti-goals for this milestone.** Do not add: a plugin system, a config file format, an abstraction over SVG, a component model, a DSL, or a second way to declare a cell. If a feature seems to need one of those, it is out of scope.

---

## 4. Core types

### 4.1 The IR (`internal/graph`)

```go
package graph

type (
    CellID string
    Symbol string   // a named result / parameter — the unit of dataflow
)

type ParamKind int

const (
    Wired    ParamKind = iota // ordinary edge: matches a Symbol produced elsewhere
    Injected                  // context.Context — supplied by the runtime, not an edge
    Delayed                   // Prev[T] — a self-edge read from the PREVIOUS epoch.
                              // NOT PRODUCED YET. Exists so cycle-check and topo-sort
                              // already know to skip it.
)

type Param struct {
    Name Symbol
    Type string    // rendered type string, for diagnostics and codegen
    Kind ParamKind
}

type Result struct {
    Name    Symbol
    Type    string
    IsError bool   // trailing error: not an edge; failure channel
}

type Cell struct {
    ID         CellID
    Pos        Position
    Doc        string            // full doc comment
    Label      string            // first sentence of Doc, or the function name
    Directives map[string]string // //notebook:k=v
    Params     []Param
    Results    []Result
    Pure       bool              // derived: false if it transitively touches time/rand/IO
}

type Graph struct {
    Cells    map[CellID]*Cell
    Producer map[Symbol]CellID // exactly one producer per Symbol (enforced)
    Order    []CellID          // source order, for default layout
}
```

Rules enforced by `graph.Check`:

1. **Single producer.** Two cells naming the same result symbol is an error, with both positions.
2. **Every `Wired` param has a producer.** Otherwise: "no cell produces `lambda PerHour`" at the param's position.
3. **Type agreement.** A `Wired` param's type must equal its producer's result type. (`go/types` gives this for free; report it as a graph diagnostic anyway, because the message is much better.)
4. **No cycles among non-`Delayed` edges.** Report the cycle as a path.
5. **`Delayed` params require a `Tick`-typed param on the same cell.** *(Deferred — write the check, return nil, leave a `// TODO(prev)` so it lands in one place.)*

`graph.Plan`:

```go
// Dirty returns the transitive downstream closure of the changed symbols.
func (g *Graph) Dirty(changed []Symbol) map[CellID]bool

// Levels returns cells in topological levels; cells within a level are independent
// and MUST be run concurrently.
func (g *Graph) Levels(dirty map[CellID]bool) [][]CellID
```

`Levels` returning *levels* rather than a flat order is deliberate: it makes the parallelism structural rather than an optimization someone might forget to apply.

### 4.2 The engine (`engine`)

```go
package engine

type (
    CellID string
    Symbol string
    LeafID = Symbol // a leaf is identified by the symbol it produces
    Epoch  uint64
)

type Inputs map[Symbol]any
type Outputs map[Symbol]any

// Node is the unit of execution. Generated cells are one implementation; an
// interpreted or remote executor can be another WITHOUT the scheduler knowing.
type Node interface {
    ID() CellID
    In() []Symbol
    Out() []Symbol
    Pure() bool
    Run(ctx context.Context, in Inputs) (Outputs, error)
}

// Value is a symbol's current value plus a version. The cache keys on versions,
// so we never hash arbitrary Go values.
type Value struct {
    V       any
    Version uint64
}
```

**Head — the only mutable state in the system:**

```go
type Head struct {
    mu    sync.Mutex
    vals  map[LeafID]any
    path  string        // persisted here
    epoch Epoch
    bus   chan<- Edit
}

// Set is the ONE place a leaf is written. Sliders call it. Later: timers, buttons,
// grips. Keeping this a single chokepoint is what makes those additive.
func (h *Head) Set(leaf LeafID, v any) Epoch

// Snapshot returns an immutable copy. A wave reads ONLY from a snapshot — this is
// what makes propagation glitch-free, and it cannot be retrofitted.
func (h *Head) Snapshot() map[LeafID]any
```

**Scheduler:**

```go
type wave struct {
    epoch  Epoch
    snap   map[LeafID]any      // immutable
    ctx    context.Context
    cancel context.CancelFunc
}

// Run executes the dirty subgraph for one wave.
//
//  - cells within a level fan out onto goroutines
//  - each cell's result is epoch-checked before commit; a superseded wave's output
//    is discarded (this is drag-coalescing, and it is free)
//  - a cell in error blocks its downstream: they emit StateBlocked, not a wrong value
//  - panics are recovered per node into an error state
func (r *Runtime) run(w *wave, dirty map[CellID]bool) error
```

**Cache — behind an interface from day one:**

```go
type Store interface {
    Get(key Key) (Outputs, bool)
    Put(key Key, out Outputs)
}

// Key is (cellID, input versions). No value hashing.
type Key struct {
    Cell CellID
    Vers []uint64
}
```

Impure nodes (`Pure() == false`) **skip the cache entirely**. Purity is derived by the toolchain (§5.3), never declared.

**Propagation pruning** — a recompute producing an identical value must not wake the subtree:

```go
func changed(old, new any) bool {
    if e, ok := new.(interface{ Equal(any) bool }); ok {
        return !e.Equal(old)
    }
    if reflect.TypeOf(new) != nil && reflect.TypeOf(new).Comparable() {
        return old != new
    }
    return true // conservative
}
```

Same structural-probe pattern as everything else.

### 4.3 Capabilities

```go
package engine

// --- inputs ---

type Bounded interface{ Bounds() (lo, hi float64) }

// Optioned is NOT used this milestone. Declared so the probe list is a list.
type Optioned interface{ Options() []string }

// Reconciler merges a saved selection into a freshly computed schema.
// Range clamps. (Multi will filter; Draggable will reset. Per-kind, not universal.)
type Reconciler interface {
    Reconcile(saved any) any
}

// --- outputs ---

type Rendered struct {
    MIME  string
    Data  string
    Grips []Handle // always empty this milestone; the field exists so grips are additive
}

type Renderable interface{ Render() Rendered }
```

**Widget discovery is probing, never a type switch.** Adding `Multi` later means adding one probe, not editing a switch statement in four places.

**A cell is a leaf iff its output value implements a widget capability.** This holds *regardless of whether the cell has parameters* — which is exactly what makes data-derived bounds (`priceRange(rows)`) additive rather than a redesign. Implement the general rule now even though only parameterless `Bounded` roots occur in `capacity.go`.

Leaf evaluation order, every wave:

1. Run the cell → the **schema** (fresh bounds / options / defaults)
2. If the head has a saved value for this leaf → `Reconcile(saved)`
3. The reconciled value is what flows downstream

### 4.4 Events

```go
type State int

const (
    StateRunning State = iota
    StateDone
    StateError
    StateBlocked   // an upstream cell failed
    StateStale     // superseded by a newer epoch
)

type Event struct {
    Epoch Epoch
    Cell  CellID
    State State
    Out   *Rendered // nil unless the cell's output is Renderable
    Err   string
}

func (r *Runtime) Subscribe() <-chan Event
```

`engine` does not import `net/http`. `engine/server` subscribes and pushes over WS. This is what keeps headless, WASM, and batch modes free later.

---

## 5. The toolchain

### 5.1 Analysis

```go
package analyze

type Analyzer interface {
    Analyze(dir string) (*graph.Graph, []Diagnostic, error)
}

type TypesAnalyzer struct{} // go/packages + go/types. The only impl for now.
```

Procedure:

1. **Two-tier load.** Graph derivation loads with `NeedName|NeedFiles|NeedSyntax|NeedTypes|NeedTypesInfo|NeedImports` — **no `NeedDeps`**. Dependency types (`context.Context` and domain types) resolve from export data via `NeedImports`; `NeedDeps` loads full dependency *source* and is ~4× more expensive. It is required only by purity's call graph (§5.3), which is a separate pass off the interactive path. *(Measured: dropping `NeedDeps` took cold derivation from 625ms to 86ms — see #16.)*
2. Find the file carrying `//go:notebook`.
3. **A cell is a non-generic, non-method, top-level func with ≥1 named non-error result.** This is the wiring rule doing double duty: the named result *is* the edge, so a func that names no result produces no edge and cannot be a cell — it is an ordinary helper, **regardless of whether it has a doc comment**. The doc comment is the *label*, not the marker. Two explicit exclusions:
   - **Generic funcs are never cells** — a func with type parameters has no concrete result type to wire. (Not optional; it is a hole otherwise.)
   - **Methods are never cells.**

   `check` prints the **helper list** (top-level funcs that name no result), so a cell that silently vanishes because the author forgot to name its result is a one-glance diagnosis. The reverse — a helper that names its results and gets promoted to a cell — fails loudly with a "no cell produces `x`" diagnostic rather than corrupting silently.
4. A cell that names *some* results but leaves a non-error result unnamed (or `_`) is a diagnostic: *"cell results must be named; the name is the edge."* (An all-unnamed func is simply a helper, not an error.)
5. Params: `context.Context` → `Injected`. `Prev[T]` → `Delayed` *(cannot occur yet; write the branch, return a "not supported in this milestone" diagnostic)*. Everything else → `Wired`.
6. Build `Producer` from result names; run `graph.Check`.

**Interactive re-analysis (KC2).** The dependency graph does not change on a one-cell edit — only the notebook's own file contents do. So the incremental path (`analyze.Session`) primes the dependency importer once and thereafter re-typechecks *only* the notebook package against that cached importer — no `packages.Load`, no `go list`. *(Measured: 0.48ms, vs. the ~86ms cold load. gopls stays a "later" item unless a warm re-typecheck ever exceeds ~100ms.)*

Diagnostics carry `token.Position` and must be **actionable**. The message for a missing producer is the difference between this feeling like a tool and feeling like a toy:

```
capacity.go:31:19: cell "utilization" needs `a Erlangs`, but no cell produces it.
                   Did you mean `offeredLoad`, which produces `a Erlangs`? (capacity.go:26)
```

### 5.2 Codegen and the overlay

**The generated registry never touches the user's source tree.** Use `go build -overlay`:

- Synthesize `notebook_gen.go` **in the notebook's package** (it must see unexported cells) and a tiny `main` package that imports it.
- Write both to a temp build dir.
- Emit an overlay JSON mapping virtual paths → temp files.
- `go build -overlay=overlay.json`.

The user's directory stays clean, `go tool notebook check` is non-invasive, and there is no `.gitignore` etiquette to explain.

```go
// notebook_gen.go  (synthesized; package capacity)
var Cells = []engine.Node{
    genCell{
        id:   "utilization",
        in:   []engine.Symbol{"a", "c"},
        out:  []engine.Symbol{"rho"},
        pure: true,
        fn: func(ctx context.Context, in engine.Inputs) (engine.Outputs, error) {
            rho := utilization(in["a"].(Erlangs), in["c"].(int))
            return engine.Outputs{"rho": rho}, nil
        },
    },
    // ...
}

var Meta = []engine.CellMeta{
    {ID: "servers", Label: "Servers in the fleet.", Directives: map[string]string{
        "slider": "", "min": "1", "max": "256",
    }},
    // ...
}
```

The type assertions are **safe by construction** — codegen knew the static types.

**A significant free win:** because cells are ordinary functions in an ordinary package, `go build` errors *inside cell bodies* already point at the user's real file and line. Only the synthesized registry needs remapping, and it is machine-generated and should never fail. Emit the position map anyway (`internal/gen/posmap.go`); it costs twenty lines and the browser-IDE case will want it.

### 5.3 Purity

`Pure` is **derived, never declared.** Build a call graph and mark a cell impure if it transitively reaches: `time.Now`, `time.Since`, `math/rand`, `crypto/rand`, `os`, `net`, `net/http`, or any cgo boundary.

**Purity is off the interactive path, and that is architecture, not optimization.** Purity is consumed by exactly one thing — the cache — and a conservative verdict is *always safe*: marking a pure cell impure only costs a cache hit, while marking an impure cell pure gives wrong answers. Therefore:

- **Cells default to impure.** The graph derivation (§5.1) sets `Pure = false` and does not compute purity at all. A separate `RefinePurity` pass upgrades cells to pure at build time or in the background — **never blocking a keystroke**.
- **Purity, not graph derivation, is what needs `NeedDeps`** (the call graph requires full dependency source). Keeping it a separate pass is exactly what lets the interactive load drop `NeedDeps`.
- **CHA, not VTA.** Purity needs only a *sound over-approximation* of "does this reach an impure primitive," and `callgraph/cha` provides that at a fraction of VTA's cost. CHA over-approximates interface dispatch, so it may occasionally mark a pure cell impure — the safe direction, costing a cache hit and nothing else. *(Earlier drafts reached for VTA to avoid `fmt`-using render cells being flagged impure via `os`; with default-impure semantics that precision buys nothing, so CHA wins.)*

> This is the correction from the design doc. A `//notebook:nocache` directive is *wrong* — a comment that changes whether the answer is correct violates the type-vs-comment rule. The toolchain already knows.

### 5.4 Never re-derive from text what the type checker already knows

**The load-bearing principle, stated once because it has already been violated three times.** When a structural fact is decidable from `go/types`, decide it there and carry the answer in the IR — never reach for the cheaper textual proxy (a comment, or the rendered type *string*). The type checker is holding the answer; asking a string to impersonate it is how silent lies get in.

Three instances, all the same error:

- **`//notebook:nocache`** (rejected in §5.3): a *comment* deciding whether a result is correct. Purity is a call-graph fact.
- **A directive deciding leaf-ness** (regression, fixed): `//notebook:slider` deciding whether a cell is an *editable input at all*, so `slaTarget() Probability` (which has `Bounds()` but no directive) silently wasn't a control. Leaf-ness is a *type* fact — a cell is a leaf iff its output is widget-capable, a scalar basic kind (parameterless), or a `Reconciler` (parameterized). The directive only refines how the control **renders**. Carried on `graph.Cell.IsLeaf`.
- **Guessing a basic kind from the type string** (fixed alongside it): codegen classifying `--set` coercion by pattern-matching the rendered type name, defaulting named types to `float64` — which silently miscompiled a `bool` leaf. The underlying kind is a `go/types` fact; it is carried on `graph.Result.Underlying`, filled by the analyzer, so codegen never parses a type name.

The test for any new feature: *is this fact one `go/types` already computed?* If yes, the analyzer records it in the plain-data IR and every later stage reads it. A comment configures **presentation**; it never decides **structure or correctness**.

---

## 6. The CLI

```
go tool notebook run   <dir|file>    analyze → generate → build → serve → open browser
go tool notebook check <dir|file>    analyze only: graph diagnostics, cycles, purity. Exit non-zero on error.
go tool notebook build <dir|file>    emit the binary and stop (GOOS/GOARCH honoured)
```

`go tool notebook run` watches the source file. On change: re-analyze, rebuild, restart, **reload head**. Restarting is a non-event because the only state is a few floats — which is the entire architectural difference from a Jupyter kernel, and the loop should make that visible.

Flags to accept now because they cost nothing and prove the shape:

```
--headless          no browser; run once and exit
--set leaf=value    override a leaf before running
--json              emit final cell values as JSON
--serial            disable goroutine fan-out (restores per-cell stdout for debugging)
```

`--headless --set --json` is the batch story from the design doc, and it should work in this milestone. It is four hours of work and it is the most differentiated thing in the whole project.

---

## 7. Milestones and kill criteria

| M | Deliverable | Done when |
|---|---|---|
| **M0** | Module, layout, CI (`go vet`, `go test -race`, `golangci-lint`) | Empty binary builds |
| **M1** | `internal/graph` + `internal/analyze` | `go tool notebook check examples/capacity` prints the correct graph. Golden tests over `testdata/graphs` pass. Cycles and duplicate producers produce good diagnostics. |
| **M2** | `internal/gen` + overlay build | `go tool notebook build examples/capacity` produces a binary. User's directory unmodified. |
| **M3** | `engine`: head, snapshot, epochs, scheduler, cache | Unit tests: glitch test, supersede test, `-race` clean with parallel branches. |
| **M4** | `engine/server` + minimal UI | `go tool notebook run examples/capacity` opens a browser, sliders move, charts repaint. |
| **M5** | `--headless --set --json` | Same file runs as a batch job. |

### Kill criteria — measure these, do not hope

- **KC1 — cold analysis.** Full graph derivation on `capacity.go` **< 1 s**. *(A cold load happens once, at launch, hidden behind the browser opening — it is not the interactive path. Original target was < 50 ms; that conflated cold startup with the edit loop and was relaxed. Measured: 86 ms.)*
- **KC2 — the one that matters.** Re-analysis after a one-cell edit **< 100 ms**. If a *warm incremental* re-typecheck is seconds, the `go/types`-per-keystroke plan is dead and gopls moves from "later" to "now." *This is the number the whole milestone exists to produce.* **It is pure `internal/analyze` and does not need the engine, so it is measured in M1, not M3.** Measured: 0.48 ms (incremental `Session`).
- **KC3 — interaction.** Slider drag → repaint, p95 **< 50 ms** on `capacity.go`.
- **KC4 — the edit loop.** Save a cell body → rebuild → restart → head restored → repainted, **< 500 ms**.

If KC2 and KC4 land, the design is alive and everything in the deferred list is worth building. If KC4 is >2 s, the compile-first bet fails at the interactive tier and the honest response is to *change the pitch* (batch and cluster work, where the loop is irrelevant) rather than to paper over it. **KC2 has landed (M1) with a ~200× margin.**

---

## 8. Testing

> **A test that observes an effect must observe the effect it *names*.** "Events streamed" is not "the value changed." "Output discarded" is not "compute abandoned." "The chart repainted" is not "the chart repainted *because of what I did*." Every real bug in this project has hidden in exactly that gap: the inert slider (the chart repainted, but from the default, not the edit), the false drag-coalescing (99 waves were "discarded" — after running to completion, not instead of it), the toothless glitch test (edits didn't overlap, so no glitch was possible), the strconv miscompile (the notebook that would have caught it wasn't built). When a test asserts a mechanism works, assert the *consequence the mechanism exists for* — the changed value, the abandoned work, the observed race — not merely that the machinery moved. If you can't observe the consequence directly, the test is decoration.

> **A gate that cannot fail is worse than no gate — you stop looking.** The same rule turned on the checking machinery itself. `make check` exists to make CI failure unrepresentable locally ("the fix isn't discipline, make the gates unable to differ"). But its lint step was written `golangci-lint run ./...; echo ok` — the `;` swallows the exit code, so the gate passed while lint failed. One character. The mechanism built so failure couldn't hide was itself hiding failure. **Running is not passing.** Any gate, hook, or CI step must propagate the failure of the thing it runs (`&&`, not `;`; check the exit, not that the command was invoked), and you must *verify it fails when it should* — a gate is only real once you've watched it go red. This is §8 applied one level up: the gate is a test of the tests, and it too must observe the effect it names.

- **`internal/graph`** is plain data with no I/O — table-driven tests, high coverage, no fixtures beyond JSON.
- **Golden graphs.** `testdata/graphs/*.go` → `*.want.json`. Every design-doc notebook that this milestone can parse goes in here, including ones whose *features* are unimplemented — parsing must not regress when they land.
- **Diagnostics are golden too.** `testdata/cycles/` and `testdata/errors/` with exact expected messages. Message quality is a feature; test it like one.
- **Glitch test.** Fabricate a diamond (`a → b, a → c, {b,c} → d`), make `b` slow, change `a`, assert `d` never observes a `b` and `c` from different epochs. This is the correctness bug the whole scheduler exists to prevent.
- **Supersede test.** Fire 100 leaf edits; assert exactly one settles and 99 are `StateStale`.
- **`go test -race`** on everything in `engine`. Non-negotiable — the parallel fan-out is the design's headline dividend and a data race in it would be an embarrassment.
- **E2E.** Build `examples/capacity`, drive the WS, assert the values.

---

## 9. Conventions

Go 1.24. Apache 2.0. `golangci-lint` with `errcheck`, `govet`, `staticcheck`, `revive`.

Errors wrapped with `%w`, no panics across package boundaries (`recover` only in the scheduler's node runner). Contexts threaded everywhere; no `context.TODO` in shipped paths.

`engine` is a **public API**: godoc every exported symbol, and treat its shape as versioned from the first commit, because generated code depends on it.

Commit granularity: one milestone per PR, each independently reviewable.

---

## 10. What to do first

1. `examples/capacity/capacity.go` — copy verbatim from the design doc. It is the fixture for everything.
2. `internal/graph` — the IR and its checks, with tests, **before any `go/types` code**. It is the thing every later decision touches, and it is pure data.
3. `internal/analyze` — populate the IR.
4. `go tool notebook check` — the first thing a human can run.

Then M2–M5.

**Get KC2 and KC4 on the board as early as possible.** They are the reason for the milestone. Everything else is scaffolding around a measurement.
