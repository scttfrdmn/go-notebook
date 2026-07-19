# Controls

*How a value becomes an interactive input, and which control it renders as. The decision is made by the value's **type** — the methods it carries — never by a directive. A directive ([see directives](reference-directives.html)) only refines an input that already exists.*

## What makes a value an input

A cell is an **input** (a "leaf") when it is a parameterless cell whose result the rest of the graph consumes, and whose type is *editable*. Editability is decided by capability — the methods the type implements — probed the same way the analyzer probes everything else. A plain scalar with no special methods is the simplest input: a number or a string.

```go
//notebook:slider min=-40 max=120 step=1
func celsius() (c int) { return 20 }     // a scalar input → slider / number box
```

You do not register an input or flag it. The type says whether it is one; the graph position says whether it is shown.

## The control types

Each control comes from a method the value's type carries. The analyzer classifies by capability, and the client renders the matching widget:

| Type carries… | Control | Meaning |
|---------------|---------|---------|
| (a bare number / string) | slider or text box | a scalar input; `//notebook:slider` refines its range |
| underlying `bool` | checkbox | a toggle |
| `Options()` + a scalar field | **select** | one choice from a list |
| `Options()` + a slice field | **multi** | many choices from a list |
| `Bounds()` | **range** | a from/to span (a two-handled slider) |
| a slice `Value` field + `Grip()` | **draggable** | direct-manipulation points you drag |
| a slice `Value` field of structs | **table** | an editable grid (columns from the struct's fields) |

The order matters: `Options()` wins over `Bounds()`, which wins over `Grip()`, which wins over a plain slice. This is why a value that is both choosable and bounded renders as a choice.

## Bounds — a ranged slider

A type with a `Bounds()` method renders as a range control on its own — no directive needed.

```go
type Rate struct{ V, Lo, Hi int }
func (r Rate) Bounds() (lo, hi int) { return r.Lo, r.Hi }
```

## Options — select and multi

A type with `Options()` becomes a chooser. If its editable `Value` is a single value, it is a **select** (one choice); if a slice, a **multi** (several).

```go
type Pick struct{ Value string; All []string }
func (p Pick) Options() []string { return p.All }
```

## Reconcile — stateful widgets

A widget that carries state across edits (the current selection, the dragged positions, the table rows) implements `Reconcile(saved any) any`: given the value that arrived over the wire, it returns the reconciled widget. This is how a `Multi`, a `Range`, a `Table`, or a draggable keeps its state as the notebook recomputes. `Reconcile` is not what *makes* something an input (`Bounds`/`Options` do that) — it is how a stateful input survives a wave.

```go
type Series struct{ CSV string }
func (s Series) Reconcile(saved any) any {
	if str, ok := saved.(string); ok {
		return Series{CSV: str}
	}
	return s
}
```

## The degradation ladder

Every richer control degrades to a simpler one without losing correctness. A draggable is still a set of numbers you can type; a select is still a value. Strip the widget methods and you get a plain scalar input — the value still flows through the graph. **Losing the control costs interaction polish, never the computation.** The same ladder governs [rendering](reference-rendering.html) on the output side.
