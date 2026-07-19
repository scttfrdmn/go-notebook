# Rendering

*How a cell's value becomes something on screen. The rule: a value is drawn if its type has a `Render()` method; what it draws is decided by the **MIME type** that method returns. Nothing about rendering runs in the browser's imagination — the client only paints MIME-tagged output the Go code produced.*

## The Render method

A cell whose result type implements `Render() Rendered` is drawn by that method. `Rendered` is a tiny envelope — a MIME type and its data — that every notebook redeclares locally (a notebook imports nothing from this project):

```go
type Rendered struct{ MIME, Data string }

type Thermo struct{ C, F int }

func (t Thermo) Render() Rendered {
	// build SVG…
	return Rendered{MIME: "image/svg+xml", Data: svg}
}
```

The method runs **in Go, in-process** — on your laptop, on the server, or compiled into the WASM in the browser. The client never interprets your value; it receives the already-rendered `{mime, data}` and paints it.

## The MIME types

| MIME | How it's shown |
|------|----------------|
| `image/svg+xml` | injected as markup (a chart, a gauge, a diagram) |
| `text/html` | injected as markup (a table, a card, an invoice — anything HTML) |
| `text/markdown` | shown as source text |
| `text/plain` | shown as a text readout |
| (no `Render`, a bare scalar) | shown as a plain value readout |
| (no `Render`, not a scalar) | stays hidden |

`image/svg+xml` and `text/html` are injected as HTML; everything else is set as text (never injected). This is why a chart is SVG or HTML and a number is just a readout.

## Why `fmt` belongs in Render, not a cell body

Formatting a value for display uses `fmt`, and `fmt` transitively reaches `os` — which the WASM portability gate forbids in a **cell**. But `Render()` is **not a cell** (it is a method the engine calls), so `fmt` there is fine. Put formatting in `Render`, keep it out of cell bodies, and the notebook stays browser-portable. (Use `strconv` if a cell body genuinely must format a number.) See [build & run](reference-build-run.html) for the portability gate.

## The degradation ladder

Rendering degrades gracefully:

- A value **with** a `Render()` returning SVG/HTML draws its rich view.
- A value with a `text/plain` render, or a bare scalar, shows as a readout.
- A value with no view stays hidden — never a broken box, never `[object Object]`.

**Losing the view costs polish, never correctness** — the computed value is unchanged; only its presentation drops to a simpler rung. This is the output-side twin of the control ladder in [controls](reference-controls.html).
