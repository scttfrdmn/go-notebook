// Package nb is the optional convenience layer for go-notebook notebooks.
//
// A notebook never has to import anything from this project — that importless
// property is central to the design (see docs/design.md). This package is the
// other track: import it and you trade the zero-import purity for editor
// autocomplete, compile-time interface checks, and a few render constructors, so
// you write
//
//	func card() (v Invoice) { return Invoice{...} }
//	func (i Invoice) Render() nb.Rendered { return nb.HTML(i.html()) }
//
// instead of redeclaring the display envelope in every notebook. The two tracks
// are interchangeable: a type returning nb.Rendered and a type returning a
// locally-declared `struct{ MIME, Data string }` are seen identically by the
// engine, which matches Render() structurally (by the MIME/Data field shape),
// never by import. Mix them freely; nothing here changes how a notebook runs.
//
// Portability note: the constructors below are trivial and touch no OS. But
// building the string you pass them usually uses fmt, and fmt transitively
// reaches os — which the WASM portability gate forbids in a *cell body*. So call
// these from a Render() method (which the engine calls, and which is not a cell),
// never from a cell body. See docs/reference-rendering.html.
package nb

// Rendered is the MIME-tagged display envelope a cell's view returns. It is the
// importable twin of the `struct{ MIME, Data string }` a notebook would
// otherwise redeclare locally; the engine reads either by field shape.
type Rendered struct {
	// MIME is the content type, e.g. "text/html" or "image/svg+xml".
	MIME string
	// Data is the rendered content.
	Data string
}

// The MIME types the client knows how to paint. text/html and image/svg+xml are
// injected as markup; the rest are shown as text (never injected). These mirror
// docs/reference-rendering.md so a notebook can name them instead of typing the
// string.
const (
	MIMEHTML     = "text/html"
	MIMESVG      = "image/svg+xml"
	MIMEMarkdown = "text/markdown"
	MIMEText     = "text/plain"
)

// HTML wraps trusted HTML markup as a rendered view. The client injects it as
// markup — so it is trusted code: never build it from untrusted input without
// sanitizing. See the security note in docs/reference-rendering.md.
func HTML(data string) Rendered { return Rendered{MIME: MIMEHTML, Data: data} }

// SVG wraps SVG markup as a rendered view (a chart, a gauge, a diagram). Injected
// as markup, like [HTML].
func SVG(data string) Rendered { return Rendered{MIME: MIMESVG, Data: data} }

// Markdown wraps Markdown source as a rendered view. The engine converts it to a
// safe HTML subset at the single render chokepoint before the client paints it.
func Markdown(data string) Rendered { return Rendered{MIME: MIMEMarkdown, Data: data} }

// Text wraps a plain-text readout. Shown as text, never injected as markup.
func Text(data string) Rendered { return Rendered{MIME: MIMEText, Data: data} }

// The capability interfaces the engine probes for, restated here for
// compile-time assertions. A domain type satisfies these structurally whether or
// not it imports this package; declaring `var _ nb.Renderable = MyView{}` in a
// notebook simply turns "did I spell Render right?" into a build error instead of
// a silently-blank cell.
type (
	// Renderable is a value drawn by its own Render method. The engine matches
	// the method structurally, so a Render() returning a locally-declared
	// Rendered-shaped struct satisfies the engine just as well — this interface
	// is only for your own compile-time check.
	Renderable interface{ Render() Rendered }

	// Bounded is a scalar input that declares its own numeric range, rendering as
	// a ranged slider with no directive. lo and hi are the ends of the range.
	Bounded interface{ Bounds() (lo, hi float64) }

	// Optioned is a value offering a fixed set of choices, rendering as a select
	// (one choice) or a multi (a slice value).
	Optioned interface{ Options() []string }

	// Reconciler is a stateful widget that survives a recompute: given the value
	// that arrived over the wire, it returns the reconciled widget. This is how a
	// Multi, Range, Table, or draggable keeps its selection across a wave.
	Reconciler interface{ Reconcile(saved any) any }
)
