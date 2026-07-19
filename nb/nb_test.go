package nb_test

import (
	"testing"

	"github.com/scttfrdmn/go-notebook/engine"
	"github.com/scttfrdmn/go-notebook/nb"
)

// TestConstructorMIME pins the MIME each helper stamps — the contract a notebook
// relies on to be painted as markup vs. text.
func TestConstructorMIME(t *testing.T) {
	cases := []struct {
		got  nb.Rendered
		mime string
	}{
		{nb.HTML("<b>x</b>"), "text/html"},
		{nb.SVG("<svg/>"), "image/svg+xml"},
		{nb.Markdown("# x"), "text/markdown"},
		{nb.Text("x"), "text/plain"},
	}
	for _, c := range cases {
		if c.got.MIME != c.mime {
			t.Errorf("MIME = %q, want %q", c.got.MIME, c.mime)
		}
		if c.got.Data == "" {
			t.Errorf("Data empty for %q", c.mime)
		}
	}
}

// convenienceView renders via the importable nb.Rendered.
type convenienceView struct{ n int }

func (v convenienceView) Render() nb.Rendered { return nb.HTML("conv") }

// localRendered is the zero-import track: a notebook's own display envelope.
type localRendered struct{ MIME, Data string }
type localView struct{ n int }

func (v localView) Render() localRendered { return localRendered{"text/html", "local"} }

// TestBothTracksRenderIdentically is the load-bearing claim of the two-track
// story: the engine's structural probe reads a Render() returning the importable
// nb.Rendered and one returning a locally-declared Rendered-shaped struct exactly
// the same way. If this ever diverges, the "import nothing OR import nb, your
// choice" promise is broken.
func TestBothTracksRenderIdentically(t *testing.T) {
	conv, okConv := engine.AsRendered(convenienceView{1})
	local, okLocal := engine.AsRendered(localView{1})
	if !okConv || !okLocal {
		t.Fatalf("both must be renderable: nb.Rendered=%v local=%v", okConv, okLocal)
	}
	if conv.MIME != "text/html" || local.MIME != "text/html" {
		t.Errorf("MIME mismatch: conv=%q local=%q", conv.MIME, local.MIME)
	}
	if conv.Data != "conv" || local.Data != "local" {
		t.Errorf("Data not read through the probe: conv=%q local=%q", conv.Data, local.Data)
	}
}

// Compile-time proof the convenience interfaces are satisfiable by a notebook's
// own types — the whole point of the assertion track.
var _ nb.Renderable = convenienceView{}
