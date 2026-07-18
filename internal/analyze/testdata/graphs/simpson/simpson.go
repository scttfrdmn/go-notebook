//go:notebook
//
// Simpson's paradox: the aggregate can reverse every subgroup.
//
// Treatment A beats Treatment B in the small-stone group AND in the large-stone
// group — yet pooled across both, B looks better. Nothing is wrong with the
// arithmetic; the pooled rate is a weighted average, and the weights (how many
// cases fall in each group) are different for the two treatments. This is the
// real 1986 kidney-stone study, and the reason "controlling for" a variable is
// not optional.
//
// It is anscombe's lesson in a second key: **don't trust the aggregate.** There
// the summary hid the shape; here the summary reverses the truth.
//
// Design deferred to HTML, and for a specific reason: the reveal IS a table. You
// have to see treatment A win each ROW and then lose the TOTAL row for the
// paradox to land — a bar chart flattens exactly the structure that matters. So
// the view is an HTML table (Go computes every rate; the table only arranges
// them), with the winning cell in each row tinted, so the eye catches "A, A …
// B?!" at a glance. A small companion chart shows the pooled bars beside it.
//
// Drag the case-mix sliders and watch the paradox switch on and off: when both
// treatments see the same mix of easy/hard cases, the pooled winner agrees with
// the subgroups; skew the mix and the aggregate flips while every subgroup holds.
//
//notebook:layout intro
//notebook:layout knobs | table

package simpson

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs. Per-group success rates are fixed at the study's values; the KNOBS are
// the case mix — how the patients were assigned — because the mix is what drives
// the paradox, and making it draggable is what lets you switch the paradox on.
// ---------------------------------------------------------------------------

// Treatment A cases in the SMALL-stone (easy) group.
//
//notebook:slider min=1 max=400 step=1 area=knobs
func aSmallCases() (aSmall int) { return 87 }

// Treatment A cases in the LARGE-stone (hard) group.
//
//notebook:slider min=1 max=400 step=1 area=knobs
func aLargeCases() (aLarge int) { return 263 }

// Treatment B cases in the SMALL-stone (easy) group.
//
//notebook:slider min=1 max=400 step=1 area=knobs
func bSmallCases() (bSmall int) { return 270 }

// Treatment B cases in the LARGE-stone (hard) group.
//
//notebook:slider min=1 max=400 step=1 area=knobs
func bLargeCases() (bLarge int) { return 80 }

// ---------------------------------------------------------------------------
// The per-group success RATES are the study's, held fixed — the paradox is a
// property of the mix, not the rates, so we pin the rates and vary the mix.
// Small stones are the easy case (both treatments do well); large are hard.
// A beats B in BOTH groups. (Percent points, 0..100.)
// ---------------------------------------------------------------------------

// Treatment A, small stones: the easy case, done well. The result name IS the
// edge, so each rate is named exactly as its consumers read it (aSmallR, …).
func aSmallRate() (aSmallR Pct) { return 93 }

// Treatment A, large stones: the hard case; still ahead of B.
func aLargeRate() (aLargeR Pct) { return 73 }

// Treatment B, small stones: a hair behind A.
func bSmallRate() (bSmallR Pct) { return 87 }

// Treatment B, large stones: behind A here too.
func bLargeRate() (bLargeR Pct) { return 69 }

// ---------------------------------------------------------------------------
// Pooled rates — the aggregate. A weighted average of the two group rates,
// weighted by each treatment's OWN case mix. This is where the reversal lives.
// ---------------------------------------------------------------------------

// Treatment A, pooled across both groups: successes / cases.
func aPooled(aSmall, aLarge int, aSmallR, aLargeR Pct) (aOverall Pct) {
	return pool(aSmall, aLarge, aSmallR, aLargeR)
}

// Treatment B, pooled across both groups.
func bPooled(bSmall, bLarge int, bSmallR, bLargeR Pct) (bOverall Pct) {
	return pool(bSmall, bLarge, bSmallR, bLargeR)
}

// The verdict: does the pooled winner CONTRADICT the per-group winner? That
// contradiction is Simpson's paradox, detected — stated as a fact, not left for
// the reader to spot.
func paradox(aSmallR, aLargeR, bSmallR, bLargeR, aOverall, bOverall Pct) (state Verdict) {
	aWinsSmall := aSmallR > bSmallR
	aWinsLarge := aLargeR > bLargeR
	aWinsPooled := aOverall > bOverall
	// A "clean" subgroup story is one treatment winning both groups.
	aSweeps := aWinsSmall && aWinsLarge
	bSweeps := !aWinsSmall && !aWinsLarge
	reversed := (aSweeps && !aWinsPooled) || (bSweeps && aWinsPooled)
	return Verdict{
		AWinsSmall:  aWinsSmall,
		AWinsLarge:  aWinsLarge,
		AWinsPooled: aWinsPooled,
		Reversed:    reversed,
	}
}

// The table itself — the design deferred to HTML. It gathers the rates, counts,
// pooled figures, and the verdict so the view is a pure projection of the cells.
//
//notebook:height=360 area=table
func table(aSmall, aLarge, bSmall, bLarge int,
	aSmallR, aLargeR, bSmallR, bLargeR, aOverall, bOverall Pct,
	state Verdict) (grid Table) {
	return Table{
		Rows: []Row{
			{Group: "small stones (easy)", ARate: aSmallR, AN: aSmall, BRate: bSmallR, BN: bSmall, AWins: state.AWinsSmall},
			{Group: "large stones (hard)", ARate: aLargeR, AN: aLarge, BRate: bLargeR, BN: bLarge, AWins: state.AWinsLarge},
		},
		AOverall: aOverall, AN: aSmall + aLarge,
		BOverall: bOverall, BN: bSmall + bLarge,
		AWinsPooled: state.AWinsPooled,
		Reversed:    state.Reversed,
	}
}

// A small companion chart: the pooled success bars, so the aggregate has a
// picture beside the table. (A chart alone would hide the paradox — that is the
// whole point — so it plays second to the table.)
//
//notebook:height=200 area=table
func pooledBars(aOverall, bOverall Pct) (bars Chart) {
	return Chart{A: float64(aOverall), B: float64(bOverall)}
}

// Orientation.
func intro() (md Markdown) {
	return `## Simpson's paradox

Treatment **A** wins the small-stone group and the large-stone group. Pool the two
and **B** can come out ahead — because the two treatments were tried on different
mixes of easy and hard cases. Drag the case counts: skew the mix and the TOTAL row
flips while every subgroup row holds. The table is the witness a bar chart can't be.`
}

// ===========================================================================
// Pooling math and helpers (unnamed returns → helpers, not cells).
// ===========================================================================

// pool computes the case-weighted success rate across two groups.
func pool(nSmall, nLarge int, rSmall, rLarge Pct) Pct {
	total := nSmall + nLarge
	if total == 0 {
		return 0
	}
	succ := float64(nSmall)*float64(rSmall) + float64(nLarge)*float64(rLarge)
	return Pct(succ / float64(total))
}

// ===========================================================================
// Types.
// ===========================================================================

// Pct is a success rate in percentage points (0..100).
type Pct float64

// Verdict carries who-won at each level and whether the aggregate reversed.
type Verdict struct {
	AWinsSmall, AWinsLarge, AWinsPooled, Reversed bool
}

// Row is one subgroup row of the table.
type Row struct {
	Group string
	ARate Pct
	AN    int
	BRate Pct
	BN    int
	AWins bool
}

// Table is the whole comparison — subgroup rows plus the pooled TOTAL row and the
// paradox verdict. Rendered as HTML.
type Table struct {
	Rows        []Row
	AOverall    Pct
	AN          int
	BOverall    Pct
	BN          int
	AWinsPooled bool
	Reversed    bool
}

// Render lays the comparison out as an HTML table — the medium the reveal needs.
// The winning treatment's cell in each row is tinted, so the eye reads the win
// column top to bottom: A, A … then the TOTAL row can flip to B. fmt lives here
// (Render, engine-called), never in a cell body. Values are model output.
func (t Table) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<table style="width:100%;border-collapse:collapse;` +
		`font:13px/1.4 -apple-system,system-ui,sans-serif;font-variant-numeric:tabular-nums">`)
	// header
	b.WriteString(`<tr style="color:#5b6472;text-align:left">` +
		`<th style="padding:.5rem .6rem;font-weight:600">group</th>` +
		`<th style="padding:.5rem .6rem;font-weight:600;text-align:right">Treatment A</th>` +
		`<th style="padding:.5rem .6rem;font-weight:600;text-align:right">Treatment B</th></tr>`)
	for _, r := range t.Rows {
		b.WriteString(`<tr style="border-top:1px solid #e7ebf0">`)
		fmt.Fprintf(&b, `<td style="padding:.5rem .6rem;color:#1a1a2e">%s</td>`, r.Group)
		b.WriteString(cell(r.ARate, r.AN, r.AWins))
		b.WriteString(cell(r.BRate, r.BN, !r.AWins))
		b.WriteString(`</tr>`)
	}
	// TOTAL row — bold, top-ruled, the line where the paradox shows.
	b.WriteString(`<tr style="border-top:2px solid #1b3a6b;font-weight:700">`)
	b.WriteString(`<td style="padding:.6rem;color:#1b3a6b">pooled (all cases)</td>`)
	b.WriteString(cellStrong(t.AOverall, t.AN, t.AWinsPooled))
	b.WriteString(cellStrong(t.BOverall, t.BN, !t.AWinsPooled))
	b.WriteString(`</tr>`)
	b.WriteString(`</table>`)

	// The verdict banner.
	if t.Reversed {
		b.WriteString(banner("#fdecea", "#b3261e",
			"Simpson's paradox: A wins every subgroup, B wins the pool."))
	} else {
		b.WriteString(banner("#eafaf1", "#0ca30c",
			"No reversal: the pooled winner agrees with the subgroups."))
	}
	return Rendered{MIME: "text/html", Data: b.String()}
}

// cell renders one treatment's rate + count for a subgroup row; the winner is
// tinted so the win column reads at a glance.
func cell(rate Pct, n int, win bool) string {
	bg := ""
	if win {
		bg = "background:#eef4ff;"
	}
	return fmt.Sprintf(`<td style="padding:.5rem .6rem;text-align:right;%scolor:#1a1a2e">`+
		`%s%%<span style="color:#5b6472;font-size:11px"> · %d cases</span></td>`,
		bg, pct1(rate), n)
}

// cellStrong is cell for the bold pooled row — a heavier tint on the winner.
func cellStrong(rate Pct, n int, win bool) string {
	bg := ""
	color := "#1b3a6b"
	if win {
		bg = "background:#d7e6fb;"
	}
	return fmt.Sprintf(`<td style="padding:.6rem;text-align:right;%scolor:%s">`+
		`%s%%<span style="color:#5b6472;font-size:11px;font-weight:400"> · %d</span></td>`,
		bg, color, pct1(rate), n)
}

// banner renders the verdict strip.
func banner(bg, fg, msg string) string {
	return fmt.Sprintf(`<div style="margin-top:.8rem;padding:.6rem .8rem;border-radius:8px;`+
		`background:%s;color:%s;font:600 13px/1.4 -apple-system,system-ui,sans-serif">%s</div>`, bg, fg, msg)
}

// pct1 formats a percentage to one decimal (no unit).
func pct1(p Pct) string { return strconv.FormatFloat(float64(p), 'f', 1, 64) }

// Chart is the pooled companion bars. Two bars, A and B, 0..100.
type Chart struct{ A, B float64 }

// Render draws the two pooled bars as a minimal SVG — second fiddle to the table.
func (c Chart) Render() Rendered {
	const w, h, pad = 360.0, 150.0, 30.0
	bar := func(x, v float64, color, label string) string {
		bh := (v / 100) * (h - 2*pad)
		y := h - pad - bh
		return fmt.Sprintf(
			`<rect x="%.0f" y="%.1f" width="70" height="%.1f" fill="%s" rx="3"/>`+
				`<text x="%.0f" y="%.1f" font:11px sans-serif font-size="11" fill="#1a1a2e" text-anchor="middle">%s</text>`+
				`<text x="%.0f" y="%.1f" font-size="12" font-weight="700" fill="#1b3a6b" text-anchor="middle">%s%%</text>`,
			x, y, bh, color, x+35, h-pad+16, label, x+35, y-6, strconv.FormatFloat(v, 'f', 1, 64))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" style="max-width:%.0fpx">`, w, h, w)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	b.WriteString(bar(pad+30, c.A, "#2a78d6", "A pooled"))
	b.WriteString(bar(pad+150, c.B, "#0797b8", "B pooled"))
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// Rendered / Markdown, redeclared per notebook.
type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
