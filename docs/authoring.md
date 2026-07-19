# Write your first notebook

*A from-scratch walkthrough. By the end you will have written your own notebook — a live thermometer that converts Celsius to Fahrenheit — run it in the browser, and compiled it to a static binary. For **why** the system works this way, read [`paper.md`](paper.md); for the full design, [`design.md`](design.md). This is the "how do I use it" doc.*

---

## 1. Install

You need **Go 1.25 or newer**. A notebook is an ordinary Go package, so it lives in a Go module.

```bash
mkdir tempconv && cd tempconv
go mod init tempconv                     # any module path; use github.com/you/tempconv to publish
go get -tool github.com/scttfrdmn/go-notebook/cmd/notebook@latest
```

The module path is just an identifier — `tempconv` is fine for something you keep local; use a real path like `github.com/you/tempconv` when you intend to publish it. (`tempconv` is also the name of this walkthrough's example — call yours whatever you like.)

`go get -tool` adds a `tool` directive to your `go.mod`, so the toolchain is available as **`go tool notebook`** in this module. It has three verbs:

```bash
go tool notebook check .    # analyze — print the dependency graph
go tool notebook run   .    # serve it in a browser; edit the source, it rebuilds
go tool notebook build .    # compile a standalone binary (or a WASM bundle)
```

*(Prefer not to add a tool directive? Every verb also works as `go run github.com/scttfrdmn/go-notebook/cmd/notebook <verb> .` once the module is a dependency.)*

## 2. The smallest notebook

Create `tempconv.go`:

```go
//go:notebook
package tempconv

// Temperature in Celsius.
//notebook:slider min=-40 max=120 step=1
func celsius() (c int) { return 20 }

// Converted to Fahrenheit — wired in by the parameter name `c`.
func fahrenheit(c int) (f int) { return c*9/5 + 32 }
```

That is a complete notebook. Two things make it one: the file carries the **`//go:notebook`** marker (the only mention of this project anywhere — no import), and each **cell is a top-level function with a doc comment and a named result.**

Run it:

```bash
go tool notebook run .
```

A browser opens showing a dependency graph (`celsius → fahrenheit`), a Celsius slider, and the Fahrenheit readout. Drag the slider — Fahrenheit recomputes. You wrote no wiring, no callback, no reactive framework. The edge exists because `celsius` produces a result named `c` and `fahrenheit` takes a parameter named `c`. That is the whole rule:

> **A cell's named result feeds any cell that takes a parameter of the same name and type.**

The graph is not something you maintain alongside the code — it is *derived from the code by the Go type checker*, so it cannot drift from it.

## 3. The four rules that bite

Before you go further, the rules that will trip you up once and never again. Each is a direct consequence of "a cell is a function," and `go tool notebook check .` catches most of them with a pointed message.

1. **A cell is a top-level function with *named* results.** `func celsius() (c int)` is a cell; `func celsius() int` is not (no named result = no edge = not a cell). The named result is the marker — *not* the doc comment, which only supplies the human label. This is also how you write a **helper**: give it *unnamed* returns and it stays ordinary Go, invisible to the graph — e.g. `func clamp(v, lo, hi int) int`. (A documented function with unnamed returns is still a helper; an undocumented function with a named result is still a cell, just labelled from its function name. Documenting cells is strongly recommended for the label and tooltip — but it is not what makes them cells.)

2. **The result name *is* the edge.** To wire a value into a consumer, the producer's result must be named exactly what the consumer's parameter is named. Rename `celsius`'s result from `c` to `temp` and the build fails with:

   ```
   cell "fahrenheit" needs `c int`, but no cell produces it.
   Did you mean `celsius`, which produces `int`?
   ```

   The name carries the meaning; rename deliberately.

3. **Result names must be unique across the notebook.** Two cells returning `(chart Chart)` collide — `check` passes but `build` fails ("a result name is an edge, must be unique"). Give each its own name.

4. **Keep `fmt` out of cell bodies.** A notebook that compiles to the browser (`GOOS=js`) must not have a *cell* whose call graph reaches `fmt`/`os`/`net` (the portability gate is derived from the graph). Formatting belongs in a `Render()` method — which the engine calls, and which is not a cell — not in a cell body. Use `strconv` if a cell body genuinely needs to format a number.

## 4. Add a rich view (the easy way first)

A cell's output is drawn by *structural probe*: return a value with a `Render() Rendered` method and the client draws its MIME-tagged content, decided by the MIME type the method returns. There are two common routes, and the gentlest is **HTML** — no drawing, just markup:

```go
import "fmt"

// A summary card, drawn as HTML.
//notebook:height=90
func card(c, f int) (view Card) { return Card{C: c, F: f} }

// Card renders an HTML card. fmt lives HERE, in Render (the engine calls it) —
// never in a cell body, so the WASM portability gate stays clear.
type Card struct{ C, F int }

func (c Card) Render() Rendered {
	return Rendered{MIME: "text/html", Data: fmt.Sprintf(
		`<div style="font:600 22px sans-serif;color:#1b3a6b">%d°C = %d°F</div>`, c.C, c.F)}
}

// A notebook imports nothing from this project; redeclare the tiny display type.
type Rendered struct{ MIME, Data string }
```

Run it: the card updates live as you drag. When the answer is a document — a card, a table, an invoice — HTML is the least work. (Prefer autocomplete and a compile-time check that you spelled `Render` right? Import the optional [`nb`](https://github.com/scttfrdmn/go-notebook/tree/main/nb) package and return `nb.HTML(…)` instead of redeclaring `Rendered`. Same result; see [rendering](reference-rendering.html).)

## 5. Draw it yourself with SVG (the low-level route)

When the answer is a *picture* rather than a document, build the SVG markup yourself. This is the most control and still requires no imports beyond the standard library — but it is deliberately low-level, so reach for it when HTML won't do. Replace the card with a thermometer:

```go
import (
	"fmt"
	"strings"
)

// A thermometer drawn from the value.
//notebook:height=160
func gauge(c, f int) (view Thermo) { return Thermo{C: c, F: f} }

// Thermo renders a simple SVG thermometer. fmt lives HERE, in Render (the engine
// calls it) — never in a cell body, so the WASM portability gate stays clear.
type Thermo struct{ C, F int }

func (t Thermo) Render() Rendered {
	frac := float64(t.C+40) / 160.0
	if frac < 0 { frac = 0 }
	if frac > 1 { frac = 1 }
	h := 20 + frac*160
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 220">`)
	fmt.Fprintf(&b, `<rect width="240" height="220" fill="#fff"/>`)
	fmt.Fprintf(&b, `<rect x="20" y="20" width="24" height="160" rx="12" fill="#e7ebf0"/>`)
	fmt.Fprintf(&b, `<rect x="20" y="%.0f" width="24" height="%.0f" rx="12" fill="#2a78d6"/>`, 180-(h-20), h-20)
	fmt.Fprintf(&b, `<text x="60" y="90" font-family="sans-serif" font-size="28" font-weight="700" fill="#1b3a6b">%d°C</text>`, t.C)
	fmt.Fprintf(&b, `<text x="60" y="125" font-family="sans-serif" font-size="20" fill="#5b6472">%d°F</text>`, t.F)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}
```

(It reuses the same `Rendered` type you declared in §4 — one per notebook is enough.) `gauge` takes `c` and `f` — so it wires downstream of both `celsius` and `fahrenheit`, and the graph forks. Run again and drag the slider: the thermometer fills and both numbers update, live.

Here is exactly that notebook, compiled to WebAssembly and running right here — drag it:

<div class="demoframe"><iframe src="../demos/tempconv/index.html" loading="lazy" title="the tempconv notebook, live"></iframe></div>

*(The client renders `image/svg+xml` and `text/html` as markup; a scalar with no `Render()` shows as a text readout; anything else stays hidden — the **degradation ladder**: losing the view costs polish, never correctness.)*

## 6. Controls come from types

You already used one: `//notebook:slider min=-40 max=120` refines how the `celsius` input looks. But *whether* something is an input is decided by its **type**, never by the comment — a directive only refines an already-input control's appearance. A type carrying `Bounds()` renders as a ranged slider on its own; `Options()` gives a select; `Reconcile()` gives a stateful widget (`Multi`, `Range`, `Table`, a draggable). Delete every directive and every control is still there, just plainer.

## 7. Arrange it (optional)

By default cells stack in source order. To present a designed layout, add `//notebook:area=` to cells and a package-level `//notebook:layout` block:

```go
//go:notebook
//notebook:layout celsius | gauge
```

That puts the Celsius control beside the thermometer instead of stacked. The full vocabulary — named regions, columns, cards — is in [Layout](reference-layout.html). It is presentation-only: strip the layout and the notebook still renders correctly.

## 8. Ship it

The same file is also a job. Build a standalone binary:

```bash
go tool notebook build -o tempconv .
./tempconv --headless --json           # run once, print the values
# {"provenance": {...}, "values": {"c": 20, "f": 68, ...}}
./tempconv --headless --set c=100 --json   # override an input (by RESULT name)
```

No Python environment, no kernel — one static binary you can `scp` to a cluster and `sbatch`. And because your notebook touches no `net`/`os`/cgo, it is also browser-portable:

```bash
go tool notebook build -target=wasm -o site .
# serve site/ over HTTP → the notebook runs entirely client-side, no server
```

That is the whole loop: **one Go file is a live browser app, a batch job, and a served page — distinguished only by where you point the compiler.**

---

## "An ordinary Go package" — what that actually means

The claim is literal, and the edges are worth spelling out:

- **One file per package carries `//go:notebook`.** Cells are the top-level
  functions *in that file*. You can split the rest of the package across as many
  `.go` files as you like — types, helpers, tests — and they are ordinary Go.
  Only the marked file's functions are scanned for cells.
- **A named-result function in an *unmarked* file is not a cell.** It is a plain
  helper the cells can call. (So "move this out of the graph" can be as simple as
  moving it to another file in the package.)
- **The package imports and compiles normally.** `go build`, `go test`, `go doc`,
  and gopls all work on it — it is a real package. A notebook can define exported
  APIs and be imported by other Go code; the `//go:notebook` marker only tells
  *this* toolchain which file to read as a notebook.
- **Methods and generic functions are never cells.** A method isn't a top-level
  function; a generic function has no concrete result type to wire. Both are
  fine to define and call — they just aren't cells.
- **Tests are ordinary tests.** Because a cell is a function, you test it by
  calling it: `go test` runs `TestX(t)` against your cells with no notebook
  runtime involved (see the [`testing`](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal/testing)
  example).

## Why `fmt` in a cell body breaks the browser build

One surprise worth naming directly: `fmt` in a *cell body* fails the WASM
portability gate, and Go developers reasonably expect `fmt.Sprintf` to be pure.
It is — but the gate is a **conservative over-approximation of the reachable call
graph**, and `fmt` transitively reaches `os`. The analyzer can't prove a
particular `fmt` call never touches the OS-facing paths, so it flags the whole
cell rather than risk a notebook that compiles but can't run in a browser. Keep
formatting in a `Render()` method (which the engine calls, and which is not a
cell) — as this walkthrough does — and use `strconv` if a cell body genuinely
must format a number. This is analyzer conservatism, not a claim that `fmt` does
I/O.

---

## Where to go next

**Reference** — every feature, in depth:

- [Directives](reference-directives.html) — the `//notebook:` comment directives.
- [Controls](reference-controls.html) — how a value becomes an input, and which widget it renders as.
- [Rendering](reference-rendering.html) — the `Render` method, MIME types, the degradation ladder.
- [Layout](reference-layout.html) — arrange a notebook as a designed dashboard with `area` + `layout`.
- [Build & run](reference-build-run.html) — the verbs, the binary's flags, the WASM gate.
- [Provenance](reference-provenance.html) — what a built artifact records about its origin.

**Deeper reads:**

- [The paper](paper.html) — the system, end to end, and why it is shaped this way.
- [The design](design.html) — the full design record.
- The [`examples/` directory](https://github.com/scttfrdmn/go-notebook/tree/main/examples) — 44 notebooks on GitHub, from an M/M/c queue to a Simpson's-paradox table; read them as Go.
