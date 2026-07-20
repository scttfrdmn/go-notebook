# Minimal examples — the cookbook

Each directory here demonstrates **exactly one mechanism** in the smallest
correct form, meant to be *copied*. They are not in the homepage gallery — the
[full corpus](../README.md) is the "see what it can do" showcase; this is the
"copy the smallest correct pattern" layer beneath it.

Every example runs the same way:

```bash
go tool notebook run ./examples/minimal/<name>     # interactive, in a browser
go tool notebook check ./examples/minimal/<name>   # print the dependency graph
```

Read them roughly top-to-bottom; each builds on the last.

## Inputs — how a value becomes a control

| Example | Mechanism | Lines | Reference |
|---------|-----------|-------|-----------|
| [`hello`](hello) | the whole model: an input cell → a derived cell, wired by name+type | 20 | [quickstart](https://go-notebook.dev/docs/quickstart.html) |
| [`slider`](slider) | a scalar input; the `//notebook:slider` directive refines the control | 19 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`checkbox`](checkbox) | a `bool` input → a checkbox | 24 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`textinput`](textinput) | a `string` input → a text box | 15 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`selectbox`](selectbox) | `Options()` + scalar `Value` → a select; `Reconcile` keeps state | 45 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`multiselect`](multiselect) | `Options()` + slice `Value` → a multi | 42 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`rangecontrol`](rangecontrol) | `Bounds()` → a two-handled range control | 43 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`draggable`](draggable) | a slice `Value` + `Grip()` → points you drag on a chart | 135 | [controls](https://go-notebook.dev/docs/reference-controls.html) |
| [`table`](table) | a slice-of-struct `Value` → an editable grid | 65 | [controls](https://go-notebook.dev/docs/reference-controls.html) |

## Outputs — how a value is drawn

| Example | Mechanism | Lines | Reference |
|---------|-----------|-------|-----------|
| [`htmlcard`](htmlcard) | a `text/html` `Render()` — the gentlest rich output; uses the optional `nb` package | 45 | [rendering](https://go-notebook.dev/docs/reference-rendering.html) |
| [`svgchart`](svgchart) | an `image/svg+xml` `Render()` — the low-level, zero-import route | 44 | [rendering](https://go-notebook.dev/docs/reference-rendering.html) |
| [`sales-analysis`](sales-analysis) | normal analysis end-to-end: parse → filter → summarize → `nb/chart` Table + Bar, no dataframe *(worked example)* | 206 | [charts](https://go-notebook.dev/docs/reference-charts.html) |

## Getting data in

| Example | Mechanism | Lines | Reference |
|---------|-----------|-------|-----------|
| [`embedded-data`](embedded-data) | `go:embed` a dataset (compile-time → still WASM-able), parse with `strings` | 74 | [charts](https://go-notebook.dev/docs/reference-charts.html) |
| [`csv-native`](csv-native) | `os.Open` + `encoding/csv` with honest `(rows, error)`; native-only (the gate refuses `os`) | 125 | [charts](https://go-notebook.dev/docs/reference-charts.html) |
| [`file-and-artifact`](file-and-artifact) | both I/O seams outside the cells: `go:embed` in (WASM-able), a sibling program that imports the cells and writes a `.svg` out | 119 + writer | [build & run](https://go-notebook.dev/docs/reference-build-run.html) |

## Graph shape and behavior

| Example | Mechanism | Lines | Reference |
|---------|-----------|-------|-----------|
| [`fanout`](fanout) | one result feeding many cells (a forking graph) | 23 | [design](https://go-notebook.dev/docs/design.html) |
| [`fanin`](fanin) | many results feeding one cell (a joining graph) | 22 | [design](https://go-notebook.dev/docs/design.html) |
| [`errorcell`](errorcell) | a `(value, error)` cell, partial failure, blocked-upstream | 50 | [design](https://go-notebook.dev/docs/design.html) |
| [`cancel`](cancel) | `context.Context` injection, cancellable recompute | 40 | [design](https://go-notebook.dev/docs/design.html) |

## Build, run, feed, test

| Example | Mechanism | Lines | Reference |
|---------|-----------|-------|-----------|
| [`headless`](headless) | the same file as an interactive app and a `--headless --set --json` batch job | 50 | [build & run](https://go-notebook.dev/docs/reference-build-run.html) |
| [`setport`](setport) | the minimal live feed: a driver POSTs one leaf, a pure cell reacts | 23 + driver | [live feeds](https://go-notebook.dev/docs/live-feeds.html) |
| [`testing`](testing) | cells tested as ordinary Go functions (`go test`) | 52 | — |

## Two things every example shows

- **No wiring.** Nothing in these files registers a cell, connects an edge, or
  imports a reactive framework. The graph is derived from the function
  signatures by the Go type checker.
- **Nothing is required to import.** Most examples import nothing from this
  project. `htmlcard` shows the *optional* [`nb`](../../nb) convenience package
  (constructors + compile-time interface checks); `svgchart` and `table` show the
  equivalent zero-import track (a locally-declared `Rendered` struct). Pick either.
