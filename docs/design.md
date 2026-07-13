# A Reactive Notebook in Go

*Design notes. Six ported notebooks, three bugs found, one new concept required.*

---

## The bet

**A cell is a function.**

Everything else is a consequence. marimo's real trick was never the `@app.cell` decorator — it was that a cell's dependencies live in its signature, derived by static analysis, so the notebook file is a valid Python module with no JSON envelope. Go can go further, because a Go function signature *already* declares typed inputs and typed outputs.

So the dependency graph is not a structure you build alongside the code. It is a projection of the code's own call graph.

```go
//go:notebook
package capacity

// Incoming jobs per hour.
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }

// Servers in the fleet.
//notebook:slider min=1 max=256
func servers() (c int) { return 80 }

// Offered load in Erlangs.
func offeredLoad(lambda, mu PerHour) (a Erlangs) {
    return Erlangs(float64(lambda) / float64(mu))
}

// Server utilization.
func utilization(a Erlangs, c int) (rho float64) {
    return float64(a) / float64(c)
}
```

That is the whole notebook format. It compiles as an ordinary Go package. `go doc` gives you the cell labels. `go test` calls the cells directly. `gopls` does cross-cell jump-to-definition, because cells *are* functions.

**The relationship between notebook and program is inverted.** marimo notebooks are Python files you *can* import. These are Go packages that *happen to have* a reactive mode. The notebook is a view onto a package, not a container around one.

---

## The wiring rule

One sentence:

> **A cell's named result feeds any cell that takes a parameter of the same name and type.**

It typechecks. You can read it off the file. `arrivalRate` returns `lambda PerHour`; `offeredLoad` takes `lambda PerHour`; that is the edge.

This is strictly stronger than marimo's name-matching, which cannot stop you crossing an `int x` to a `string x`. Two cells returning different things of the same type stay distinct because names disambiguate: `priceRange` returns `prices Range[USD]` and `piecePriceRange` returns `piecePrices Range[USD]`, and a downstream cell takes both.

Two exceptions, both narrow and both idiomatic:

- **A trailing `error` result is not an edge.** It marks the cell failed and blocks downstream cells, which display "blocked upstream" rather than a wrong number. Errors are values in the graph, not process death.
- **A `context.Context` parameter is injected, not wired.** It is the one parameter with no upstream cell. Cells that want to be cancellable ask for it; cells that don't, don't.

**Undocumented top-level functions are not cells.** They are ordinary helpers. The doc comment is the marker, and it is also the label — you'd have written it anyway.

Implementation: `go/types` over the synthesized package, `Info.Defs` and `Info.Uses`, cross-referenced against cell spans. The type checker has already done the semantics. Migrate to a headless `gopls` when incremental re-checking on keystroke starts to hurt — which it will, and which is the first real engineering risk in this document.

---

## The view is a readout of the code

Nothing is imported. The UI is derived from the shape the code already has.

| What | Comes from | Why |
|---|---|---|
| **Label** | the doc comment's first sentence | Go's own doc convention. You wrote it anyway. |
| **Control kind** | the return type | `Unit` has `Bounds()`, so it renders as a `[0,1]` slider. |
| **Layout** | source order | You already ordered them. |
| **Everything else** | a `//notebook:` directive | The residual, and it is small. |

**Structural typing is what makes this importless.** A type satisfies `Bounds()` without importing whatever package declares it. The author writes a method shape; only the runtime names it:

```go
if b, ok := ret.(interface{ Bounds() (float64, float64) }); ok {
    renderSlider(b.Bounds())
}
```

Python has no structural satisfaction, so `mo.ui.slider` is the only gateway to a slider and `import marimo as mo` is load-bearing. Here "sliderness" is a property a type *has*. A notebook file can have **zero notebook-specific imports**.

An importable `notebook` package still earns its place — for autocomplete, a curated vocabulary, and a compile-time `var _ Bounded = Unit(0)` check — but nothing in the file is ever *required* to import anything.

### The degradation ladder

Strip lines and it never breaks, only plainens:

1. `func alpha() float64` — text field, label from the function name
2. `func alpha() Unit` — slider `[0,1]`
3. `+ doc comment` — real label and tooltip
4. `+ //notebook:` — override the rare ambiguous case

Delete every comment and every semantic type and the notebook still runs and still renders. **Losing the view costs polish, never correctness.** If it could cost correctness, you put compute in the view layer by mistake.

The same ladder runs on the **output** side, and symmetry is the point: a value with a `Render()` method shows its rich view; a bare scalar (a `float64`, or a named type over one like `Erlangs`) shows its value as a `text/plain` readout, live, updating as upstream leaves change; a composite with no `Render` (a raw slice/struct) shows nothing and the transport hides it. So *every value the graph computes is visible by default* — the display mirror of "nothing editable is invisible." The engine supplies the scalar readout in `doneEvent` (the "caller-chosen default readout" `AsRendered` always anticipated); it is not an engine API change — `Event.Out` was always the display seam, scalars simply stop being nil. A leaf's control already *is* its view, so the client suppresses a leaf's cell-body to avoid echoing an input as a value.

### Type or comment?

Not a matter of how much you care. The question is: **does any cell downstream depend on this being true?**

- `Scale.Bounds()` returns `[0.70, 1.0]` **as a type**, because `seamOrder` *reads it* to decide how far to precompute. It is a compute precondition.
- `//notebook:step=0.05` is **a comment**, because nothing computes differently when the slider moves in fives.

Reach for the comment. Upgrade to a type the day a cell needs to trust the bound. The one invariant: when both describe the same control, **the type wins** — not because types outrank comments, but because if a value is a `Unit`, `[0,1]` is what it *is*.

---

## Widgets are values

A widget is a plain Go value with a method shape. The registry is keyed by **capability**, not by concrete type:

```go
type Bounded interface{ Bounds() (lo, hi float64) }

type Unit float64
func (Unit) Bounds() (float64, float64) { return 0, 1 }
```

The registry has no `Unit` entry. It has a `Bounded → slider` entry. Every bounded numeric you ever define renders for free. The registry grows with the number of *presentation categories* — slider, select, toggle, table, grip — which is a small, nearly-closed set. It does not grow with your domain types.

**A cell may return a widget.** This is the one addition the ports forced, and it covers a surprising amount:

```go
// Price range. Bounds track the data; your selection survives the data changing.
func priceRange(rows []Set) (prices Range[USD]) {
    return Range[USD]{Lo: 0, Hi: maxOf(rows, Set.price), From: 0, To: 150}
}
```

The cell computes the **schema** (bounds, options). The head holds the **selection**. Reconciliation is **per-widget-kind**, not universal — a correction the ports forced:

- `Range` → **clamp** into the new bounds
- `Multi` → **filter** out options that no longer exist
- `Draggable` → **reset** on arity change (control point #3 of a quintic is not control point #3 of a septic)

This is already better than the original it was ported from: marimo *reconstructs* `price_range` whenever `subset` changes, so picking a new theme resets your price filter. Here it clamps.

### Three rules that forbid a framework

A widget system metastasizes when it acquires exactly three capabilities. Forbid each structurally:

1. **The registry is keyed by capability, not instance.** There is nowhere to put per-instance config. "Make *this* one `[0,100]`" means it's a `Percent`, a different type.
2. **Renderers are pure functions, `value → element`.** No state, no callbacks. A renderer that *cannot* hold state cannot become a component toolkit.
3. **All reactivity is graph edges.** Widget-to-widget wiring is *structurally unrepresentable*. "This slider's range depends on that dropdown" is not a widget feature — it's an edge, and it already exists. This is what ipywidgets regrew (`observe`, `link`, `jslink`) because its widget layer and compute layer were separate substrates that then had to be reconnected. Here they aren't separate.

### Direct manipulation

A brush on a chart, a draggable control point, a pannable crosshair — the whole "the widget *is* the data" category — is one mechanism:

> **The renderer reads the leaf. The runtime writes it. A write is not an edge.**

`editor` depends on `ctrl` (it draws the handles). `ctrl` does not depend on `editor` (dragging is an edit to the head, exactly like moving a slider). Acyclic. No two-way binding. No cycle to detect, because the cycle is unrepresentable.

The renderer emits *declarative grips*; the runtime binds them to leaf writes. And the grip names its leaf without a string — `Draggable[T]` carries an opaque runtime-stamped token, so `ctrl.Grip(i)` is typechecked. **The leaf's identity rides along with the value.** That is only possible because widgets are values. A grip drag still goes through the one `Head.Set` chokepoint — the token says *which* leaf, not *how* to write it — so grips prove the no-cycle claim by using the same write path as a slider.

> **The token's real justification — cross-cell leaf identity — and a near-miss worth recording.** The original argument for the token was "a string would re-import stringly-typed coupling," which is only an aesthetic objection. The corpus supplies the structural one: **a grip is emitted by a cell that does not own the leaf it writes.** curvefit's grip lives in `editor`'s `Chart` and writes to the `ctrl` leaf (`controlPoints`); mandelbrot's lives in `view`'s `Image` and writes to the `at` leaf (`center`). *Every* grip in the corpus is cross-cell — and that separation is the whole "the renderer reads the leaf, the runtime writes it" claim: `editor` composes a chart from `samples`, `ctrl`, and `fit`, draws the handles, and must not *own* them. So the identity has to ride with the value precisely because the value crossed a cell boundary. `CellMeta.Widget.Kind` associates a leaf with *its own* cell and therefore cannot supply a foreign cell's grip identity. A revision of this doc proposed deleting the token as redundant-given-`Kind`; that reasoning held only for in-place controls and was refuted by the corpus. The near-miss is the lesson: the check was framed around the wrong criterion (multi-leaf-per-chart, which the corpus lacks), would have passed, and the mechanism would have been deleted — caught only by not stopping at the answer.

marimo does this entire category by dropping to anywidget — i.e. writing JavaScript. Here it is one Go cell.

**The honest boundary:** grips cover direct manipulation of *structured numeric leaves*. Drawing canvases, gamepads, webcams, keyboard capture are genuinely arbitrary input and need a raw HTML/JS escape hatch. That escape hatch *is* a framework surface. Quarantine it and say so.

---

## State: the one new concept

Everything above fell out of "cells are functions." This did not.

A timer is trivial — a leaf the runtime writes on a schedule instead of the user dragging it. A robot moving a slider. But nobody wants a clock; they want to **accumulate** over ticks, and a pure DAG of pure functions cannot accumulate.

```go
// Queue state. The one stateful cell in the notebook, and it says so in its signature.
func sim(prev Prev[Sim], tick Tick, seed int64, lambda, mu PerHour, c int) (state Sim)
```

`Prev[T]` holds the cell's own previous output. A self-edge — legal because it is **delayed**: it reads the last epoch, not this one. This is Lustre's `pre`, Elm's `foldp`, a clocked register in VHDL. It comes with that tradition's rule, which the toolchain enforces:

> **A cell taking `Prev[T]` must also take a `Tick`. A register needs a clock. It steps when the `Tick` advances — and *not* when any other input changes.**

That last clause matters. `sim` depends on the arrival-rate slider. In a pure DAG, changing an input re-runs the cell — which would advance the simulation one step *every time you nudged a slider*. The `Tick` is the clock; everything else is a parameter absorbed into the next step. **The signature says which is which, so the runtime never guesses.**

### When to fold, and when not to

The ports produced a rule I now trust, because it appeared independently in three unrelated domains:

> **Path-dependent state** (queue depth, a PRNG walk, a viewport history) → `Prev[T]`.
> **A sufficient statistic** (a sum, a count, a max) → an ordinary cell.
>
> **Relative gestures accumulate. Absolute controls recompute.**

Bayesian updating is *literally called updating*, and the fold is the wrong tool for it: the posterior depends on the data only through ΦᵀΦ and Φᵀt, which are **sums** — a monoid, order-free, a pure function of a value the graph already holds. Using a fold there is not merely unnecessary, it is **harmful**: *a fold only goes forward*. Scrub the slider back from 40 points to 5 and a fold must replay from zero. A pure cell recomputes. Backward scrubbing — the thing that makes that demo worth looking at — works *only* because nothing in it is stateful.

And the tempting patch ("keep a history of states inside the fold so you can index backward") is just the prefix cell wearing a costume. If you find yourself doing that, you didn't need a fold.

This is why every zoom UI has a back button and no slider does.

### Randomness

Splits along the same seam, and *both* answers are pure:

- **Path-dependent** (a random walk): the PRNG state lives **inside the folded value** — a `uint64` field.
- **Not path-dependent** (sampling lines from a posterior): the **seed is an ordinary input leaf**, passed as an argument to every stochastic cell.

Either way there is no global RNG, because state has nowhere to hide when the only place it *can* live is a struct field or a function argument. Compare `np.random`'s global state — the classic hidden-state bug that makes notebooks silently unreproducible.

### What state costs

Three things, and they only apply to notebooks that actually have a `Prev` cell:

- **Cache eviction becomes mandatory.** Ticks generate unbounded distinct input keys.
- **Wave coalescing becomes unsafe.** Drag-coalescing was free; a fold must see *every* tick or it silently drops steps. Ticks feeding a fold are **queued, not coalesced**.
- **A replay log comes back.** A fold's output depends on *when* you moved the slider. Stateless notebooks need only "code + current values." Stateful ones need the edit timeline to replay — and the distinction is visible in one cell's signature.

---

## The runtime

There is no host process that loads notebooks. **The notebook is the binary.**

### Toolchain (compile time)

`go tool notebook run capacity.go` loads with `go/packages`, typechecks, discovers cells, matches results to parameters, checks for cycles, and emits one generated file *in the notebook's package* (so it can see unexported cells):

```go
// capacity/notebook_gen.go  (generated, gitignored)
var Cells = []engine.Cell{
  {ID: "utilization", In: []string{"a", "c"}, Out: "rho",
   Fn: func(in engine.Inputs) any {
     return utilization(in["a"].(Erlangs), in["c"].(int))
   }},
  ...
}
var Widgets = []engine.Widget{
  {Leaf: "servers", Kind: "slider", Min: 1, Max: 256, Label: "Servers in the fleet."},
  ...
}
```

The type assertions are safe by construction — codegen knew the static types. Doc comments and `//notebook:` directives are already flattened. **Nothing is parsed at runtime.**

### Engine (a library, linked in)

**Head** — `map[leaf]any`. The only mutable state in the system. Persisted, so reopening restores your sliders.

**Scheduler** — the load-bearing piece. An edit bumps the epoch and snapshots the head **immutably**; every cell in the wave reads that snapshot. Glitch-freedom falls out: no cell can see new-λ against old-μ, because there is no shared mutable view to observe halfway. The dirty set is the transitive downstream closure; execution is a topological walk where siblings **fan out onto goroutines**. Independent branches run in parallel with no GIL — the one place the language choice pays a dividend instead of a tax.

Superseding gives drag-coalescing for free: each wave carries a `Context` cancelled when a newer epoch starts, and results are epoch-checked before commit. Three hundred drag events, one settled recompute, no debounce logic.

**Cache** — keyed on input versions, so no hashing of arbitrary Go values. Propagation pruning via a structural ladder: `==` if comparable, else `Equal(any) bool` if present, else assume changed. Same probe pattern as `Bounds()` and `Render()`.

**Cacheability is derived, not declared.** A cell that transitively calls `time.Now()`, an RNG, or does I/O is impure and therefore uncacheable. That's an ordinary `go/callgraph` query. *I briefly invented a `//notebook:nocache` directive for a benchmark cell and it was wrong* — a comment that changes whether the answer is correct violates the type-vs-comment rule. The toolchain already knew.

**Purity is not the only verdict that callgraph pays for — and I conflated two of them.** When the WASM topology got built, "which notebooks can run client-side?" looked like the purity question already answered. It is not. **Purity → cacheability. Portability → deployability. Same callgraph machinery, orthogonal verdicts.** Purity disqualifies `time.Now` and an RNG because they break caching — but both run *fine* in a browser. Portability disqualifies `net`, `os`, and cgo because they have no client-side form — but a cell that calls `time.Now()` in a loop is impure yet perfectly WASM-able. So a cell has (at least) two independent derived properties, computed by the same `go/callgraph` walk with different disqualifying sets: `Pure` (no time/rand/IO → cacheable) and `WASMable` (no net/os/cgo → deployable to the browser). Treating "is it portable?" as "is it pure?" would have wrongly grounded every notebook that so much as reads a clock. The lesson is the general one: a cell is a bundle of *orthogonal* attestable properties over one call graph, not a single pure/impure axis. (Conservative-by-default holds for both: over-rejecting portability annoys; under-rejecting ships a `.wasm` that panics on `os.Open`.)

**Transport** — the binary serves HTTP/WS. Renderers run **in Go, in-process**; the client receives `{cell, mime, data}` and is entirely ignorant of Go types.

### The one thing that genuinely doesn't work

**No per-cell stdout.** `os.Stdout` is a package var; parallel cells writing to it interleave into garbage, and per-cell capture would require serializing the wave. `fmt.Println` goes to your terminal as dev logging. If you want something *in* the notebook, **return a value**.

This is a real capability Jupyter has and this does not, and it is the direct price of the goroutine fan-out. A `--serial` debug flag buys it back when you need it.

Panics get `recover()`'d per node into a typed error state on that cell.

---

## Where it runs

A notebook is a self-contained artifact with **no runtime dependency on the toolchain**. So:

```
sbatch ./capacity        # your notebook is the job script
```

No JupyterHub. No spawner. No kernel specs. No conda env drifting from the one you developed in. You built it on your laptop, `scp`'d one file, and it ran under Slurm with real MPI ranks and real GPUs. **The binary that produced the figure is bit-identical to the binary you'll rerun in six months.** Reproducibility stops being a practice you enforce and becomes a property of the artifact.

Every other notebook system's artifact is *the notebook plus an environment that reconstitutes it*. The thing you run is never the thing you shipped. Here they are the same object.

### The toolchain is placeable

Because the state is a `map[leaf]any` — a few floats — and everything else is derivable, the edit loop is: **browser edits text → server compiles a package → new binary → reload head → recompute.** Statelessly. The binary is disposable, which is what makes it relocatable.

| Toolchain | Compute | What it is |
|---|---|---|
| local | local | `go tool notebook run` — the laptop case |
| local | remote | build, push, `sbatch` — the job case |
| remote | remote | browser IDE + build service — the hosted case |
| remote | browser | WASM — *portable* notebooks (no net/os/cgo), no server at all |

Same file, same engine, four topologies, distinguished only by **where you put a compiler**. And the build service is *boring* — a Go build cache and `go build`, stateless, a Lambda or a warm pod. The thing everyone assumes is the fatal cost of compile-first turns out to be the cheap cacheable part, while Jupyter's "free" interactivity costs a long-lived stateful kernel per user with a memory footprint and a babysitter.

All four are now built and measured. The fourth — compute in the browser — was the real test, because the only reason it *should* work is the discipline that the engine never imports a transport: `engine/wasm` is a syscall/js sibling of `engine/server`, and standing it up required **zero changes to any existing engine public API** (the diff of every engine file is empty). capacity compiles to `GOOS=js GOARCH=wasm` at ~1 MB gzipped, cold-loads to an interactive slider in ~40 ms, and repaints in ~300 µs — an order of magnitude smaller than an interpreter-shipping stack (Pyodide alone is 10 MB+ before the notebook), because a compiled program is not an interpreter plus a program. **One honest caveat:** `GOOS=js` is single-threaded, so the goroutine fan-out — the parallel-branch dividend that is the whole point of the language choice — is *absent* in the browser. Waves run serially. Correct, just not parallel; invisible on capacity, real on anything compute-heavy.

**A fifth topology is rejected, not deferred: the browser as editor.** It is tempting — WASM already runs the notebook in the tab, so why not author it there too? Because a notebook *is a plain Go file*, and authoring a Go file is a solved problem owned by your editor and `gopls`: completion, go-to-definition, refactoring, type errors as you type, the whole apparatus. A browser text pane would be a strictly worse editor with none of it, and building one would violate the project's own premise — that a notebook carries no special format, so the ordinary Go toolchain already edits it. The browser's job is the *other* end: it is an **output, diagnostic, and graph surface** — it shows what the notebook computes, what failed and where, and the live dependency graph, none of which your editor does. Input stays with the tools built for it. This line matters because "add an editor to the browser" will look like an obvious next feature forever; it is a foreclosed one. (Contrast the genuinely-deferred items — SQL, `Prev`, the widget vocabulary — which are *not yet built*; this is *chosen against*.)

### Headless

Because the head is a file:

```
go tool notebook run capacity.go --headless --set servers=120 --json
go tool notebook run queue.go    --headless --ticks 100000 --set lambda=1400
```

The **same file** is an interactive notebook, a batch job, and a callable model. Papermill exists as a separate tool to fake this for Jupyter. Here it is a consequence.

And fold + clock is a discrete-event loop — so a notebook with a `Prev` cell is a **simulator**, driven from the CLI.

---

## SQL and data that doesn't fit

These are the same problem: a SQL cell must return rows *of some Go type*, and if the data doesn't fit in RAM that type cannot be a slice of the rows.

**The struct is the schema.**

```go
type Trip struct {
    Pickup   time.Time `parquet:"tpep_pickup_datetime"`
    Dropoff  time.Time `parquet:"tpep_dropoff_datetime"`
    Fare     USD       `parquet:"fare_amount"`
    ...
}

func demand(all Rel[Trip], m Select[Month]) (hours []HourStat, err error) {
    return Query[HourStat](all, `
        SELECT hour(Pickup) AS Hour,
               count(*)     AS Trips,
               avg(Fare)    AS MeanFare
        FROM trips WHERE month(Pickup) = ? GROUP BY 1 ORDER BY 1`, m.Value.N)
}
```

The toolchain parses that string at **build time** and checks every identifier against `Trip`, and the result columns against `HourStat`:

- a column that isn't a `Trip` field → **compile error**
- a result struct that doesn't match the `SELECT` → **compile error**
- `avg(Fare)` assigned to an `int` → **compile error**
- **rename a column and every SQL cell that used it fails to compile**

No data is needed to compile — the struct *is* the schema. The parquet file's actual schema is validated against it once, at load, so a mismatched file is an error rather than a silently-wrong column. Three-way agreement between struct, query, and file.

marimo *cannot* do this. Not "does not" — there is no compile step to hang it on. This is the strongest claim in the design and it is a straight consequence of deciding to compile, ten decisions earlier, for unrelated reasons.

### A path is not a handle

`Rel[Trip]` is a handle on an edge, which should set off an alarm — a handle on an edge is exactly what let marimo's portfolio tracker chart a portfolio of secretly-Microsoft stocks. The distinction:

> **A path is not a handle. A handle identifies its contents.**

`parent_folder` was the constant `Path("invest-data")` — identical whether the download succeeded, failed, or fetched the wrong company. `Rel` carries source + row count + schema hash: change the file, the handle changes, everything downstream invalidates. Both are "a reference on an edge"; only one is a **value**.

Out-of-core works because the SELECT touches 42M rows and returns 24. **Pushing compute to the data is not an optimization here — it is the only reason a slice of the result is a legal Go value at all.**

### The wound

**DuckDB is cgo.** `CGO_ENABLED=0` breaks, and with it the one-line "cross-compile, scp, sbatch" story that everything above leans on.

The honest resolution is two tiers, visible in the file's imports: **pure-Go parquet** for scans and simple aggregates, keeping the static binary intact; **DuckDB via cgo** when you need real SQL. Cross-compiling cgo is possible (zig cc) and it is not free.

Two more limits, stated plainly. The toolchain now needs a **SQL parser and typechecker** — the single biggest piece of engineering in this document, bigger than the scheduler. `sqlc` proves it's tractable; it is still real work, and dialect-specific. And **unit types don't survive arbitrary SQL arithmetic**: `avg(Fare)` → `USD`, but `avg(epoch(Dropoff)-epoch(Pickup))/60` → `float64`. I won't pretend to infer units through expression trees.

---

## What the ports found

Six notebooks from marimo's gallery, ported to test the design. They found three real bugs, and the bugs are more interesting than the port.

### Lego dashboard — a dimensional error

```python
.with_columns(inflation = pl.col("price") * 1.03**(pl.col("year") - 1970))
...
.with_columns(price = pl.col("price") * pl.col("inflation"))   # dollars × dollars
```

`inflation` is an adjusted *price*, in dollars. Multiplying `price` by it yields **price² · factor**. The intent was obviously `price × factor`.

It survives because in Polars everything is `f64` and columns are strings, so nothing objects. Type `inflation` as a `Factor` and `price * inflation` is a **compile error** — you're forced through `.scale()`, which is the moment you'd notice.

### Seam carving — the cache was patching a broken graph

Two problems. First, `find_seam` computes the DP table and returns only `backtrack`; `remove_seam` then picks the seam's start column with `argmin(backtrack[-1])` instead of `argmin(dp[-1])`. Backpointers in the bottom row are all within ±1 of their own column, so that argmin is **column 0, every time** — verified: 200/200 on random energy maps, agreeing with the correct choice 0/200 times. The gallery's seam carving never does minimum-energy seam selection. It shaves the left edge.

Second, and more instructive: the notebook's stated purpose is *"a demonstration of marimo's caching feature, which is helpful because the algorithm is compute intensive."* But `@mo.cache def efficient_seam_carve(image_path, scale_factor)` makes the DP depend on the slider, so every new slider value re-runs the entire carve. **The seam order doesn't depend on the slider** — carving to 0.85 is a strict *prefix* of carving to 0.70. Hoist it and the slider becomes an O(pixels) filter and the notebook needs **no cache at all**.

The memoization wasn't making a slow notebook fast. It was compensating for an edge that shouldn't exist — and hiding the mis-factoring instead of exposing it.

### Portfolio tracker — the edge carried a token

```python
yf.Ticker("MSFT")                   # hardcoded
  .assign(Ticker=ticker)            # relabeled as whatever you asked for
  .to_csv(f"{parent_folder}/{ticker}.csv")
```

Every ticker downloads Microsoft's history and writes it to `AAPL.csv`, labeled AAPL. And because the fetch is guarded by `if not (...).exists()`, once the wrong file lands it is **never re-downloaded**.

No type catches that — `Ticker("MSFT")` is a fine `Ticker`. What *enabled* it is that `download_tickers` returns `None` and the next cell depends on `parent_folder`, a **constant**. The graph is structurally incapable of noticing anything is wrong. The **stickiness** is 100% attributable to file-as-cache, and value-flow eliminates it: `prices` **returns the bars**, the memo key is the ticker set, and there is no filename to go stale behind your back.

(Also, quieter: the original joins investments to prices on an exact date string. Buy on a Saturday and your money silently vanishes.)

### The recurring pattern

Three notebooks, one move, and the gallery misses it every time:

> **When a slider indexes into a prefix of something, the prefix belongs in its own cell.**

- seam carving: `seamOrder(img)` doesn't depend on `scale`
- Bayesian regression: `evidence(data)` doesn't depend on `n`
- portfolio: cumulative shares don't depend on the display window

The graph makes that refactor obvious. A cache makes it invisible.

---

## What is actually unresolved

Everything above is design, and design was the easy part. The remaining risk is engineering, and porting cannot touch it.

**1. Incremental static analysis.** The graph derivation is only pleasant if re-deriving on keystroke is cheap. `go/types` is not incremental. Either cache symbol tables and re-check only the edited cell (works until a definition's *type* changes), or drive a headless `gopls`, which already does incremental cross-file defs/uses. **This is the largest risk in the project.**

**2. Glitch-free propagation.** The epoch'd immutable snapshot is the answer, and it is the one piece of "think hard" that survived every round, because it is an actual correctness bug a user sees — not a purity concern.

**3. The SQL typechecker.** Tractable (`sqlc` exists), large, dialect-specific.

Those three are where the weeks go.

---

## The honest position

**Where this belongs.** Not "replace Jupyter for data science." Go has no pandas, no matplotlib, no scipy — though far less of that is a real gap than it looks, because much of the Python scientific stack is compensating for the *interpreter*, not for the domain, and the genuinely valuable parts (LAPACK, the distributions) were never Pythonic in the first place. Gonum is good. Plotting is SVG and a `Render()` method, and the hand-rolled charts in these six notebooks are cleaner than the matplotlib incantations they replaced.

The niche is **systems, simulation, and cluster work** — queueing models, capacity planning, benchmark harnesses, workload characterization — where the compute is code you'd write in Go anyway, the artifact needs to run as a job, and the thing you're modeling *is* infrastructure. There is no incumbent there. Jupyter-on-HPC is conda environments and spawners; this is one file you `scp`.

**Why it holds together.** The same cut kept working at every layer, which is usually the sign the decomposition is right rather than locally tidy. The **compute/view seam** drew the line for widget config, registry keying, import surface, drag telemetry, log retention, and cacheability — six unrelated questions, same answer, every time. Compute is what must be in the graph, ordered, attestable. View is what can degrade to a text field without anyone being wrong.

And every "how do we avoid X" turned out to be "you already avoided it three decisions ago." That is the opposite of the accretion pattern. **Fewer layers isn't a gap to be filled. It's the point.**

**What it cost.** One new concept (`Prev` + `Tick`). Three corrections the ports forced (per-widget reconciliation; cacheability is derived, not declared; the edit log is real but only for stateful notebooks) — plus a fourth the WASM build forced: purity and portability are *different* callgraph verdicts, not one (see above) — plus a fifth the widget build *reaffirmed*: the grip `leafToken` was nearly deleted as redundant-given-`Widget.Kind`, until the corpus showed every grip is cross-cell (drawn by a cell that doesn't own its leaf), which `Kind` can't identify — so the token stays, now justified structurally rather than aesthetically (see above). One genuine wound (cgo, for SQL). No per-cell stdout, and no goroutine parallelism in the browser tier.

Everything else compounded from a single sentence.

> **A cell is a function.**
