//go:notebook
//
// htmlcard — a rich view without hand-drawing SVG.
//
// The gentlest rich output: return a value whose Render() emits text/html, and
// the client injects it as markup. When the answer is a document (a card, a
// table, an invoice) rather than a plot, HTML is less work than SVG.
//
// This example uses the OPTIONAL nb convenience package (nb.HTML) — the second
// track. The svgchart example shows the zero-import track. Both are equivalent;
// see docs/reference-rendering.html.
//
//	go tool notebook run ./examples/minimal/htmlcard
//
// Demonstrates: text/html Render, the nb convenience package.

package htmlcard

import (
	"fmt"

	"github.com/scttfrdmn/go-notebook/nb"
)

// Monthly revenue, in dollars.
//
//notebook:slider min=0 max=100000 step=1000
func revenue() (usd int) { return 42000 }

// A summary card drawn as HTML.
//
//notebook:height=120
func summary(usd int) (view Card) { return Card{USD: usd} }

// Card renders an HTML summary. fmt lives HERE, in Render (the engine calls it),
// never in a cell body — so the WASM portability gate stays clear.
type Card struct{ USD int }

func (c Card) Render() nb.Rendered {
	status, color := "on track", "#0b7a99"
	if c.USD < 20000 {
		status, color = "below target", "#d0433b"
	}
	return nb.HTML(fmt.Sprintf(
		`<div style="font-family:sans-serif;padding:1rem;border:1px solid #e7ebf0;border-radius:10px">
		  <div style="font-size:2rem;font-weight:700;color:%s">$%d</div>
		  <div style="color:#5b6472">monthly revenue — %s</div>
		</div>`, color, c.USD, status))
}

// A compile-time check that the view really is renderable (a misspelled Render
// becomes a build error, not a silently-blank cell).
var _ nb.Renderable = Card{}
