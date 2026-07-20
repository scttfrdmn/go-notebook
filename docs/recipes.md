# Recipes

*The cookbook: each recipe demonstrates **exactly one mechanism** in the smallest
correct form, meant to be copied. This is the "copy the smallest correct pattern"
layer beneath the [corpus](https://go-notebook.dev/#corpus) — where the corpus
shows what the system can do, these show how one piece works.*

Every recipe runs the same way:

```bash
go tool notebook run ./examples/minimal/<name>     # interactive, in a browser
go tool notebook check ./examples/minimal/<name>   # print the dependency graph
```

Read them roughly top-to-bottom; each builds on the last. Every recipe links to
its complete source and to the reference page for the mechanism it isolates. Most
are 15–65 lines; a few (`draggable`, `sales-analysis`, `wrap-existing-package`)
are longer because they include a full rendering, an end-to-end composition, or a
wrapped API surface — the line count is noted so you know which is a one-mechanism
recipe and which is a worked example.

## Inputs — how a value becomes a control

| Recipe | Mechanism | Lines | Reference |
|--------|-----------|-------|-----------|
| [`hello`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/hello) | the whole model: an input cell → a derived cell, wired by name+type | 20 | [quickstart](quickstart.html) |
| [`slider`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/slider) | a scalar input; the `//notebook:slider` directive refines the control | 19 | [controls](reference-controls.html) |
| [`checkbox`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/checkbox) | a `bool` input → a checkbox | 24 | [controls](reference-controls.html) |
| [`textinput`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/textinput) | a `string` input → a text box | 15 | [controls](reference-controls.html) |
| [`selectbox`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/selectbox) | `Options()` + scalar `Value` → a select; `Reconcile` keeps state | 45 | [controls](reference-controls.html) |
| [`multiselect`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/multiselect) | `Options()` + slice `Value` → a multi | 42 | [controls](reference-controls.html) |
| [`rangecontrol`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/rangecontrol) | `Bounds()` → a two-handled range control | 43 | [controls](reference-controls.html) |
| [`draggable`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/draggable) | a slice `Value` + `Grip()` → points you drag on a chart *(worked example)* | 135 | [controls](reference-controls.html) |
| [`table`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/table) | a slice-of-struct `Value` → an editable grid | 65 | [controls](reference-controls.html) |

## Outputs — how a value is drawn

| Recipe | Mechanism | Lines | Reference |
|--------|-----------|-------|-----------|
| [`htmlcard`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/htmlcard) | a `text/html` `Render()` — the gentlest rich output; uses the optional `nb` package | 45 | [rendering](reference-rendering.html) |
| [`svgchart`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/svgchart) | an `image/svg+xml` `Render()` — the low-level, zero-import route | 44 | [rendering](reference-rendering.html) |
| [`sales-analysis`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/sales-analysis) | normal analysis end-to-end: parse → filter → summarize → `nb/chart` Table + Bar, no dataframe *(worked example)* | 206 | [charts](reference-charts.html) |

## Getting data in

| Recipe | Mechanism | Lines | Reference |
|--------|-----------|-------|-----------|
| [`embedded-data`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/embedded-data) | `go:embed` a dataset (compile-time → still WASM-able), parse with `strings` | 74 | [charts](reference-charts.html) |
| [`csv-native`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/csv-native) | `os.Open` + `encoding/csv` with honest `(rows, error)`; native-only | 125 | [charts](reference-charts.html) |
| [`file-and-artifact`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/file-and-artifact) | both I/O seams outside the cells: `go:embed` in (WASM-able), a sibling program that imports the cells and writes a `.svg` out | 119 + writer | [build & run](reference-build-run.html) |

## Graph shape and behavior

| Recipe | Mechanism | Lines | Reference |
|--------|-----------|-------|-----------|
| [`fanout`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/fanout) | one result feeding many cells (a forking graph) | 23 | [design](design.html) |
| [`fanin`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/fanin) | many results feeding one cell (a joining graph) | 22 | [design](design.html) |
| [`errorcell`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/errorcell) | a `(value, error)` cell, partial failure, blocked-upstream | 50 | [design](design.html) |
| [`cancel`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/cancel) | `context.Context` injection, cancellable recompute | 40 | [design](design.html) |
| [`multi-file-package`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/multi-file-package) | a notebook spanning several `.go` files: only one carries `//go:notebook`, but cells use types/methods/helpers from the sibling files — it is an ordinary Go package | 3 files | [design](design.html) |
| [`wrap-existing-package`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/wrap-existing-package) | a thin reactive view over a mature Go API: cells just call the existing package (here stdlib `regexp`) — you wrap your code, you don't rewrite it | 195 | [design](design.html) |

## Build, run, feed, test

| Recipe | Mechanism | Lines | Reference |
|--------|-----------|-------|-----------|
| [`headless`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/headless) | the same file as an interactive app and a `--headless --set --json` batch job | 50 | [build & run](reference-build-run.html) |
| [`setport`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/setport) | the minimal live feed: a driver POSTs one leaf, a pure cell reacts | 23 + driver | [ports](reference-ports.html) |
| [`testing`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/testing) | cells tested as ordinary Go functions (`go test`) | 52 | — |

## Two things every recipe shows

- **No wiring.** Nothing in these files registers a cell, connects an edge, or
  imports a reactive framework. The graph is derived from the function signatures
  by the Go type checker.
- **Nothing is required to import.** Most recipes import nothing from this project.
  `htmlcard` shows the *optional* [`nb`](https://github.com/scttfrdmn/go-notebook/tree/main/nb)
  convenience package (constructors + compile-time interface checks); `svgchart`
  and `table` show the equivalent zero-import track (a locally-declared `Rendered`
  struct). Pick either — see [rendering](reference-rendering.html#two-ways-to-write-it--same-contract).
