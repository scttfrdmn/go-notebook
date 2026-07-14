//go:notebook
//
// A phase transition you can scrub.
//
// Fill a grid: each cell is open with probability p, blocked otherwise. Ask one
// question — is there a connected path of open cells from the top edge to the
// bottom? For small p the open cells form scattered islands and the answer is no.
// For large p they merge into one blob and the answer is obviously yes. The
// interesting part is what happens in between: the switch from "no" to "yes" is not
// gradual. It snaps, near a critical probability p_c ≈ 0.593, and it snaps harder
// the bigger the grid. That sudden snap is a *phase transition* — the same kind of
// thing as water freezing — and here you can drag a slider through it.
//
// Two design choices, both the corpus's usual discipline:
//
//   - **Pure, so you can scrub both ways.** The whole picture is a pure function of
//     (size, p, seed): the grid is a deterministic fill, the clusters are computed
//     by union-find, the spanning test is a lookup. Nothing accumulates. So you can
//     scrub p up THROUGH the transition and back down and the field is exact at
//     every step — the same reason the bayes posterior can re-widen. A fold could
//     not do this; a phase transition is exactly the thing you want to sweep back
//     and forth across, and pure cells let you.
//   - **The graph shows the pipeline.** grid → clusters → view is three cells, and
//     the dependency graph draws that pipeline: the random field, the connected
//     components, the picture. Each is independently inspectable.
//
// This is stat-mech in the systems niche: percolation is the textbook model for
// connectivity thresholds — when does a random network of open links first conduct
// end to end. No incumbent notebook shows the transition *moving*.

package percolation

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Grid size — the world is n×n cells. Bigger grids make the transition sharper:
// the snap from "no path" to "path" tightens around p_c as n grows.
//
//notebook:slider min=32 max=160 step=8
func size() (n int) { return 96 }

// Occupation probability, in percent — the scrub axis. Drag it up through ~59% and
// watch a spanning cluster suddenly appear; drag back down and watch it dissolve.
//
//notebook:slider min=30 max=80 step=1
func probabilityPct() (pct int) { return 55 }

// Seed for the random fill. A leaf, not a global RNG, so the grid is a pure and
// reproducible function of the sliders (the corpus's no-hidden-state rule).
//
//notebook:slider min=1 max=999 step=1
func seed() (s int) { return 7 }

// ---------------------------------------------------------------------------
// Compute (Go) — all pure.
// ---------------------------------------------------------------------------

// The open/blocked grid: n×n cells, each open with probability ~pct/100, chosen by
// a deterministic LCG keyed on the seed. Pure — same (n, pct, s) gives the same
// grid — so scrubbing p is exact in both directions.
func grid(n int, pct int, s int) (field Grid) {
	open := make([]bool, n*n)
	state := uint64(s)*2654435761 + 1
	threshold := uint64(pct) * 0xFFFFFFFF / 100
	for i := range open {
		state = state*6364136223846793005 + 1442695040888963407
		open[i] = (state >> 32) < threshold
	}
	return Grid{N: n, Open: open}
}

// Connected components of the open cells (4-connectivity), by union-find, plus which
// component — if any — spans top edge to bottom edge. This is the physics question
// turned into a graph question: a spanning cluster is a conducting path.
func clusters(field Grid) (labeled Clusters) {
	n := field.N
	uf := newUF(n * n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if !field.Open[y*n+x] {
				continue
			}
			i := y*n + x
			if x+1 < n && field.Open[i+1] {
				uf.union(i, i+1)
			}
			if y+1 < n && field.Open[i+n] {
				uf.union(i, i+n)
			}
		}
	}
	// A component spans if it reaches both the top row and the bottom row.
	topRoots := map[int]bool{}
	for x := 0; x < n; x++ {
		if field.Open[x] {
			topRoots[uf.find(x)] = true
		}
	}
	spanning := -1
	for x := 0; x < n; x++ {
		i := (n-1)*n + x
		if field.Open[i] && topRoots[uf.find(i)] {
			spanning = uf.find(i)
			break
		}
	}
	// Component sizes, and the largest.
	size := map[int]int{}
	for i := 0; i < n*n; i++ {
		if field.Open[i] {
			size[uf.find(i)]++
		}
	}
	largest := 0
	for _, sz := range size {
		if sz > largest {
			largest = sz
		}
	}
	roots := make([]int, n*n)
	for i := range roots {
		if field.Open[i] {
			roots[i] = uf.find(i) + 1 // +1 so 0 means "blocked"
		}
	}
	return Clusters{
		N: n, Root: roots, Spanning: spanning, Count: len(size), Largest: largest,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The grid. Blocked cells are dark; each open cluster gets its own colour; and the
// spanning cluster — the conducting path, if one exists — is drawn bright, so the
// transition is unmistakable: below p_c the field is a confetti of small clusters,
// and the moment one spans it lights up top to bottom.
//
//notebook:height=440
func view(labeled Clusters) (picture Picture) {
	return Picture{C: labeled}
}

// The numbers under the picture: where p sits relative to the critical threshold,
// how many clusters there are, the largest, and the verdict — does it span?
func stats(field Grid, labeled Clusters, pct int) (report Readout) {
	verdict := "no spanning path"
	if labeled.Spanning >= 0 {
		verdict = "SPANS top → bottom"
	}
	openFrac := 0
	for _, o := range field.Open {
		if o {
			openFrac++
		}
	}
	return Readout{Cards: []Card{
		{Label: "p", Value: "0." + pad2(pct), Caption: "critical p_c ≈ 0.593"},
		{Label: "clusters", Value: itoa(labeled.Count)},
		{Label: "largest cluster", Value: pctOf(labeled.Largest, field.N*field.N), Caption: "of all cells"},
		{Label: "percolates?", Value: verdict},
	}}
}

// A phase transition you can scrub.
func intro() (md Markdown) {
	return `Each cell is **open** with probability *p*. Is there a connected path of
open cells from the top edge to the bottom? Drag *p* and watch.

Below about **p_c ≈ 0.593** the open cells are scattered islands — no path. Above
it, one cluster suddenly spans the whole grid and lights up bright green. The switch
is not gradual; it *snaps*, and it snaps sharper on a bigger grid. That snap is a
**phase transition**.

It's all pure — the grid, the clusters (by union-find), the spanning test are a pure
function of the sliders — so you can scrub *p* up through the transition and back
down and every frame is exact. A phase transition is precisely the thing you want to
sweep back and forth across, and nothing here is stateful, so you can.`
}

// ===========================================================================
// Union-find
// ===========================================================================

type uf struct{ parent, rank []int }

func newUF(n int) *uf {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &uf{parent: p, rank: make([]int, n)}
}

func (u *uf) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]] // path halving
		x = u.parent[x]
	}
	return x
}

func (u *uf) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if u.rank[ra] < u.rank[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	if u.rank[ra] == u.rank[rb] {
		u.rank[ra]++
	}
}

// ===========================================================================
// Helpers
// ===========================================================================

func itoa(n int) string { return strconv.Itoa(n) }

func pad2(v int) string {
	s := strconv.Itoa(v)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func pctOf(part, whole int) string {
	if whole == 0 {
		return "0%"
	}
	return strconv.Itoa(part*100/whole) + "%"
}

// clusterColor maps a (1-based) cluster root to an R,G,B. Blocked (root 0) is the
// dark background; the spanning cluster is overridden to bright green by the caller.
// A cheap hash spreads roots across a muted palette so adjacent clusters differ.
func clusterColor(root int) (uint8, uint8, uint8) {
	h := uint32(root) * 2654435761
	// muted, mid-brightness so the bright spanning green stands out against them
	r := uint8(90 + (h>>16)&0x3f)
	g := uint8(90 + (h>>8)&0x3f)
	b := uint8(120 + h&0x3f)
	return r, g, b
}

// ===========================================================================
// Types
// ===========================================================================

// Grid is the open/blocked field — the pure random input.
type Grid struct {
	N    int
	Open []bool
}

// Clusters is the connected-components result: a per-cell root label (0 = blocked),
// which root spans (or -1), and summary counts.
type Clusters struct {
	N        int
	Root     []int
	Spanning int
	Count    int
	Largest  int
}

// Picture renders a Clusters to a coloured grid image.
type Picture struct {
	C Clusters
}

// Render draws the grid as a PNG embedded in an SVG wrapper (the client injects
// image/svg+xml but shows a bare PNG as text, so the wrapper is what paints it,
// upscaled crisp with pixelated rendering). Blocked cells dark, each cluster its own
// colour, the spanning cluster bright green.
func (p Picture) Render() Rendered {
	n := p.C.N
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for i, root := range p.C.Root {
		var r, g, b uint8
		switch {
		case root == 0:
			r, g, b = 15, 21, 36 // blocked — the dark background
		case root-1 == p.C.Spanning:
			r, g, b = 62, 212, 135 // the spanning cluster — bright green
		default:
			r, g, b = clusterColor(root)
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, 255
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	const box = 420
	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, box, box)
	fmt.Fprintf(&s, `<image x="0" y="0" width="%d" height="%d" href=%q style="image-rendering:pixelated"/>`,
		box, box, uri)
	s.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: s.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
