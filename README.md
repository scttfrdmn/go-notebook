# go-notebook

**A reactive notebook where the notebook *is* an ordinary Go package.**

A cell is a top-level function with a doc comment. The dependency graph is a projection of the type checker's own def-use analysis. The result compiles to a single static binary — so a notebook is also a job:

```
go tool notebook run ./examples/capacity     # interactive
sbatch ./capacity                            # the same file, as a job
```

No `.ipynb`. No kernel. No spawner. No conda environment to reconstitute. **A notebook file contains no mention of this project** — no import, no framework, nothing but `//go:notebook` and Go.

```go
//go:notebook
package capacity

// Incoming jobs per hour.
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }

// Offered load in Erlangs.
func offeredLoad(lambda, mu PerHour) (a Erlangs) {
    return Erlangs(float64(lambda) / float64(mu))
}
```

The wiring rule, in one sentence:

> **A cell's named result feeds any cell that takes a parameter of the same name and type.**

---

## Status

**The core loop is built and the compile-first bet is measured.** The toolchain analyzes a notebook, derives the graph from `go/types`, generates a registry via `go build -overlay` (never touching your source tree), and runs it through a glitch-free reactive engine served to a browser — or headless as a batch job.

```
go tool notebook check ./examples/capacity     # print the dependency graph
go tool notebook run   ./examples/capacity     # serve it; edit source, it rebuilds
go tool notebook build ./examples/capacity     # emit a standalone binary
./capacity --headless --set servers=120 --json # the same file, as a job
```

The four kill criteria, measured on an M4 Pro (see the design's `docs/core-loop-spec.md` §7 for what each proves):

| KC | What | Target | Measured |
|----|------|--------|----------|
| KC1 | cold graph derivation | < 1s | **86 ms** |
| KC2 | re-analysis after a one-cell edit | < 100 ms | **~0.5 ms** |
| KC3 | slider → repaint (p95) | < 50 ms | **~15 µs** engine + **~165 µs** transport |
| KC4 | save → rebuild → restart → repaint | < 500 ms | **~470 ms** (capacity); marginal on larger notebooks |

KC2 — the number the design hinged on — lands with a ~200× margin, which retires the project's largest engineering risk (incremental analysis) and defers the gopls migration indefinitely. **KC4 is the honest caveat:** it passes for capacity-scale notebooks but is marginal-to-failing on the largest example (lego, ~510 ms–1.1 s), dominated by `go build` and OS binary-first-exec — both toolchain/OS, not the engine (a wave is ~2 µs). Overlapping the rebuild with the running binary is the identified path to real margin ([#22](https://github.com/scttfrdmn/go-notebook/issues/22)).

**Built so far:** `internal/graph` (plain-data IR, no `go/types`), `internal/analyze` (incremental type-checking `Session`, CHA-based purity), `internal/gen` (codegen + overlay), `engine` (head + epoch'd glitch-free scheduler + cache + capability probes), `engine/server` (SSE + edits; the only `net/http`). Deferred by design (seams cut, features skipped): `Prev[T]` folds, grips, SQL/`Rel[T]`, WASM. Progress is tracked in [GitHub issues](https://github.com/scttfrdmn/go-notebook/issues); kill-criteria numbers live on [#16](https://github.com/scttfrdmn/go-notebook/issues/16).

## Documents

| | |
|---|---|
| [`docs/design.md`](docs/design.md) | The design. Start here. |
| [`docs/core-loop-spec.md`](docs/core-loop-spec.md) | Buildable first milestone. Repo layout, interfaces, foreclosure table, kill criteria. |
| [`docs/kickoff.md`](docs/kickoff.md) | Handoff prompt for Claude Code. |

---

## The notebooks

Each was written to stress one thing. Together they're the evidence the design has, and the corrections it took.

| Notebook | What it tests | What it found |
|---|---|---|
| **`capacity`** | The baseline. M/M/c fleet model. | The reference fixture — the smallest file that exercises typed wiring, semantic types, and a non-trivial DAG. |
| **`lego`** *(port)* | Dataframe dashboard. Data-derived widget options, bounds computed from data, stringly-typed axis dropdowns. | **Bug:** the original multiplies price × an already-price-scaled "inflation" column — dollars × dollars, silently `price²·factor`. Typing the factor as `Factor` makes it a compile error. Forced the rule *a cell may return a widget*. |
| **`seam`** *(port)* | Expensive compute. Where memoization stops being an optimization. | **Bug:** `find_seam` discards the DP table and picks the seam start with `argmin(backtrack[-1])`, which is column 0 in 200/200 random cases — the original never does minimum-energy seam selection. **And:** its `@mo.cache` was patching a broken graph. Seam *order* doesn't depend on the slider; hoist it and no cache is needed at all. |
| **`curvefit`** | **Falsification test.** A leaf whose value *is* the data, edited by dragging on the output it produces. Should be a cycle. | It isn't: *the renderer reads the leaf, the runtime writes it, a write is not an edge.* Generalized Lego's brush into grips. **Correction:** the reconcile rule is per-widget-kind, not universal. |
| **`queue`** | Timers. The first non-human writer to the head. | Forced the design's **one new concept**: `Prev[T]` + `Tick`. A fold steps on the clock, *not* when any other input changes. Randomness became reproducible for free — the PRNG state is a field. |
| **`bayes`** *(port)* | Incremental compute. Is "posterior after n points" a fold? | **No — and using a fold would break it.** Sufficient statistics are sums, so `posterior` is pure and you can scrub *backward*. Gave the rule: *relative gestures accumulate; absolute controls recompute.* |
| **`portfolio`** *(port)* | Side effects, caching, financial units. | **Bug:** `yf.Ticker("MSFT")` is hardcoded — every ticker downloads Microsoft's history, relabeled, and never re-downloads. The *enabling* flaw: the graph edge carries `parent_folder`, a constant. **Rule:** *a path is not a handle; a handle identifies its contents.* |
| **`mandelbrot`** *(port)* | The rigged fight, made honest: strong scaling instead of Go-vs-Python. | **Correction:** I invented a `//notebook:nocache` directive and it was wrong — cacheability is derivable from the call graph. A button turned out to be the same `Tick` as a timer, with a different writer. |
| **`taxi`** | **SQL + out-of-core.** 42M rows; a query cell must return rows *of some Go type*. | The struct **is** the schema, so SQL typechecks at build time: rename a column and every SQL cell fails to compile. **The wound:** DuckDB is cgo, which costs the static-binary story for SQL notebooks. |

---

## What it cost

One new concept (`Prev[T]` + `Tick`). Three corrections the ports forced. One genuine wound (cgo, for SQL). No per-cell stdout — the price of goroutine fan-out.

Everything else compounded from a single sentence.

> **A cell is a function.**

---

## License

Apache 2.0.
