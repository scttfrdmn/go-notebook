# Build & run

*The three toolchain verbs, the flags a built binary understands, and the one file / three topologies idea: the same notebook is an interactive browser app, a headless batch job, and a served page — distinguished only by where you point the compiler.*

## The three verbs

The toolchain is `go tool notebook` (after `go get -tool …`) or `go run github.com/scttfrdmn/go-notebook/cmd/notebook`:

```bash
go tool notebook check .    # analyze — print the dependency graph, report errors
go tool notebook run   .    # build and serve in a browser; edit the source, it rebuilds
go tool notebook build .    # compile a standalone binary
```

### `check`

Analyzes the notebook and prints the derived dependency graph — every cell, its inputs (with the producer each wires to), and its output. It is how you see the graph without running anything, and how wiring errors surface with a pointed message. `--timing` prints the graph-derivation wall time.

### `run`

Builds the notebook and serves it in your browser over HTTP, rebuilding on source edits. Flags: `--addr` (listen address, default `127.0.0.1:8080`), `--no-open` (don't launch a browser), `--timing`.

### `build`

Compiles the notebook. Flags:

| Flag | Meaning |
|------|---------|
| `-o <path>` | output path (a binary, or a directory for `--target=wasm`) |
| `-target native\|wasm` | native binary (default) or a self-contained WASM host directory |
| `--showcase` | (wasm) lead with the dependency graph open — for gallery demos |
| `--timing` | print codegen + compile wall time |

## The built binary's flags

A native binary you built is itself runnable, with its own flags:

```bash
./tempconv                          # serve it (like `run`, but the standalone binary)
./tempconv --headless --json        # run once, print final cell values as JSON, exit
./tempconv --headless --set c=100 --json   # override an input by its RESULT name
```

| Flag | Meaning |
|------|---------|
| `--headless` | run once and exit, no server |
| `--json` | print final cell values as JSON (implies `--headless`) |
| `--set leaf=value` | override an input by its result name; repeatable |
| `--addr` | listen address when serving |
| `--head <file>` | where slider positions persist between runs |

`--set` names the input by its **result name** — the same name that is the edge in the graph. `--set c=100` sets the cell whose result is `c`.

## One file, three topologies

The same notebook file compiles three ways, differing only in the compiler target:

- **Interactive (WASM):** `build -target=wasm` → a directory you serve over HTTP; the notebook runs entirely client-side, no server.
- **Headless (batch):** the native binary with `--headless --json` → run once, emit values; `scp` it to a cluster and `sbatch` it.
- **Served (HTTP):** `run`, or the native binary serving → a live page backed by the process.

## The WASM portability gate

A notebook compiles to the browser only if **no cell's call graph reaches `net`, `os`, or cgo** — there is no browser equivalent. The toolchain derives this from the graph; you do not annotate it. `build -target=wasm` refuses a notebook that isn't portable and names the offending cells.

A common false positive: a cell that calls `fmt` on a number is flagged, because `fmt` transitively reaches `os`. The fix is to keep `fmt` in a `Render()` method (not a cell) — see [rendering](reference-rendering.html) — or use `strconv` in the cell body.

## Provenance

Every headless run and every built page carries a provenance record — see [provenance](reference-provenance.html) — so a figure can be traced to the exact source and toolchain that produced it.
