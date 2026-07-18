//go:notebook
//
// The same file is a notebook, a job, and the bill you hand finance.
//
// Every other notebook renders a chart. This one renders a DOCUMENT — a cloud
// invoice — because that is the form the answer actually takes. The pricing is
// ordinary Go: instance-hours × rate, a spot discount, egress, a committed-use
// discount, tax. Drag the knobs and the invoice re-totals live.
//
// The point on stage: **presentation deferred to HTML.** A cost model's output is
// not a line chart — it is a receipt, with line items, a subtotal, a discount, and
// a total in a box. HTML/CSS is the right medium for that, so the view is authored
// as HTML in Render() rather than forced into the default readout. The escape
// hatch the design doc quarantines for WebGL is used here for a humbler, more
// common reason: **HTML is simply the better design surface for a document.**
//
// The seam stays honest, exactly as surface/gpulife keep it: **Go owns the
// arithmetic** (pure cells, in the dependency graph, unit-typed so dollars and
// hours cannot cross); **HTML owns the layout** (the Render() string computes no
// price, only how to present one). Strip the styling and the same numbers are
// still there — the invoice is a projection of the cells, not a second source.
//
// And because it is the same file: `notebook build ./invoice && ./invoice
// --headless --json` prints the very same totals as a batch job. The figure your
// finance team approves and the number your pipeline emits are one artifact.
//
// Arranged as a dashboard: the knobs beside the bill they drive, the invoice the
// hero. The intermediate line-item cells (computeGross, spotCredit, …) fall below
// in source order — the model's working, shown, not hidden. Strip the layout
// lines and it degrades to the plain stack.
//
//notebook:layout intro
//notebook:layout knobs | bill

package invoice

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs — the knobs on a cloud bill.
// ---------------------------------------------------------------------------

// Number of instances in the fleet.
//
//notebook:slider min=1 max=500 step=1 area=knobs
func instances() (n int) { return 40 }

// Hours each instance runs this month (730 = always-on).
//
//notebook:slider min=1 max=730 step=1 area=knobs
func hours() (h int) { return 730 }

// On-demand price per instance-hour, in cents (so the slider is integer-clean).
//
//notebook:slider min=1 max=1000 step=1 area=knobs
func rateCents() (cents int) { return 96 }

// Fraction of the fleet running on spot capacity (spot ≈ 70% off on-demand).
//
//notebook:slider min=0 max=100 step=1 area=knobs
func spotPercent() (pct int) { return 30 }

// Outbound data transfer this month, in terabytes (egress is the line nobody budgets).
//
//notebook:slider min=0 max=200 step=1 area=knobs
func egressTB() (tb int) { return 12 }

// Committed-use discount tier: 0 = none, 1 = 1-year (~30% off), 2 = 3-year (~55% off).
//
//notebook:slider min=0 max=2 step=1 area=knobs
func commitTier() (tier int) { return 1 }

// ---------------------------------------------------------------------------
// The pricing model — pure Go, unit-typed. Each line of the bill is a cell.
// ---------------------------------------------------------------------------

// Compute cost before any discount: the on-demand fleet, per hour, times hours.
func computeGross(n, h, cents int) (gross USD) {
	return USD(float64(n) * float64(h) * float64(cents) / 100.0)
}

// Spot savings: the spot slice of the fleet costs ~30% of on-demand, so the
// saving is 70% of that slice's gross. A credit (negative line) on the bill.
// The result is named `spot` — a result name IS the edge, so it must match the
// parameter every consumer reads it by.
func spotCredit(gross USD, pct int) (spot USD) {
	return USD(-float64(gross) * float64(pct) / 100.0 * spotDiscount)
}

// Egress cost: the first terabyte is free, the rest bill at a flat per-TB rate.
func egressCost(tb int) (egress USD) {
	billable := tb - 1
	if billable < 0 {
		billable = 0
	}
	return USD(float64(billable) * egressPerTB)
}

// The subtotal: compute (net of spot) plus egress, before the committed-use
// discount and tax.
func subtotal(gross, spot, egress USD) (sub USD) {
	return gross + spot + egress
}

// Committed-use discount: a credit on the subtotal, sized by the tier.
func commitCredit(sub USD, tier int) (commit USD) {
	return USD(-float64(sub) * commitRate(tier))
}

// Tax on the discounted subtotal (a flat rate, applied last).
func tax(sub, commit USD) (t USD) {
	return USD(float64(sub+commit) * taxRate)
}

// The total due — the number on the last line, the one finance approves.
func total(sub, commit, t USD) (due USD) {
	return sub + commit + t
}

// The bill itself: a value that renders as an HTML invoice. It gathers every
// computed line so the view is a pure projection of the cells above — the
// Render() method arranges them, it does not price anything.
//
//notebook:height=520 area=bill
func bill(n, h, cents int, gross, spot, egress, sub, commit, t, due USD, pct, tier int) (statement Invoice) {
	return Invoice{
		Lines: []Line{
			{Desc: itoa(n) + " instances × " + itoa(h) + " h × $" + money2(float64(cents)/100) + "/h", Amount: gross},
			{Desc: "spot capacity credit (" + itoa(pct) + "% of fleet at ~70% off)", Amount: spot, Credit: true},
			{Desc: "outbound data transfer (egress)", Amount: egress},
		},
		Subtotal:     sub,
		CommitLabel:  commitLabel(tier),
		CommitCredit: commit,
		TaxLabel:     "tax (" + money2(taxRate*100) + "%)",
		Tax:          t,
		Total:        due,
	}
}

// A one-line orientation shown above the bill.
func intro() (md Markdown) {
	return `## Cloud bill

Drag the knobs on the left. The invoice on the right re-totals live — this is the
same Go file you would run as a batch job to emit the number your pipeline reports.
The chart-less notebook: the answer here is a **document**, so the view is HTML.`
}

// ===========================================================================
// Constants — the model's fixed rates. Ordinary Go, not cells.
// ===========================================================================

const (
	spotDiscount = 0.70 // spot runs at ~30% of on-demand → 70% saved on that slice
	egressPerTB  = 90.0 // $ per TB of outbound transfer after the first free TB
	taxRate      = 0.08 // flat sales tax on the discounted subtotal
)

// commitRate maps a committed-use tier to its discount fraction.
func commitRate(tier int) float64 {
	switch tier {
	case 1:
		return 0.30 // 1-year commit
	case 2:
		return 0.55 // 3-year commit
	default:
		return 0.0
	}
}

// commitLabel is the human name for a committed-use tier, for the invoice line.
func commitLabel(tier int) string {
	switch tier {
	case 1:
		return "committed use — 1 year (30% off)"
	case 2:
		return "committed use — 3 year (55% off)"
	default:
		return "committed use — none"
	}
}

// ===========================================================================
// Types. A notebook imports nothing from the project; these are redeclared per
// file and discovered by the engine through method shape (Render, Bounds, …).
// ===========================================================================

// USD is dollars. A named float64 so the model reads in the unit a bill is in;
// the engine formats it like its float64 underlying without knowing the type.
type USD float64

// Line is one row of the invoice. Credit marks a negative (a saving), drawn in
// green and with a minus, so the bill reads the way a real one does.
type Line struct {
	Desc   string
	Amount USD
	Credit bool
}

// Invoice is the whole statement: the line items and the running totals. It is a
// pure record of the computed cells; Render() lays it out as HTML.
type Invoice struct {
	Lines        []Line
	Subtotal     USD
	CommitLabel  string
	CommitCredit USD
	TaxLabel     string
	Tax          USD
	Total        USD
}

// Render emits the invoice as styled HTML — the design deferred to the medium
// that fits a receipt. fmt lives here (in Render, engine-called) not in a cell
// body, so the fmt→os WASM gate never trips. It computes no price: every number
// arrives already totalled from the cells above. Values are model output (no
// user text), so no escaping is needed.
func (inv Invoice) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="font:14px/1.5 -apple-system,system-ui,sans-serif;max-width:460px;` +
		`border:1px solid #e7ebf0;border-radius:12px;overflow:hidden">`)
	b.WriteString(`<div style="background:#1b3a6b;color:#fff;padding:.9rem 1.1rem;font-weight:700;` +
		`font-size:15px;letter-spacing:.02em">INVOICE · cloud fleet</div>`)
	b.WriteString(`<table style="width:100%;border-collapse:collapse;font-variant-numeric:tabular-nums">`)

	for _, ln := range inv.Lines {
		b.WriteString(row(ln.Desc, ln.Amount, ln.Credit, false))
	}
	b.WriteString(rule())
	b.WriteString(row("subtotal", inv.Subtotal, false, false))
	if float64(inv.CommitCredit) != 0 {
		b.WriteString(row(inv.CommitLabel, inv.CommitCredit, true, false))
	}
	b.WriteString(row(inv.TaxLabel, inv.Tax, false, false))
	b.WriteString(rule())
	b.WriteString(row("TOTAL DUE", inv.Total, false, true))

	b.WriteString(`</table></div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

// row renders one invoice line: description left, amount right. A credit shows a
// minus and reads green; the total row is bold and larger.
func row(desc string, amt USD, credit, total bool) string {
	descStyle := "padding:.5rem 1.1rem;color:#1a1a2e"
	amtStyle := "padding:.5rem 1.1rem;text-align:right;color:#1a1a2e;white-space:nowrap"
	if credit {
		amtStyle = "padding:.5rem 1.1rem;text-align:right;color:#0ca30c;white-space:nowrap"
	}
	if total {
		descStyle = "padding:.7rem 1.1rem;color:#1b3a6b;font-weight:700;font-size:16px"
		amtStyle = "padding:.7rem 1.1rem;text-align:right;color:#1b3a6b;font-weight:700;font-size:16px;white-space:nowrap"
	}
	return fmt.Sprintf(`<tr><td style="%s">%s</td><td style="%s">%s</td></tr>`,
		descStyle, desc, amtStyle, dollars(amt))
}

// rule is a hairline separator row spanning both columns.
func rule() string {
	return `<tr><td colspan="2" style="padding:0"><div style="border-top:1px solid #e7ebf0"></div></td></tr>`
}

// dollars formats a USD as -$1,234.56 (credits carry the sign, drawn green by row).
func dollars(v USD) string {
	neg := v < 0
	f := float64(v)
	if neg {
		f = -f
	}
	s := "$" + money2(f)
	if neg {
		return "−" + s
	}
	return s
}

// money2 formats a float as a 2-decimal, comma-grouped number (no unit).
func money2(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	dot := strings.IndexByte(s, '.')
	intPart, frac := s[:dot], s[dot:]
	neg := strings.HasPrefix(intPart, "-")
	if neg {
		intPart = intPart[1:]
	}
	// Insert thousands separators.
	var g strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			g.WriteByte(',')
		}
		g.WriteRune(c)
	}
	out := g.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// itoa is strconv.Itoa under a short name (kept out of cell bodies is unnecessary
// here — these are helpers, not cells — but strconv keeps the cell graph clean).
func itoa(n int) string { return strconv.Itoa(n) }

// Rendered is a MIME-tagged blob the runtime displays.
type Rendered struct {
	MIME string
	Data string
}

// Markdown renders as prose.
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
