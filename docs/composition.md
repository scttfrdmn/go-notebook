# Design doc — composing a notebook (presentation ≠ source order)

*Status: **DESIGNED — Tier A, decided** (Scott + Claude, 2026-07-18). The decisions are at the top; the framing that led to them follows. Not built yet — this is the spec to build from.*

---

## Decision

A notebook can be arranged deliberately with three presentation-only additions, all optional, all degrade-to-linear:

1. **`//notebook:area=<name>`** on any cell (including a leaf/control) assigns it to a named region, grouped **by name, not source adjacency** — `area=panels` collects every `panels` cell wherever it sits. Replaces `//notebook:row=` (7 users migrate; a "row" is an area the layout lays horizontally). One grouping model, reorder-safe. This rides the existing per-cell `directives(fn)` → `CellMeta.Directives` rail unchanged.

2. **Package-level layout, one `//notebook:layout` directive per row** — NOT a multi-line block. Each row is its own directive line, listing area-or-cell tokens split by `|`:
   ```go
   //go:notebook
   //notebook:layout controls | curve
   //notebook:layout readouts
   ```
   reads as "row 1: `controls` beside `curve`; row 2: `readouts` full width." **This form is forced by gofmt** (see "The gofmt constraint" below): an indented ASCII-art block is reflowed and reordered by gofmt — which CI enforces — so it is not viable; a per-row `//notebook:` directive is preserved verbatim. The rows are contiguous on the `//go:notebook` file, so the arrangement is still read in one place.

   **Token resolution:** each token is matched as an **area name first, then a cell ID** — so a lone cell needs no `area=` wrapper (`//notebook:layout curve` places the `curve` cell directly), while `controls` matches the area grouping the sliders. Defined order, so no real collision.

   **Grammar (deliberately tiny):** rows are directive lines; `|` splits a row into **equal-flex columns** (wrapping to stacked when narrow, as `cellrow` does today). **No weights, no spans, no nesting** — it names *relationships and regions, never geometry*. That is the line between "annotations that arrange" and a layout DSL.

3. **Controls place into areas.** A leaf carrying `area=inputs` renders its control in that region instead of the top `#controls` block — so a slider sits beside the chart it drives. Highest-value piece, most invasive client change (`buildControlsAndCells` stops assuming one control block).

**The invariant (non-negotiable):** strip every layout directive and the notebook renders correctly in source order. Layout is presentation-only — never changes what a cell computes, what feeds what, or whether it runs, only where output is drawn. It carries no Go types and never touches the graph.

**The gofmt constraint (the finding that shaped the syntax).** Verified empirically: gofmt (CI-enforced on every file) treats an indented, non-directive comment as *prose* and reflows it — an ASCII-art layout block gets its lines reordered to the top of the file and its indentation stripped, destroying the grammar. gofmt preserves a line verbatim **only** when it is a directive: `//word:` with no space after `//`. Hence one `//notebook:layout` directive per row. Confirmed stable, and the rows land in `f.Doc.List` in order — a clean file-level parse site.

**Two mechanism costs, named (the doc earlier undersold these):**
- The package-level layout needs its **own file-level parser**. The existing `directives()` builds a flat `map[string]string` from space-split `key=val` tokens; it would shred `layout controls | curve` into meaningless bare tokens. Per-cell `area=` reuses that parser; the ordered multi-token rows do not. Bounded new work, not free reuse.
- Layout crosses the wire as a **sibling of `NotebookMeta`, not a field inside `[]CellMeta`** (it is not per-cell). A generated `NotebookLayout` var passed exactly as `NotebookProvenance` already is — surfaced as `opts.layout` to `NB.init` (whose opts bag has room) and `nb.layout` on the wasm port. `meta` stays `[]CellMeta`, so batch/headless is untouched. This mirrors the proven provenance-add pattern; the client is **address-by-ID** everywhere (`cellEls[ev.cell]`), so DOM placement is decoupled from event routing — reordering is a contained change to the build step, not a rearchitecture.

**Resolved edge cases:**
- **Unplaced cells** append below the layout in source order — the common case (most notebooks place 2–3 things and let the rest flow), never a silent drop.
- **Order within an area** is source order; finer control is a smell — split into more areas.
- **The dependency graph** (built from edges) is unaffected, stays where it is; placeable-graph is a possible future hook, out of scope.
- **A 2×2** (anscombe) is two rows of two — `plot1 | plot2` / `plot3 | plot4` — flat, no nesting.

**Out of scope (the escape hatch covers it):** true geometry — spans, tracks, pixel widths, nested trees. A notebook that needs it uses the raw-HTML/JS escape hatch (`surface`, `gpulife`); one notebook pays, the toolchain never becomes a layout engine.

---

## The problem

A notebook lays its cells out in **source order**: the order functions happen to appear in the file. That is authoring order — where a function was convenient to write — and it is almost never presentation order. You put inputs at the top because that's tidy Go; you write helpers between cells; a headline number lands wherever its function did. The reader gets the file's structure, not the argument you want to make.

Today the entire composition vocabulary is **one directive**: `//notebook:row=<name>` lays *consecutive* same-named cells side by side (7 notebooks use it). Everything else stacks top-to-bottom in `g.Order` (source order), and every control piles into one `#controls` block at the top, divorced from the cell it drives. There is no way to say "inputs beside this chart," "present summary first," or "these three are a row."

## The tension (why this is design work, not a feature)

Two hard constraints bound the solution, and naming them is half the design:

1. **A notebook is a plain Go package; nothing is imported; a cell is a function.** Layout cannot be a config file, a `.layout` sibling, or an imported builder. The only vocabulary the project has for presentation is the `//notebook:` directive — presentation-only doc comments the analyzer flattens into `CellMeta.Directives`, which the client reads. Any composition vocabulary rides that rail.

2. **Named anti-goals: "no layout DSL," "no component model"** (`hard-constraints`, spec §3 foreclosure table). "Designed placement" must stay declarative annotations on cells — not a component tree, not a templating language, not a second file describing the UI. The moment an author writes layout *instead of* cells, the line is crossed.

The test that keeps this honest is the **degrade-to-linear invariant** above: if a notebook is *unreadable* without its layout directives, layout became load-bearing and we failed. It is the same principle the controls already follow ("losing the view costs polish, never correctness," design.md).

## Why each decision went the way it did

- **Name-based `area=` replaces adjacency-based `row=`.** `row=` works today only because you *write* row-mates adjacently. Add presentation reordering and "consecutive same-named row" stops being stable — order and grouping would be two mechanisms fighting. Grouping by name (collect all `area=X` wherever they sit) is reorder-safe and leaves one model. Cost: migrate 7 notebooks (and their prose — lotka *explains* its `row=panels` in the doc comment, so the narrative updates too).

- **Package-level `layout` block, not per-cell `order=`.** Per-cell ordering (`order=N` or `after=X`) needs no new parse site (directives are per-function today), but it **scatters the arrangement across N cells — recreating the exact "you can't see the composition" problem we're fixing**, and `order=N` integers are brittle to insertion (the BASIC-line-number problem). A package-level block costs one new parse site (package-doc `//notebook:` parsing doesn't exist yet — `directives()` reads `fn.Doc` only), but it is the only option where the arrangement is legible in one place and reads like the intent. One bounded new mechanism beats a permanent legibility loss.

- **Equal-flex, no weights.** Weights (`controls | chart chart` = 1:2) are geometry — the first step onto the layout-DSL slope. A genuinely wide chart uses per-area sizing hints (`height=` already exists) or the escape hatch. The block stays purely relational.

- **Flat, no nesting.** A tree of regions *is* a component model by another name. Every arrangement the corpus needs (2×2, inputs-beside-chart, headline-over-grid) is expressible as flat rows of areas.

- **Controls into areas, in v1.** "A slider beside its chart" is the change authors would feel most (inputs next to effects). It's the most invasive client change, but deferring it leaves the highest-value piece on the table, so it ships in v1.

## Worked examples

**lotka** — one trajectory, two linked views, a phase-plane grip input. Today: controls block on top, `series`/`portrait` side by side via `row=panels`, in source order.
```go
//go:notebook
//notebook:layout controls | portrait
//notebook:layout series
```
Inputs and the draggable phase portrait at the top; the time series full-width below. (`series`/`portrait` become `area=`; the doc comment's `row=panels` explanation updates to the area/layout model.)

**capacity** — five sliders, a cost/latency curve, scalar readouts.
```go
//go:notebook
//notebook:layout controls | readouts
//notebook:layout curve
```
Knobs beside the numbers they produce; the big chart full-width under. Today: five sliders stacked at top, chart, then readouts wherever their functions landed.

**anscombe** — four scatter plots whose entire point is *simultaneous* comparison.
```go
//go:notebook
//notebook:layout plot1 | plot2
//notebook:layout plot3 | plot4
//notebook:layout stats
```
A 2×2 of the plots with the shared statistics readout below — flat, no nesting. Source-order stacking actively fights the argument this notebook makes.

## Build shape (the seam, not the code)

- **analyzer:** collect the ordered `//notebook:layout` directive lines from the `//go:notebook` file's `f.Doc.List` (a new file-level parser — the per-function `directives()` can't hold ordered multi-token rows) into a structured form: a list of rows, each a list of `|`-split tokens. Keep flattening per-cell `area=` into `CellMeta.Directives` as today.
- **metadata:** carry the layout as a sibling `NotebookLayout` (list of rows of tokens) alongside `NotebookMeta`, passed exactly as `NotebookProvenance` is — `meta` stays `[]CellMeta`, so batch/headless is untouched. Surfaced as `opts.layout` (SSE `NB.init`) and `nb.layout` (wasm port).
- **client (`internal/webui`):** rework `buildControlsAndCells` — bucket every cell and placeable control by `area` (matching a layout token area-first then cell-ID), render rows of equal-flex columns per the layout, append unplaced cells below in source order. Reuse the `.cellrow` flex CSS. DOM order is free to change because rendering is address-by-ID (`cellEls[ev.cell]`), not position.
- **migration:** the 7 `row=` notebooks → `area=` + `layout` lines; update lotka's prose (it explains `row=panels` in narrative).

Each step is presentation-only and independently testable; the degrade-to-linear invariant is the regression test (strip the layout → source-order render still correct). **A gofmt-stability test on a notebook carrying layout directives is mandatory** — it is the check that would have caught the block-syntax flaw.

## Sequencing vs the color sweep

The chart-palette sweep (task #6, validated palette ready) and composition are both "designerly" but independent — color is a per-notebook hex remap, composition is a directive+client change. Neither blocks the other. Do either first.
