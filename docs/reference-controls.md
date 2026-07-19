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

## Cookbook — need this UI, write this shape

Each row links to a complete, buildable notebook in
[`examples/minimal/`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal)
— copy one and change the domain.

| Need this UI | Write this shape | Complete example |
|--------------|------------------|------------------|
| Number / text input | a parameterless cell returning a bare scalar: `func x() (n int)` | [`slider`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/slider), [`textinput`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/textinput) |
| Bounded slider | add `//notebook:slider min=… max=… step=…` | [`slider`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/slider) |
| Checkbox | return a `bool` scalar | [`checkbox`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/checkbox) |
| Select (one choice) | a type with `Options() []string` + a scalar `Value` field | [`selectbox`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/selectbox) |
| Multi-select | `Options() []string` + a **slice** `Value` field | [`multiselect`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/multiselect) |
| Range (from/to) | a type with `Bounds() (lo, hi float64)` | [`rangecontrol`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/rangecontrol) |
| Draggable points | a slice `Value` + a `Grip(i)` method, drawn as grip `Handle`s in a `Render` | [`draggable`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/draggable) |
| Editable table | a type with a slice-of-struct `Value` field | [`table`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/table) |

A few specifics the table can't hold:

- **Use value receivers.** The probe reflects over the exact value a cell
  returns. A cell returns a value (`func v() (view T)`), and a value does not
  carry pointer-receiver methods — so `func (t T) Bounds()` is found but
  `func (t *T) Bounds()` is not. Declare the capability methods on the value
  receiver.
- **Numeric types.** Any Go numeric kind works as a scalar input, including named
  scalar types (`type Erlangs float64`) — the kind is read structurally, so a
  named type keeps its control.
- **`Bounds()` returns `float64`.** Even an integer range declares `Bounds() (lo,
  hi float64)`; the client rounds to the step.
- **What crosses the wire.** A saved selection arrives in `Reconcile` as a
  JSON-decoded value: a select as a `string`, a multi as `[]string`, a range as
  `[]float64`, a draggable as a flat `[]float64` (`[x0,y0,x1,y1,…]`), a table as
  `[]map[string]any`. Type-assert it (see each example's `Reconcile`); if the
  assertion fails, return the fresh default — never keep a stale value.

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
