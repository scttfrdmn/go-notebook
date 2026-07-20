# Examples

Two layers, two jobs.

- **[`minimal/`](minimal) — learn by copying.** Single-mechanism notebooks meant
  to be copied into your own project — most under ~75 lines, a few worked
  examples (a draggable plot, wrapping an existing package) longer. Start here if
  you are building your first real notebook.
- **The corpus (this directory) — see what it can do.** Larger notebooks that
  each put one design property under pressure — direct manipulation, linked
  views, typed units, document rendering, live feeds, native fan-out. 38 of them
  run live in the browser at [go-notebook.dev](https://go-notebook.dev).

New here? Read these five first, in order:

| # | Example | What it teaches | Run |
|---|---------|-----------------|-----|
| 1 | [`minimal/hello`](minimal/hello) | the whole model on one screen: input → derived, wired by name+type | `go tool notebook run ./examples/minimal/hello` |
| 2 | [`tempconv`](tempconv) | a complete notebook end-to-end: controls, an SVG view, a test | `go tool notebook run ./examples/tempconv` |
| 3 | [`minimal/htmlcard`](minimal/htmlcard) | rich output the easy way (`text/html`), and the optional `nb` package | `go tool notebook run ./examples/minimal/htmlcard` |
| 4 | [`minimal/headless`](minimal/headless) | the differentiator: the same file as a browser app *and* a `--headless --json` batch job | `go tool notebook run ./examples/minimal/headless` |
| 5 | [`capacity`](capacity) | a real interactive planning tool that is also a schedulable job | `go tool notebook run ./examples/capacity` |

Then browse [`minimal/`](minimal/README.md) for a copy-paste recipe per
mechanism, or the corpus below for fuller demonstrations.

## The corpus by theme

Every corpus notebook is an ordinary Go package — read it as Go. Those marked
**live** run in the browser (their call graph touches no `net`/`os`/cgo, so they
compile to WASM); the rest are the same file built as a native binary.

**Controls and interaction** — `curvefit` (draggable points), `anscombe`,
`boundary`, `fourier`, `kmeans`.

**Linked views** — `lotka`, `spectrogram`, `nbody` (an invariant exposes
"running is not passing").

**Document output (HTML)** — `invoice`, `simpson` (a table is the right medium
for Simpson's paradox), `punchcard`.

**Systems and HPC** — `critpath`, `backfill`, `roofline`, `amdahl`, `fattree`,
`cachepolicy`, `latencybw`, `aimd`, `bdp`, `reliability`, `retrystorm`, `fleet`.

**Advanced browser integration** — `surface` (a raw HTML/JS escape hatch),
`gpulife` (WebGPU for browser-side parallelism), `turing` (native fan-out vs.
single-threaded browser execution).

**Live feeds** — `sensorfeed`, `homefeed`, `tickerfeed`, `apifeed` (each is a
pure notebook plus a `driver/` that pushes data through the set port; see
[docs/live-feeds.md](../docs/live-feeds.md)).

**Native or heavyweight** — `lego`, `seam`, `portfolio`, `taxi` (DuckDB via
cgo), `queue`.

**Numerical and statistical** — `bayes` (reversible recomputation), `clt`,
`simpson`, `curvefit`, `mandelbrot`, `percolation`, `pid`, `little`
(dimensional correctness through Go types), `consistenthash`.

For the full argument the corpus makes — and the design findings that came out
of building it — see [the paper](../docs/paper.md).
