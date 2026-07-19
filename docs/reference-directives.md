# Directives

*The `//notebook:` comment directives, what each does, and the one rule they all obey: a directive only ever **refines presentation** — it never decides behavior. Delete every directive and the notebook still runs; it just looks plainer. For how a value becomes an input at all, see [controls](reference-controls.html); for arranging cells, [layout](reference-layout.html).*

A directive is a line in a cell's doc comment (or the package doc comment) that begins with `//notebook:`. The token right after the prefix names the kind; each following `key=value` refines it.

```go
// Incoming jobs per hour.
//notebook:slider min=0 max=5000 step=50
func arrivalRate() (lambda PerHour) { return 1200 }
```

## The directives

| Directive | Scope | What it does |
|-----------|-------|--------------|
| `//notebook:slider min=… max=… step=…` | cell | Refines a scalar input's control into a ranged slider with those bounds. |
| `//notebook:height=<px>` | cell | Sets the rendered height (pixels) reserved for this cell's view. |
| `//notebook:area=<name>` | cell | Tags the cell into a named layout region (see [layout](reference-layout.html)). |
| `//notebook:layout <row>` | package | One presentation row; `\|` splits equal-flex columns (see [layout](reference-layout.html)). |

## `slider`

Refines how an already-input scalar looks. `min`, `max`, and `step` set the range and increment.

```go
//notebook:slider min=-40 max=120 step=1
func celsius() (c int) { return 20 }
```

The directive **does not make `celsius` an input** — its being a parameterless cell whose result is consumed does. The directive only shapes the control. A directive on a value that is not an input is inert. (Whether something is an input is decided by its type and position in the graph — see [controls](reference-controls.html).)

## `height`

Reserves vertical space for a cell's view, so a chart is not clipped to a default height.

```go
//notebook:height=420
func curves(m Model) (chart Chart) { return Chart{M: m} }
```

## `area`

Groups a cell into a named region referenced by a `//notebook:layout` row. Cells sharing an `area` stack together inside that region.

```go
//notebook:height=200 area=gauges
func dials(cpuPct, memMB int) (view Gauges) { return Gauges{CPU: cpuPct, MemMB: memMB} }
```

## Not a directive: caching

There is deliberately **no** `//notebook:nocache`. Whether a cell's result can be cached is *derived* from its call graph — a cell that transitively reaches `time.Now()`, an RNG, or I/O is impure and recomputes on its own; everything else is cached. Cacheability affects correctness, and the governing rule is that a comment never decides correctness — only the code does. (An early design had such a directive; it was removed for exactly this reason. See [the design](design.html), *Cacheability is derived, not declared*.)

## The rule they share

Every directive is **presentation-only**. The graph — which cell feeds which — comes from function signatures, never from a comment. Strip all directives and:

- inputs are still inputs (decided by type),
- cells still recompute in dependency order,
- charts still render (just at a default size, stacked in source order).

You lose polish, never correctness. This is the same **degradation ladder** the [rendering](reference-rendering.html) doc describes, applied to layout.
