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

### Two ways to write it — same contract

Rendering is matched **structurally**, by the *shape* `Render() Rendered` (a
one-field-pair envelope), not by any imported type. So there are two ways to
satisfy it, and they are interchangeable:

**Zero-import protocol** — redeclare the envelope locally. Nothing from this
project appears in the file; the notebook is a plain Go package.

```go
type Rendered struct{ MIME, Data string }

func (v View) Render() Rendered {
	return Rendered{MIME: "text/html", Data: html}
}
```

**Optional `nb` convenience package** — import `nb` for autocomplete, named MIME
constructors (`nb.HTML`, `nb.SVG`, `nb.Markdown`), and compile-time checking of
the MIME string:

```go
import "github.com/scttfrdmn/go-notebook/nb"

func (v View) Render() nb.Rendered {
	return nb.HTML(html)
}
```

Both produce the identical `{mime, data}` on the wire — the engine probes the
method shape and never cares which `Rendered` you named. Use the protocol when
you want a file that imports nothing; use `nb` when you'd rather not hand-type
MIME strings. (The `nb` package is optional and the toolchain never depends on
it — see [charts](reference-charts.html) for its `nb/chart` sibling.)

## The MIME types

| MIME | How it's shown |
|------|----------------|
| `image/svg+xml` | injected as markup (a chart, a gauge, a diagram) |
| `text/html` | injected as markup (a table, a card, an invoice — anything HTML) |
| `text/markdown` | converted to a **safe HTML subset** and injected (see below) |
| `text/plain` | shown as a text readout |
| (no `Render`, a bare scalar) | shown as a plain value readout |
| (no `Render`, not a scalar) | stays hidden |

`image/svg+xml` and `text/html` are injected as raw markup; `text/markdown` is converted to a safe HTML subset and injected; everything else is set as text (never injected). This is why a chart is SVG or HTML and a number is just a readout.

### Markdown is a safe subset, not raw HTML

A cell that returns `text/markdown` (every notebook's `intro()` does) is converted to HTML at the one render chokepoint — so it arrives at the client as `text/html` and shows as formatted prose, not literal `**asterisks**`. But unlike raw `text/html`, the converter is **safe by construction**: every run of literal text is HTML-escaped, the only tags emitted are a fixed set, and a link's `href` must start with `http`, `https`, or `#` — so a notebook's markdown prose cannot inject script. It is a deliberate stdlib-only subset (no CommonMark parser, so the engine stays dependency-free and cross-compiles to a static binary and WASM):

- `#` / `##` / `###` headings
- `-` and `*` bullet lists
- `**bold**`, `*italic*`, `` `code` ``
- `[text](url)` links (`http`/`https`/`#` targets only)
- blank-line-separated paragraphs

Anything else degrades to escaped plain text rather than breaking — the same degradation ladder the rest of the view uses.

## Why `fmt` belongs in Render, not a cell body

Formatting a value for display uses `fmt`, and `fmt` transitively reaches `os` — which the WASM portability gate forbids in a **cell**. But `Render()` is **not a cell** (it is a method the engine calls), so `fmt` there is fine. Put formatting in `Render`, keep it out of cell bodies, and the notebook stays browser-portable. (Use `strconv` if a cell body genuinely must format a number.) See [build & run](reference-build-run.html) for the portability gate.

## Rich output is trusted code

`image/svg+xml` and `text/html` are **injected as markup** — so whatever your
`Render()` returns runs with the privileges of the host page. This is deliberate
(it is what makes an HTML invoice or an SVG chart possible, and it is the
`text/html` escape hatch the paper describes — an author can even ride JavaScript
in an image `onerror`). But it means:

- **A notebook's rendered output is code you are choosing to run.** Treat a
  notebook from an untrusted source the way you'd treat any untrusted program —
  do not serve it casually.
- **Never build HTML/SVG from untrusted input without sanitizing it.** If a cell
  incorporates external data into `text/html` or `image/svg+xml`, escape it; the
  engine injects what you return **verbatim**.
- `text/markdown` is the exception: it is injected, but through a safe-subset
  converter that escapes all literal text and validates link targets — so
  markdown prose (even from untrusted input) cannot execute.
- `text/plain` and scalar readouts are set as **text**, never injected — they
  cannot execute.

The trust boundary is the same one Go always has: you are running Go code. Rich
rendering just extends that to the markup it emits.

## Accessible output

A `Render()` that returns HTML or SVG is producing **application UI**, so the
usual web accessibility applies — the engine can't add it for you, because it only
injects the markup you return. A short checklist for the output you author:

- **SVG needs a text alternative.** A chart or gauge is invisible to a screen
  reader without one — add `role="img"` and a `<title>` (and `aria-labelledby`
  pointing at it), or a `<desc>`, so the picture has a spoken form.
- **Label values and carry units.** "42" is not "42 ms" — put the unit in the
  text, not only in an axis a sighted reader infers.
- **Draggable grips need a keyboard path.** A grip you can only drag with a mouse
  excludes keyboard and assistive-tech users; expose the same leaf as a typed
  input too (the [control degradation ladder](reference-controls.html) already
  gives you this — a draggable is still a set of numbers you can type).
- **Don't rely on color alone.** Pair a color with a label, shape, or pattern so a
  distinction survives color-blindness and grayscale. (The `nb/chart` palette is
  built for this — see [charts](reference-charts.html).)
- **Contrast and focus.** Meet WCAG AA contrast for text and marks; keep a visible
  focus ring on anything interactive.
- **Semantic HTML.** Use a real `<table>` with `<th>` for tabular output, headings
  for structure — the elements screen readers already understand.
- **Honor `prefers-reduced-motion`.** If a view animates (a propagating graph, a
  transition), gate the motion on that media query.

None of this is a framework obligation — it is the same care any hand-authored web
UI needs, applied to the markup a cell returns. The docs site itself follows it
(skip links, focus rings, measured contrast, reduced-motion), as a worked example.

## The degradation ladder

Rendering degrades gracefully:

- A value **with** a `Render()` returning SVG/HTML draws its rich view.
- A value with a `text/plain` render, or a bare scalar, shows as a readout.
- A value with no view stays hidden — never a broken box, never `[object Object]`.

**Losing the view costs polish, never correctness** — the computed value is unchanged; only its presentation drops to a simpler rung. This is the output-side twin of the control ladder in [controls](reference-controls.html).
