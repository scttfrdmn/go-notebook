# Claude Code — Kickoff

*Paste this as the first message. Put `reactive-notebook-go.md` and `core-loop-spec.md` in the repo root first.*

---

We're building `go-notebook`: a reactive notebook where the notebook **is** an ordinary Go package. A cell is a top-level function with a doc comment; the dependency graph is derived from the type checker; the whole thing compiles to a single binary.

Read `reactive-notebook-go.md` (the design) and `core-loop-spec.md` (the build spec) before writing any code. The spec is detailed and considered — treat it as the contract.

## What this first milestone is actually for

**It exists to produce two numbers, not a demo.**

- **KC2** — re-analysis after a one-cell edit: target **< 100 ms**
- **KC4** — full loop (save cell body → rebuild → restart → head restored → repainted): target **< 500 ms**

If those land, the whole design is alive. If KC4 is > 2 s, the compile-first bet fails at the interactive tier and we change the pitch rather than paper over it. So: **instrument these from the moment they're measurable, and report them.** Don't polish the UI before we know the loop is viable.

## Order of work

Follow the spec's milestones. Do not skip ahead.

1. **M0** — module (`go 1.24`), layout per spec §2, CI: `go vet`, `go test -race`, `golangci-lint`.
2. **`examples/capacity/capacity.go`** — copy verbatim from the design doc. It's the fixture for everything.
3. **M1** — `internal/graph` **first**, with tests, *before any `go/types` code*. It's plain data; it should be table-driven tests with no fixtures. Then `internal/analyze` populates it. Ship `go tool notebook check` — the first thing a human can run.
4. **M2** — codegen + `go build -overlay`.
5. **M3** — engine: head, snapshot, epochs, scheduler, cache. **Report KC2 here.**
6. **M4** — server + minimal UI. **Report KC3 and KC4 here.**
7. **M5** — `--headless --set --json`.

**Stop and check in after M1 and after M3.** Don't run the whole spec end to end unattended.

## Hard constraints — violating these is a rewrite, not a refactor

These are in the spec but they're the ones that get quietly broken, so they're repeated here:

- **`internal/graph` must not import `go/types`.** The IR is plain data. This is what makes the future gopls swap additive.
- **`engine` must not import `net/http`.** It emits `Event`s on a channel; `engine/server` subscribes. This is what makes headless/WASM/batch free.
- **The scheduler reads only from an immutable head snapshot, per epoch.** Glitch-freedom cannot be retrofitted. Write the glitch test (spec §8) before the scheduler works.
- **Every leaf write goes through one `Head.Set(leaf, value)`.** Don't scatter writes. Timers, buttons, and grips are all just callers of this later.
- **Cells have multiple named outputs.** Do not assume one result per cell.
- **`ParamKind` ships with a `Delayed` member that nothing produces yet**, and the cycle checker already skips it. Five lines now; touches every graph algorithm later.
- **Widget discovery is capability probing, never a type switch.**
- **Codegen writes nothing into the user's source tree.** `go build -overlay` only.
- **Purity is derived from the call graph, never declared.** No `//notebook:nocache` directive — that was explicitly rejected.

## Anti-goals

Do not add: a plugin system, a config file format, an abstraction layer over SVG, a component model, a DSL, a second way to declare a cell, or a brand/logo/import in the notebook file. **A notebook file must contain zero mention of this project** — no import, no marker beyond `//go:notebook`. That property is the whole point; protect it.

Do not implement anything in the spec's deferred list (`Prev[T]`, timers, grips, SQL, WASM, hot-reload beyond restart). The spec's §3 foreclosure table says what seam each one needs. Cut the seam; skip the feature.

## Quality bar

This is a real project, not a prototype. Godoc every exported symbol in `engine` (generated code depends on it — treat its API as versioned from commit one). Wrap errors with `%w`. No panics across package boundaries (`recover` only in the scheduler's node runner). Contexts threaded, no `context.TODO` in shipped paths. `go test -race` clean — the parallel fan-out is the design's headline dividend and a data race in it would be embarrassing.

**Diagnostic message quality is a feature, and it's tested like one.** Golden tests over expected error text. The difference between a tool and a toy is:

```
capacity.go:31:19: cell "utilization" needs `a Erlangs`, but no cell produces it.
                   Did you mean `offeredLoad`, which produces `a Erlangs`? (capacity.go:26)
```

## If you disagree with the spec

Say so, and stop. The design survived six ported notebooks and three real corrections, so most of it is load-bearing in ways that aren't locally obvious — but it's also not scripture, and a couple of things in it have never been compiled. If something is wrong, unbuildable, or there's a materially better approach, **raise it before implementing it.** Silent deviation is the one failure mode I can't recover from.

Start with M0 and M1. Show me `go tool notebook check examples/capacity` printing the correct graph.
