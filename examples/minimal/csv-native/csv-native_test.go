package csvnative

import (
	"strings"
	"testing"
)

// TestParseGood parses well-formed records the way rows() would after ReadAll.
func TestParseGood(t *testing.T) {
	recs := [][]string{
		{"region", "quarter", "units", "revenue"},
		{"North", "Q1", "1200", "54000"},
		{"South", "Q1", "890", "40050"},
	}
	sales, err := parse(recs)
	if err != nil {
		t.Fatalf("parse: unexpected error: %v", err)
	}
	if len(sales) != 2 {
		t.Fatalf("parse: got %d rows, want 2", len(sales))
	}
	if sales[0].Units != 1200 || sales[0].Revenue != 54000 {
		t.Errorf("first row = %+v, want Units=1200 Revenue=54000", sales[0])
	}
}

// TestParseBadNumberFails is the recipe's whole point: a value that isn't a
// number is a FAILURE, not a silent zero. If this test ever passes with err==nil,
// the honest-error lesson has been quietly reverted to the strings-based recipes'
// `_`-discard, and the notebook would chart a total missing a row without saying so.
func TestParseBadNumberFails(t *testing.T) {
	recs := [][]string{
		{"region", "quarter", "units", "revenue"},
		{"North", "Q1", "1200", "54000"},
		{"South", "Q1", "eight-ninety", "40050"}, // not a number
	}
	sales, err := parse(recs)
	if err == nil {
		t.Fatalf("parse: got nil error for a non-numeric units field; want a failure "+
			"(a silent zero here is exactly the bug this recipe teaches against). rows=%+v", sales)
	}
	if !strings.Contains(err.Error(), "units") {
		t.Errorf("parse error = %q, want it to name the offending column (units)", err)
	}
}

// TestParseShortRowFails guards the second failure mode: a truncated row is a
// fault, not silently skipped.
func TestParseShortRowFails(t *testing.T) {
	recs := [][]string{
		{"region", "quarter", "units", "revenue"},
		{"North", "Q1", "1200"}, // missing revenue column
	}
	if _, err := parse(recs); err == nil {
		t.Fatal("parse: got nil error for a 3-column row; want a failure")
	}
}
