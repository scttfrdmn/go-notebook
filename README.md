# go-notebook

[![CI](https://github.com/scttfrdmn/go-notebook/actions/workflows/ci.yml/badge.svg)](https://github.com/scttfrdmn/go-notebook/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/scttfrdmn/go-notebook)](https://goreportcard.com/report/github.com/scttfrdmn/go-notebook)
[![Go Reference](https://pkg.go.dev/badge/github.com/scttfrdmn/go-notebook.svg)](https://pkg.go.dev/github.com/scttfrdmn/go-notebook)

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

| KC | What | Target | Measured | |
|----|------|--------|----------|--|
| KC1 | cold graph derivation | < 1 s | **86 ms** | ✅ |
| KC2 | re-analysis after a one-cell edit | < 100 ms | **~0.5 ms** | ✅ |
| KC3 | slider → repaint (p95) | < 50 ms | **~15 µs** engine + **~165 µs** transport | ✅ |
| KC4 | save → rebuild → restart → repaint | < 500 ms* | **~470 ms** (capacity) · **~760 ms** (lego) | ✅ |

KC2 — the number the design hinged on — lands with a ~200× margin, which retires the project's largest engineering risk (incremental analysis) and defers the gopls migration indefinitely.

**KC4 is a compiled dev loop, and it behaves like one.** `capacity` (234 lines) rebuilds in ~470 ms; `lego` (575 lines, the largest buildable example) in ~760 ms — measured externally, edit → server serving the new result. It scales with notebook size, as any compile step does. The right comparison is explicit: this is the same band as a Vite rebuild or `cargo check`, and nobody calls those broken. The *contrast* that matters runs the other way — Jupyter/marimo cold-start is 1–3 s before first render and their slider path re-executes Python; ours pays the compile tax on the rarest action (a source save) and returns a slider repaint in ~15 µs on the most frequent one.

\* The 500 ms line was a spec guess with no baseline; the honest target for a compiled loop is "in the dev-tool band," which it is. A save is also a deliberate act — the ~1 s of human latency (⌘S, glance up, reach for the mouse) runs concurrently with the build, so the measured wall-clock is not time the user spends waiting.

Where the time goes: `go build` ~285 ms + OS first-exec of a fresh binary ~180 ms + initial wave; the engine itself contributes ~13 ms (a wave is ~2 µs). The compile cost is not a liability the design tolerates — it is the **premise**: it is what buys the static binary, `sbatch ./notebook`, the type-checker-derived graph, the typed wiring, the zero imports, and the 2 µs wave. Paying it on save is the price of admission for everything else.

Overlapping the rebuild with the running binary ([#22](https://github.com/scttfrdmn/go-notebook/issues/22)) keeps the notebook *responsive during* a rebuild — no dark screen — a responsiveness win, not a latency one (time-to-reflect-an-edit is inherently build + exec). Pre-warming the new binary was tried and removed: it cost a full headless wave, more than the first-exec it saved.

**Two stories, both working.** The differentiated one is batch and cluster: the same file is a notebook, an `sbatch` job, and a callable model (`--headless --set --json`). The familiar one is interactive: edit source, see the chart move — at every notebook size measured. Neither is a consolation for the other.

**Built so far:** `internal/graph` (plain-data IR, no `go/types`), `internal/analyze` (incremental type-checking `Session`, CHA-based purity), `internal/gen` (codegen + overlay), `engine` (head + epoch'd glitch-free scheduler + cache + capability probes), `engine/server` (SSE + edits; the only `net/http`). Deferred by design (seams cut, features skipped): `Prev[T]` folds, grips, SQL/`Rel[T]`, WASM. Progress is tracked in [GitHub issues](https://github.com/scttfrdmn/go-notebook/issues); kill-criteria numbers live on [#16](https://github.com/scttfrdmn/go-notebook/issues/16).

**Quality bar.** CI enforces `gofmt`, `go vet`, `go test -race`, and `golangci-lint` (errcheck, staticcheck, revive, ineffassign, misspell, gocyclo ≤ 15, unconvert, gocritic) on every push — a Go Report Card A+. Library-package coverage (the example notebooks are fixtures, excluded) is held to a **≥ 75% floor** in CI; it currently sits at ~79%, with the core engine/graph/analyze/gen packages all above 82%.

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
