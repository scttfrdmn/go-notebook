package invoice

import (
	"math"
	"strings"
	"testing"
)

// approx compares two dollar amounts within a cent.
func approx(a, b USD) bool { return math.Abs(float64(a-b)) < 0.01 }

// TestPricingModel pins the arithmetic each line of the bill claims, so the
// invoice is a true projection of the model — not a plausible-looking layout over
// wrong numbers. Defaults: 40 instances × 730 h × $0.96/h.
func TestPricingModel(t *testing.T) {
	n, h, cents := instances(), hours(), rateCents()
	gross := computeGross(n, h, cents)
	if !approx(gross, 28032.00) {
		t.Errorf("gross = %.2f, want 28032.00 (40×730×0.96)", float64(gross))
	}

	// Spot credit: 30% of the fleet at 70% off → -0.30·0.70·gross.
	spot := spotCredit(gross, spotPercent())
	if !approx(spot, USD(-0.30*0.70*float64(gross))) {
		t.Errorf("spot credit = %.2f, want %.2f", float64(spot), -0.30*0.70*float64(gross))
	}
	if spot >= 0 {
		t.Error("spot credit must be a negative line (a saving)")
	}

	// Egress: first TB free, rest at $90/TB → (12-1)·90 = 990.
	egress := egressCost(egressTB())
	if !approx(egress, 990.00) {
		t.Errorf("egress = %.2f, want 990.00 ((12-1)×90)", float64(egress))
	}

	sub := subtotal(gross, spot, egress)
	commit := commitCredit(sub, commitTier())
	if commit >= 0 {
		t.Error("commit credit must be negative (a discount)")
	}
	tax := tax(sub, commit)
	due := total(sub, commit, tax)

	// The total is the discounted subtotal plus tax; it must be less than gross
	// (discounts applied) and positive.
	if due <= 0 || due >= gross {
		t.Errorf("total due = %.2f, want 0 < due < gross(%.2f)", float64(due), float64(gross))
	}
	// The whole chain reconciles: due == sub + commit + tax exactly.
	if !approx(due, sub+commit+tax) {
		t.Errorf("total %.2f != subtotal %.2f + commit %.2f + tax %.2f", float64(due), float64(sub), float64(commit), float64(tax))
	}
}

// TestEgressFirstTBFree pins the "first terabyte is free" rule (an off-by-one is
// exactly the kind of billing bug a chart would hide).
func TestEgressFirstTBFree(t *testing.T) {
	if egressCost(0) != 0 || egressCost(1) != 0 {
		t.Error("0 and 1 TB must both be free")
	}
	if !approx(egressCost(2), 90.00) {
		t.Errorf("2 TB = %.2f, want 90.00 (one billable TB)", float64(egressCost(2)))
	}
}

// TestCommitTiersMonotone confirms a longer commit is a bigger discount: tier 2
// (3-year) beats tier 1 (1-year) beats tier 0 (none).
func TestCommitTiersMonotone(t *testing.T) {
	sub := USD(10000)
	c0, c1, c2 := commitCredit(sub, 0), commitCredit(sub, 1), commitCredit(sub, 2)
	if !(c0 == 0 && c1 < c0 && c2 < c1) {
		t.Errorf("commit credits not monotone by tier: none=%.0f 1yr=%.0f 3yr=%.0f", float64(c0), float64(c1), float64(c2))
	}
}

// TestInvoiceRendersTotals confirms the HTML view actually contains the computed
// figures — the design is deferred to HTML, but the numbers must still reach the
// page (running is not passing: a pretty invoice with no total is a failure).
func TestInvoiceRendersTotals(t *testing.T) {
	n, h, cents := instances(), hours(), rateCents()
	gross := computeGross(n, h, cents)
	spot := spotCredit(gross, spotPercent())
	egress := egressCost(egressTB())
	sub := subtotal(gross, spot, egress)
	commit := commitCredit(sub, commitTier())
	tx := tax(sub, commit)
	due := total(sub, commit, tx)
	inv := bill(n, h, cents, gross, spot, egress, sub, commit, tx, due, spotPercent(), commitTier())

	html := inv.Render()
	if html.MIME != "text/html" {
		t.Fatalf("invoice MIME = %q, want text/html (the design is deferred to HTML)", html.MIME)
	}
	for _, want := range []string{"INVOICE", "TOTAL DUE", "$28,032.00", "$17,490.27"} {
		if !strings.Contains(html.Data, want) {
			t.Errorf("rendered invoice missing %q", want)
		}
	}
	// The credits must render green (the saving reads as a saving).
	if !strings.Contains(html.Data, "#0ca30c") {
		t.Error("credit lines should render in the brand green")
	}
}
