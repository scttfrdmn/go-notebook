//go:notebook
//
// Fat-tree — the interconnect trade nobody sees until the all-to-all crawls.
//
// A cluster's compute nodes are wired to each other through a tree of switches. Nodes
// sit at the leaves; switches stack in tiers up to a root. The question that decides
// whether your cluster is fast or a money pit: **how much bandwidth survives the climb
// to the top?** The number that answers it is the **bisection bandwidth** — cut the
// tree in half and count the link capacity crossing the cut. An all-to-all exchange
// (the pattern behind every distributed FFT, every AllReduce, every shuffle) is bottle-
// necked by exactly that cut.
//
// A **non-blocking fat-tree** keeps every tier as fat as the sum of what feeds it: the
// uplinks out of a switch carry as much as all its downlinks combined. Bisection
// bandwidth then scales linearly with node count, and an all-to-all runs at full
// speed. This is the ideal Charles Leiserson's fat-tree was designed to hit, and it is
// expensive — the switches at the top are enormous.
//
// So people **oversubscribe**: make each tier's uplinks carry only a fraction of its
// downlinks (2:1, 4:1). It saves real money — the higher tiers shrink — and for
// *local* traffic (jobs that stay within a rack) you never notice. The trap, and the
// whole point of this notebook, is what it does to *global* traffic:
//
//   **oversubscription compounds across tiers.** A per-tier uplink fraction f, over a
//   tree of depth L, cuts bisection bandwidth by f^(L-1) — so an all-to-all slows down
//   by 1/f^(L-1). A "modest" 2:1 per tier (f = 0.5) over four tiers is not 2× slower,
//   it's **8× slower** (0.5³). On a *shallow* tree oversubscription is a fine trade —
//   it saves more of the switch budget than it costs in bandwidth. But the deal gets
//   worse with every tier: by depth 4 you give up ~87% of your all-to-all bandwidth to
//   save ~77% of the switch cost, and the gap keeps widening. Worse still, the loss is
//   invisible for *local* traffic — you won't find out until the job that does a global
//   exchange runs.
//
// Drag the tree depth and the per-tier fatness. The diagram draws the actual switch
// tiers, with **link width proportional to capacity** — watch the upper links go thin
// as you oversubscribe, and watch the bisection cut (the dashed line through the root)
// starve. The verdict reports bisection bandwidth, the all-to-all slowdown, and the
// switch-cost saving, so you can see the trade both ways at once.
//
// The mechanism: the whole topology — every switch, every link width, the bisection
// and cost — is a **pure function of (depth, fatness)**. No state, no clock; change a
// slider and the tree is recomputed and redrawn from scratch. WASM-live. This is the
// interconnect the cluster is actually built on, modelled in the notebook that runs on
// it.

package fattree

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Tree depth — the number of switch tiers above the nodes. Node count is 2^depth
// (a binary fat-tree), so depth 4 = 16 nodes. Deeper trees make oversubscription
// compound harder.
//
//notebook:slider min=2 max=5 step=1
func treeDepth() (depth int) { return 4 }

// Per-tier fatness (%) — how much of a switch's total downlink bandwidth its uplinks
// carry. 100 = non-blocking (full fat-tree); 50 = 2:1 oversubscription per tier; 25 =
// 4:1. This is the switch-budget knob, and the one that compounds.
//
//notebook:slider min=25 max=100 step=5
func fatnessPct() (fatness int) { return 100 }

// ---------------------------------------------------------------------------
// Compute (Go) — the topology and its metrics, all pure.
// ---------------------------------------------------------------------------

// topology builds the fat-tree: node count, the switch count per tier, and the uplink
// capacity leaving each tier (which thins by the fatness fraction as you climb). Pure
// in (depth, fatness). Node link rate is 1 unit; a tier's uplink capacity is the sum
// of its downlinks times the fatness fraction.
func topology(depth int, fatness int) (tree Tree) {
	f := float64(fatness) / 100
	nodes := 1 << depth

	tiers := make([]Tier, depth)
	for level := 1; level <= depth; level++ {
		switches := nodes >> level
		// a switch at this level aggregates 2^level nodes below it; non-blocking
		// uplink capacity would be 2^level, thinned by f at every tier climbed so far.
		fullUplink := math.Pow(2, float64(level))
		uplinkCap := fullUplink * math.Pow(f, float64(level))
		tiers[level-1] = Tier{
			Level:     level,
			Switches:  switches,
			UplinkCap: uplinkCap,
		}
	}
	return Tree{Nodes: nodes, Depth: depth, Fatness: f, Tiers: tiers}
}

// analyze reads the interconnect trade off the topology: bisection bandwidth (the
// capacity across a cut through the root), the all-to-all slowdown vs a non-blocking
// tree, and the switch-cost saving from oversubscription. Pure in (tree). Bisection is
// nodes × f^(depth-1) — the fatness compounding once per tier climbed. The slowdown is
// its reciprocal; cost is the summed uplink bandwidth over all tiers (the switch $).
func analyze(tree Tree) (result Result) {
	compound := math.Pow(tree.Fatness, float64(tree.Depth-1))
	bisection := float64(tree.Nodes) * compound
	slowdown := 1.0 / compound

	cost := 0.0
	costFull := 0.0
	for _, t := range tree.Tiers {
		cost += float64(t.Switches) * t.UplinkCap
		costFull += float64(t.Switches) * math.Pow(2, float64(t.Level))
	}
	saving := 0.0
	if costFull > 0 {
		saving = 1 - cost/costFull
	}
	return Result{
		Tree:       tree,
		Bisection:  bisection,
		FullBisect: float64(tree.Nodes),
		Slowdown:   slowdown,
		CostSaving: saving,
	}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The fat-tree diagram: nodes along the bottom, switch tiers stacked to a single root,
// each link drawn with width proportional to its capacity. As you oversubscribe, the
// upper links visibly thin and the bisection cut (the dashed line through the root)
// starves — the picture of where your all-to-all bandwidth went.
//
//notebook:height=460
func diagram(result Result) (chart Tree2D) {
	return Tree2D{R: result}
}

// The verdict: bisection bandwidth, the all-to-all slowdown it implies, and the
// switch-cost saving that bought it. The saving is linear; the slowdown compounds —
// read them together and the trade is obvious.
func verdict(result Result) (report Readout) {
	blocking := "non-blocking — full bisection"
	if result.Slowdown > 1.01 {
		blocking = f1(result.Slowdown) + "× slower all-to-all"
	}
	return Readout{Cards: []Card{
		{Label: "nodes", Value: strconv.Itoa(result.Tree.Nodes), Caption: "2^depth leaves on the tree"},
		{Label: "bisection bandwidth", Value: f1(result.Bisection) + " / " + f0(result.FullBisect), Caption: "capacity across the cut vs non-blocking"},
		{Label: "all-to-all penalty", Value: blocking, Caption: "1 / fatness^(depth−1) — this is what compounds"},
		{Label: "switch-cost saving", Value: pct(result.CostSaving), Caption: "always less than the bandwidth given up"},
	}}
}

// Fat-tree — the interconnect trade nobody sees until the all-to-all crawls.
func intro() (md Markdown) {
	return `Cluster nodes talk through a tree of switches, and the **bisection
bandwidth** — the capacity across a cut through the root — sets how fast an all-to-all
(FFT, AllReduce, shuffle) can run. A **non-blocking fat-tree** keeps every tier as fat
as what feeds it, so bisection scales with node count. It's expensive, so people
**oversubscribe** — thin the upper links to save switch budget.

The trap: **oversubscription compounds.** A per-tier fatness f over depth L cuts
bisection by f^(L−1), so an all-to-all slows by 1/f^(L−1). A "modest" 2:1 per tier
(f=0.5) over four tiers is **8× slower**, not 2×. On a shallow tree it's a fine trade;
by depth 4 you give up ~87% of your bandwidth to save ~77% of the cost, and the gap
widens with every tier. Drag depth and fatness: the diagram draws the tiers with link
width ∝ capacity (watch the upper links thin), and the verdict shows the bandwidth lost
against the cost saved. Pure function of (depth, fatness); scrub freely.`
}

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// ===========================================================================
// Types
// ===========================================================================

// Tier is one level of switches and the capacity of the uplinks leaving it.
type Tier struct {
	Level     int
	Switches  int
	UplinkCap float64
}

// Tree is the fat-tree topology: node count, depth, fatness fraction, and its tiers.
type Tree struct {
	Nodes   int
	Depth   int
	Fatness float64
	Tiers   []Tier
}

// Result is the interconnect trade read off a topology.
type Result struct {
	Tree       Tree
	Bisection  float64
	FullBisect float64
	Slowdown   float64
	CostSaving float64
}

// Tree2D draws the fat-tree with link width proportional to capacity.
type Tree2D struct{ R Result }

func (t Tree2D) Render() Rendered {
	r := t.R
	tree := r.Tree
	const w, h, pad = 720.0, 460.0, 40.0
	plotW, plotH := w-2*pad, h-2*pad

	// rows: bottom = nodes, then one row per tier up to the root at the top.
	rows := tree.Depth + 1 // nodes + one per tier
	rowY := func(rowFromBottom int) float64 {
		return h - pad - float64(rowFromBottom)/float64(rows-1)*plotH
	}
	// x position of the i-th of count evenly-spaced items.
	xAt := func(i, count int) float64 {
		if count == 1 {
			return pad + plotW/2
		}
		return pad + float64(i)/float64(count-1)*plotW
	}
	maxCap := math.Pow(2, float64(tree.Depth)) // full root uplink, for width scaling
	lw := func(cap float64) float64 {
		if maxCap <= 0 {
			return 1
		}
		return 0.5 + cap/maxCap*7 // 0.5..7.5 px
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	// Precompute switch counts per row (row 0 = nodes, row k = tier k).
	countAt := func(row int) int {
		if row == 0 {
			return tree.Nodes
		}
		return tree.Tiers[row-1].Switches
	}
	// capacity of the uplink LEAVING row r (0 = node link = 1 unit, else the tier's).
	upCap := func(row int) float64 {
		if row == 0 {
			return 1
		}
		return tree.Tiers[row-1].UplinkCap
	}

	// Draw links first: each item in row r connects up to its parent in row r+1.
	for row := 0; row < tree.Depth; row++ {
		cnt := countAt(row)
		parentCnt := countAt(row + 1)
		y0 := rowY(row)
		y1 := rowY(row + 1)
		width := lw(upCap(row))
		// amber marks a link the tree oversubscribes: its capacity is less than the
		// non-blocking ideal for that tier (full = 2^level). Only the node links (row 0)
		// and a full-fat tree stay blue.
		color := "#3b82f6"
		if row > 0 && tree.Fatness < 1.0 {
			color = "#f59e0b"
		}
		for i := 0; i < cnt; i++ {
			parent := i * parentCnt / cnt
			x0 := xAt(i, cnt)
			x1 := xAt(parent, parentCnt)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke=%q stroke-width="%.2f"/>`,
				x0, y0, x1, y1, color, width)
		}
	}

	// bisection cut: a dashed horizontal line just below the root.
	cutY := rowY(tree.Depth) + (rowY(tree.Depth-1)-rowY(tree.Depth))*0.4
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dc2626" stroke-dasharray="6 4" stroke-width="1.5"/>`,
		pad, cutY, w-pad, cutY)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#dc2626">bisection cut: %.1f (of %.0f non-blocking)</text>`,
		pad+2, cutY-4, r.Bisection, r.FullBisect)

	// Draw nodes (row 0) and switches (rows 1..depth).
	for row := 0; row <= tree.Depth; row++ {
		cnt := countAt(row)
		y := rowY(row)
		for i := 0; i < cnt; i++ {
			x := xAt(i, cnt)
			if row == 0 {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="9" height="9" rx="1.5" fill="#1e293b"/>`, x-4.5, y-4.5)
			} else {
				fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#fff" stroke="#475569" stroke-width="1.5"/>`, x, y)
			}
		}
	}

	fmt.Fprintf(&b, `<text x="%.0f" y="20" font-family="sans-serif" font-size="12" fill="#334155">fat-tree: %d nodes, %d tiers — link width ∝ capacity (amber = oversubscribed)</text>`,
		pad, tree.Nodes, tree.Depth)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="10" fill="#94a3b8">nodes</text>`, pad, h-pad+18)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

// Render draws the verdict cards. A composite value with no Render method carries no
// MIME and the client hides the cell — so the cards render themselves. Engine-called,
// not a cell, so fmt here is fine.
func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:14px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e2e8f0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#64748b">%s</div>`, esc(c.Label))
		fmt.Fprintf(&b, `<div style="font-size:20px;font-weight:700;color:#1e293b;margin:2px 0">%s</div>`, esc(c.Value))
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#94a3b8">%s</div>`, esc(c.Caption))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
