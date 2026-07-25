// notebook-in-notebook — embed a live, self-contained notebook as an interactive
// FIGURE inside another notebook, driven through nothing but a rendered iframe.
//
// This is composition the ALLOWED way (see the paper's §13, "the notebook as a
// component"): the inner notebook runs itself, as its own compiled WASM program;
// the outer one merely PLACES it, via the text/html Render escape hatch. There is
// no import, no merged dependency graph, no nested-cell model — those would be the
// "component model" the design deliberately forbids (§69), because the graph is a
// projection of ONE package's function signatures and cannot span two. What you
// get instead: a document that narrates around a live, parameterized demo.
//
// The inner notebook is referenced by URL, as a DEPLOYED SIBLING — build it too
// and place it beside this one (see below). Append ?embed so it renders as a bare
// figure (no header, graph panel, or footer — just its controls and output).
//
//	# build both, side by side, then serve the directory:
//	go tool notebook build --target=wasm -o ./out ./examples/minimal/notebook-in-notebook
//	go tool notebook build --target=wasm -o ./out/reaction ./examples/capacity
//	(cd out && python3 -m http.server)   # the iframe finds ./reaction/?embed
//
// Demonstrates: text/html Render escape hatch, ?embed chromeless view, notebook
// composition without an import. See docs/reference-rendering.html and §13.
//
//go:notebook
package nbinnb

// Rendered is the zero-import render envelope (a notebook imports nothing from the
// project); see the svgchart recipe for the same track.
type Rendered struct{ MIME, Data string }

// Markdown is prose: a cell that returns it renders as formatted text, not a
// cramped scalar readout. Every notebook's narration uses this shape — a bare
// `string` return would show one word per line in a control column. See the
// rendering reference's degradation ladder.
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }

// intro is the outer notebook's own prose — the "chemistry text" around the figure.
func intro() (heading Markdown) {
	return "## Reaction throughput\n\n" +
		"A rate-limited step behaves like a queue: offered load against a fixed " +
		"number of servers. The figure below is a **live, embedded notebook** — " +
		"drag its sliders and watch utilization and wait time respond."
}

// Figure renders an <iframe> to a sibling notebook's ?embed view. The inner
// notebook is a separate compiled program; this cell only decides where it sits.
type Figure struct{ Src string }

func (f Figure) Render() Rendered {
	return Rendered{
		MIME: "text/html",
		Data: `<iframe src="` + f.Src + `" title="embedded capacity model" loading="lazy" ` +
			`style="width:100%;height:640px;border:1px solid #e7ebf0;border-radius:10px;background:#fff"></iframe>`,
	}
}

// reaction places the embedded demo. ?embed drops the inner notebook's chrome so
// it reads as a figure, not a page-in-a-page.
func reaction() (fig Figure) { return Figure{Src: "reaction/?embed"} }

// closing is more of the outer notebook's own narration, AFTER the figure —
// proving the embed is just one cell among the outer notebook's normal cells.
// (Its result is named `note`, not `heading`: a result name is a globally-unique
// edge, so two prose cells cannot share one — check catches a clash before build.)
func closing() (note Markdown) {
	return "Past **ρ = 1** the queue is unstable and wait time diverges — the " +
		"reaction backs up faster than it clears. Everything above this line is the " +
		"outer notebook; the figure is a second notebook running inside it."
}
