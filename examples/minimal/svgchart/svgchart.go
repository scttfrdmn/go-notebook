//go:notebook
//
// svgchart — the low-level, dependency-free rendering route.
//
// SVG is the most control you can have over a view and requires no imports at
// all: build the markup string yourself and return it tagged image/svg+xml. This
// example uses the ZERO-IMPORT track — it redeclares the tiny Rendered envelope
// locally rather than importing nb. (Compare htmlcard, which uses nb.HTML.)
//
//	go tool notebook run ./examples/minimal/svgchart
//
// Demonstrates: image/svg+xml Render, the zero-import track. See docs/reference-rendering.html.

package svgchart

import (
	"fmt"
	"strings"
)

// How many bars to draw.
//
//notebook:slider min=1 max=12 step=1
func bars() (n int) { return 6 }

// A simple bar chart drawn from the count.
//
//notebook:height=200
func chart(n int) (view Bars) { return Bars{N: n} }

// Bars renders an SVG bar chart. fmt is fine HERE (Render is not a cell body).
type Bars struct{ N int }

func (b Bars) Render() Rendered {
	const w, h, pad = 320, 180, 10
	bw := float64(w-2*pad) / float64(b.N)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, w, h)
	fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#fff"/>`, w, h)
	for i := 0; i < b.N; i++ {
		bh := float64(h-2*pad) * float64(i+1) / float64(b.N)
		x := float64(pad) + float64(i)*bw
		fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#0070b8"/>`,
			x+2, float64(h-pad)-bh, bw-4, bh)
	}
	sb.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: sb.String()}
}

// Rendered is the tiny display envelope, redeclared locally (no import).
type Rendered struct{ MIME, Data string }
