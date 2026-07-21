# go-notebook: A Reactive Notebook Where a Cell Is a Function

*A system paper. Everything here is built and running; where something is unbuilt, deferred, or withdrawn, it is labelled as such. The corpus, the numbers, and the findings are reproduced in CI, and the source is one click away.*

---

## Abstract

go-notebook is a reactive notebook toolchain for Go in which **a notebook is an ordinary Go package and a cell is a top-level function.** The reactive dependency graph is not a structure the author maintains; it is *derived by the Go type checker* from function signatures — a cell's named result feeds any cell that takes a parameter of the same name and type. From that single decision everything else follows: the notebook file has no JSON envelope and imports nothing from the project; the same file runs three ways (interactive in a browser over WebAssembly, headless as a batch job, or served over HTTP) distinguished only by where you point the compiler; controls, charts, and even direct-manipulation "grips" are read off cell *types* by structural capability probing rather than declared; and the artifact you ship is a single static binary with no interpreter, no kernel, and no environment to reconstitute.

The system is deliberately small: one new concept, a handful of load-bearing rules, and no SQL parser or cgo in the toolchain (typed Go over a content-addressed handle gives compile-checked schemas without them). Its most transferable finding is a discipline, not a feature — *"running is not passing":* a system producing a thing is not the same as the thing reaching anyone, and verifying by observed consequence caught bugs that green test suites passed over. A corpus of 44 notebooks (38 running live in the browser) exercises the design across queueing theory, statistics, physics, distributed systems, and HPC, and each was built as a falsification attempt: 19 of them across GPU/DFT/ODE/gradient-descent/cellular-automata subjects required **zero engine changes and zero violations of "a cell is a function."**

---

## 1. The bet

> **A cell is a function.**

Everything else is a consequence.

marimo's real advance over Jupyter was never the `@app.cell` decorator — it was that a cell's dependencies live in its signature, derived by static analysis, so the notebook file is a valid Python module with no JSON envelope. Go can go further, because a Go function signature *already* declares typed inputs and typed outputs. So the dependency graph is not a structure you build alongside the code. It is a projection of the code's own call graph.

```go
//go:notebook
package capacity

// Incoming jobs per hour.
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }

// Servers in the fleet.
//notebook:slider min=1 max=256
func servers() (c int) { return 80 }

// Offered load in Erlangs. lambda and mu wire in by name+type — that is the edge.
func offeredLoad(lambda, mu PerHour) (a Erlangs) {
    return Erlangs(float64(lambda) / float64(mu))
}
```

There is no import of this project anywhere in that file — only the `//go:notebook` marker, in the same register as `//go:generate`. The file is a compilable Go package a human reads as Go and the toolchain reads as a notebook. That property — *the notebook is the source, not a container for it* — is the root from which the rest of the design grows, and it is the constraint every later decision was checked against.

## 2. The wiring rule, in one sentence

**A cell's named result feeds any cell that takes a parameter of the same name and type.**

That is the entire dependency mechanism. `arrivalRate` produces `lambda PerHour`; every cell with a `lambda PerHour` parameter consumes it. The edge is *name + type*, which has a sharp practical consequence the corpus reconfirmed repeatedly: renaming a result renames an edge, so a producer's result must be named exactly as its consumers read it. The analyzer catches a mismatch with a "did you mean" hint at build time; the type checker catches a type mismatch as an ordinary compile error.

Because the graph is a projection of the code, it *cannot drift from the code* — it **is** the code. This is why the live UI leads with the dependency graph and animates the propagation wave through it: watching which cells recompute, in what order, is the best debugger in the system, and no other notebook can show it, because in every other notebook the graph is incidental rather than the artifact.

## 3. The view is a readout of the code

A cell's output is drawn by **structural probe, not declaration.** The runtime reflects on the returned value: a value with a `Render() Rendered` method is drawn as its rich MIME-tagged content (SVG, HTML, Markdown); a scalar (or a named type over one, like `USD` over `float64`) falls back to a text readout; anything else is left hidden. Inputs are discovered the same way — a type carrying `Bounds()` renders as a ranged slider, one carrying `Options()` as a select, one with `Reconcile()` as a stateful widget — so **adding a new control kind means adding a capability probe, never editing a switch statement.**

### The degradation ladder

Every view has a graceful fallback. Lose the `Render()` method and a chart degrades to a text field showing the value; lose a `//notebook:` directive and a slider degrades to a plain number input. The principle: *losing the view costs polish, never correctness.* This is enforced, not aspirational — and it is the same principle that later made the composition system safe (a notebook with every layout directive stripped still renders correctly, in source order).

### Type or comment?

The load-bearing question is what decides a cell's role, and the answer is a hard rule: **whether something is an input is decided by its TYPE, never by a directive.** A `//notebook:slider` comment only refines how an already-input control *looks*; it can never make a non-input into one. (An earlier design had a `//notebook:nocache` directive deciding cacheability by comment; it was removed as a mistake, and the rule generalized — the most important structural properties are never a comment's to decide.)

## 4. Widgets are values; direct manipulation is a cell

A control is a Go value whose type carries an input capability. A `Multi[Theme]` is a multiselect; a `Range[Year]` is a two-handle slider; a `Table[Lot]` is an editable grid — each discovered structurally, each with its data-derived options/bounds computed by the cell body every wave while the head holds only the user's *selection*.

Direct manipulation — dragging a point on a chart — is the same mechanism, not a special case. A renderable value can draw **grips** (`data-grip="leaf:index"` handles) for a leaf it does not own; dragging one emits the whole point set through the same single mutation chokepoint a slider uses. The renderer reads, the runtime writes, and there is no cycle and no two-way binding. (A `leafToken` carrying which leaf a grip belongs to was nearly deleted as redundant with the widget's static `Kind` — until the corpus showed every grip is *cross-cell*, drawn by a cell that doesn't own its leaf, which `Kind` cannot identify. The token stays, justified structurally.)

Three rules forbid the framework from growing back: no plugin system, no config format, no component model, no SVG abstraction, no DSL, no second way to declare a cell. These are not stylistic preferences; they are the anti-goals that keep the notebook a Go package. ("No component model" is about *internals* — there is no framework for composing a notebook out of sub-components. It does not preclude the opposite direction: the whole notebook can be embedded in a foreign page as an externally-driven computational component through its one port, which §13 develops. The anti-goal forbids a component framework inside; it does not forbid the notebook from being a component outside.)

## 5. State: the one genuinely new concept

The reactive core is stateless — a cell is a pure function of its inputs, recomputed each wave — which is what makes scrubbing a slider *reversible*: drag `n` down and a converged distribution un-converges exactly, because the cell is a function of `n`, not a fold accumulating history. Dynamics that genuinely need history (an ODE integrator, a controller's step response) are expressed as **fixed-horizon pure cells**: loop to a fixed step count inside one cell and return the whole trajectory. Pure, WASM-able, and scrubbable both directions.

The one new concept the design admits is `Prev[T]` (a value read from the previous epoch) plus a `Tick` — the seam for true folds and timers. It is *cut but not built*: the analyzer branch and the `Delayed` parameter kind ship so every graph algorithm already accounts for it, but a notebook that actually uses a fold is reported as unsupported rather than silently mis-scheduled. This is the project's characteristic move — **cut the seam, skip the feature** — so the capability can be added additively later without touching the graph algorithms.

## 6. The runtime: two pieces

**The toolchain (compile time)** analyzes the package with `go/types`, derives the graph, and generates a tiny registry via `go build -overlay` — writing *nothing* into the user's source tree. **The engine (a library, linked in)** executes the graph: a scheduler runs a *wave* per edit against an **immutable per-epoch snapshot** of the single mutable state (the "head" of leaf values). That snapshot is the whole answer to glitch-freedom — the one correctness obligation the scheduler has: no cell may ever observe inputs from two different epochs. It is the single piece of "think hard" that survived every design round, because it is a real bug a user sees, not a purity concern, and it cannot be retrofitted onto a scheduler that reads shared mutable state — so the glitch-freedom test was written before the scheduler worked.

Two things genuinely don't work and are stated plainly: there is **no per-cell stdout** in the fanned-out scheduler (a `--serial` mode restores it for debugging), and **no goroutine parallelism in the browser tier** (Go's `js/wasm` is single-threaded; independent cells run serially in a tab, though native builds fan out across cores).

## 7. Where it runs: the same file, three ways

Because the engine emits events on a channel and imports no transport, the same compiled graph runs under three transports distinguished only by where the compiler points:

- **Interactive, in a browser:** compiled to `GOOS=js GOARCH=wasm`, the notebook is a self-contained WebAssembly artifact (~1 MB gzipped, ~40 ms cold to an interactive slider, ~300 µs slider→repaint) that runs with no server. Portability is a *derived* verdict, distinct from purity: a notebook is browser-portable iff its call graph touches no `net`, `os`, or cgo, and the toolchain decides that from the graph.
- **Headless, as a batch job:** `notebook build ./capacity && ./capacity --headless --json` prints a self-identifying `{provenance, values}` envelope — the source hash and git commit that produced the figure, alongside the results. The figure your pipeline emits and the number a human approves are one artifact.
- **Served over HTTP:** the same client, driven by Server-Sent Events, with a `/set` endpoint for edits.

The niche this serves is **systems, simulation, and cluster work** — capacity planning, benchmark harnesses, workload characterization — where the compute is code you'd write in Go anyway, the artifact needs to run as a job, and the thing you're modeling *is* infrastructure. Jupyter-on-HPC is conda environments and spawners; this is one file you `scp`.

## 8. Data that doesn't fit

Out-of-core data enters as a **content-addressed handle**, `Rel[T]`, that carries the source, row count, and a hash of the *contents* — not the rows. Change the file and the handle changes and everything downstream invalidates; a constant *path* could not (a real bug this prevents: a portfolio tracker that charted the wrong company because a constant path can't notice the file changed). *A path is not a handle.*

Queries stay **typed Go over `Rel[T]`** — `Scan`/`Filter`/`GroupBy` — rather than a checked SQL string, which is the counter-intuitive call: the obvious way to make a query safe is a compile-time-checked SQL dialect, and this project does not have one. It doesn't need one — rename a column and every cell that reads it fails to compile, the same guarantee, enforced by the compiler that already exists with no SQL parser, no dialect, and no cgo. A notebook that genuinely wants SQL calls DuckDB from a helper and pays the cgo cost *locally*; keeping it out of the toolchain is what keeps **the static binary intact and all four topologies clean**, which the whole `scp` + `sbatch` story depends on. (Full decision record: `docs/sql-decision.md`.)

## 9. Composition: presenting a notebook, not just running it

A notebook lays out in **source order** — the order functions happen to appear in the file — which is authoring order, almost never presentation order. The composition system lets an author arrange a notebook deliberately with two presentation-only, fully-optional directives:

- `//notebook:area=<name>` groups cells (and controls) into a named region, **by name, not source adjacency**.
- Package-level `//notebook:layout` directive lines name the arrangement in presentation order; within a row, `|` splits into **equal-flex columns**.

```go
//go:notebook
//notebook:layout intro
//notebook:layout controls | readouts
//notebook:layout curve
```

The whole system is bounded by one invariant — *strip every layout directive and the notebook renders correctly in source order* — and by a line it will not cross: the directives name **relationships and regions, never geometry** (no spans, weights, pixels, or nesting). That line is what separates "annotations that arrange" from a layout DSL (a named anti-goal); true geometry lives in the raw-HTML escape hatch, so one notebook can pay that cost without the toolchain becoming a layout engine. Arranged areas render as **cards** (bordered, equal-height panels), so a composed notebook reads as a designed dashboard rather than a linear document — inputs beside the chart they drive, a headline first.

The syntax itself was shaped by a finding: an indented, ASCII-art layout block is *reflowed and reordered by `gofmt`* (which CI enforces), because `gofmt` treats an indented comment as prose. A `//notebook:layout` line is a directive (`//word:`, no space) and survives verbatim. The block form would have failed on first commit; the per-row directive is the form the tooling permits. A `gofmt`-stability test now guards it.

## 10. Deferring design to HTML

Because a `Render()` method may emit `text/html`, a notebook can **author its presentation as HTML/CSS** when that is the better medium than a chart. This began as the escape hatch for WebGL (`surface`, `gpulife` — where Go has no form for the GPU and JS rides in on an image `onerror` handler, since an injected `<script>` will not run), and generalized into a design idiom: the answer to some questions is a *document*, not a plot.

- **invoice** renders a cloud pricing model as a styled HTML invoice — line items to a bold TOTAL DUE — because the answer is a receipt. The same file emits the same totals as a batch job.
- **simpson** renders Simpson's paradox as an HTML *table* where Treatment A wins both subgroup rows and the TOTAL row flips to B — a reveal a bar chart would flatten away.
- **punchcard** renders a 7×24 cluster-utilization heatmap as a CSS grid with native hover, no JavaScript.

In each, the seam stays honest exactly as the WebGL cases do: **Go owns the science** (pure cells, in the graph); **HTML owns the presentation** (the `Render()` string computes no result, only how to show one). The view is a projection of the cells, never a second source of truth.

## 11. The corpus as a falsification instrument

The 44-notebook corpus (38 live in the browser) is not a demo gallery; it is how the design is pressure-tested. Each notebook was built to put one mechanism on stage or to reproduce a real result, and the building is where the design is falsified. Two structural findings:

**The design held.** Nineteen notebooks across GPU compute, DFT, ODE integration, gradient descent, and cellular automata required **zero engine changes and zero violations of "a cell is a function."** The only flex needed was additive (the `area=` grouping directive). When the same cut — the **compute/view seam** — keeps drawing the line for six unrelated questions (widget config, registry keying, import surface, drag telemetry, log retention, cacheability), that is the sign the decomposition is right rather than locally tidy.

**Porting found real bugs in the originals.** The `lego` dataframe port multiplied dollars by an already-price-scaled column — `price²·factor`, silent — which typing the factor as its own unit turns into a compile error. Seam-carving's cache was patching a broken graph. The portfolio tracker's edge carried a stale token. Each bug was a *dimensional or dataflow* error the type system or the derived graph exposed once the notebook was expressed in this model.

## 12. Running is not passing

This is the finding the paper is really about, observed 12+ times across three codebases: **a system producing a thing is not the same as the thing reaching anyone.** Its upstream sibling — *a specification is a claim* — is the same error in prose: a mechanism described in a design doc but never cashed into code reads as done and is not.

The corpus kept re-proving it. Nearly every notebook had a bug that `go test` passed over, because the first test asserted a *mechanism* rather than a *consequence*: spectrogram's spectral-leakage metric was inverted relative to its own prose; nbody's first initial condition drifted in one jump rather than every step; a `Readout` value with no `Render()` method computed correctly and **reached no one**, left hidden by the client, invisible because no one screenshotted below the chart; consistent-hashing's first hash function clustered near-identical server names and faked 100% key churn, hiding the entire result the notebook exists to show. Each was caught only by *driving the real output* — sweeping a parameter, reading the rendered pixels, checking the invariant — not by a green suite.

Two disciplines came out of it, now written into the project's testing rules: **(1) assert the teaching claim before trusting the demo** — drive the effect you named, don't assert the shape you hoped for; and **(2) verify the instrument against a known-good before trusting a green** — a gate that goes green on a file it never built has told you nothing. If this system's paper has one transferable result, it is this discipline, not the notebook.

## 13. The notebook as a component: one port, in and out

The most recent milestone asks what a notebook's interface to the outside world *is*, and the answer is **one port**: you `set` a leaf (data in) and `subscribe` to a cell (data out) — the same two calls whether the counterparty is a human at a slider, a program over a socket, a foreign web page, or a batch job, over any transport. This is not a new mechanism; it is the naming of the write/subscribe seam the engine already had. A foreign host page, with the project's own UI *absent from the bundle*, can hold a single named object (`globalThis.notebook = {meta, set, subscribe, values, …}`) and drive the compute — observed by consequence, our default UI reduced to *one consumer of the port* rather than a privileged part of it. Bulk data-in crosses the same port as a content-addressed handle, no rebuild. A served notebook announces its address and readiness on stdout so any launcher can spawn, tunnel, and drive it — the notebook as an ephemeral compute service, the child announcing its port rather than making a parent guess.

## 14. What is unresolved, and the honest position

The project's original largest risk was retired during the build; what remains is engineering, not design.

**Retired: incremental static analysis.** Deriving the graph on every keystroke is only pleasant if re-derivation is cheap, and `go/types` is not incremental — this was the original largest risk. The session-based analyzer measures re-analysis after a one-cell edit at **~0.5 ms** (KC2), a ~200× margin over the 100 ms budget, so a headless `gopls` migration is not required at present corpus sizes. The one open question it leaves is behavior on substantially larger packages, not whether the approach works.

**Remaining: glitch-free propagation at scale** — solved in principle by the epoch'd snapshot, guarded by a sabotage test, but not yet stress-tested on a large, deep graph.

The standing *costs* are named, not hidden: no per-cell stdout under fan-out, and no goroutine parallelism in the browser tier. And one milestone (spore.host as the remote compute tier) is scoped on paper with a predicted zero-change diff, but *does not count as done* until observed against a real, billable spawn run — per §12.

**Why it holds together.** The same cut — compute versus view — kept drawing the line for unrelated questions (widget config, registry keying, import surface, drag telemetry, cacheability), which is usually the sign a decomposition is right rather than locally tidy. Fewer layers is not a gap to be filled; it is the point. The whole system is one new concept (`Prev` + `Tick`) and a small set of load-bearing rules: reconciliation is per-widget-kind, cacheability is derived not declared, purity and portability are distinct call-graph verdicts, the grip token is justified structurally, and schemas are checked by typed Go rather than a SQL parser — plus a testing discipline (§12) earned the hard way.

Everything else compounded from a single sentence.

> **A cell is a function.**

---

*Reproducibility: the corpus, the performance numbers, and the golden analyzer fixtures are all checked in CI. The design record, with the full derivation and the foreclosure table, is `docs/design.md`; the composition and notebook-as-service designs are `docs/composition.md` and `docs/notebook-as-service.md`.*
