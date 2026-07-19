# Layout

*Arranging a notebook as a designed presentation instead of a source-order stack. Two pieces: `//notebook:area=<name>` tags cells into named regions, and a package-level `//notebook:layout` block orders those regions into rows and columns. Layout is presentation-only — strip it and the notebook renders correctly in source order.*

## The default: source order

With no layout directives, cells render top-to-bottom in the order they appear in the file. That is often fine, but source order rarely matches presentation order — you write helpers and models between the views you want side by side.

## `area` — name a region

Tag each cell that belongs together with the same `area`:

```go
//notebook:height=200 area=gauges
func dials(cpuPct, memMB int) (view Gauges) { … }

//notebook:height=220 area=status
func status(cpuPct, memMB int) (report Readout) { … }
```

Cells sharing an area stack together within that region.

## `layout` — order the regions

A package-level block, one `//notebook:layout` line per **row**. Within a row, `|` splits **equal-flex columns**; each column names an `area` (or, failing that, a single cell):

```go
//go:notebook
//notebook:layout intro
//notebook:layout gauges | status
//notebook:layout history
package sensorfeed
```

That reads: a full-width `intro` row, then a two-column row with `gauges` beside `status`, then a full-width `history` row. An arranged row's columns render as cards.

## Why per-row lines, not one indented block

The syntax is one directive line per row rather than an indented ASCII-art block, and that is deliberate: `gofmt` (which the project enforces) treats an indented, non-directive comment as prose and **reflows it** — reordering lines and stripping indentation, which would wreck a layout block. A `//notebook:layout …` line is a directive (no space after `//`), so `gofmt` leaves it verbatim.

## Reading the package doc

The layout is read from the comment groups **before the `package` clause** — not strictly the attached package doc, so a blank line between the layout lines and `package` is fine. Per-cell doc comments below `package` are not scanned for layout.

## Degrade to linear

Layout is the one presentation feature with the strongest invariant: **strip every `area` and `layout` directive and the notebook still renders**, cell by cell in source order. You lose the designed arrangement, never the content or the computation. It composes with the other presentation directives ([directives](reference-directives.html)) under the same rule — nothing about layout touches the dependency graph, which comes from function signatures alone.
