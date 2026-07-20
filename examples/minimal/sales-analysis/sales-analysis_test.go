package salesanalysis

import (
	"strings"
	"testing"
)

// TestRowsFilters exercises the parse-and-filter cell: at floor 0 every row
// survives, and at a floor the filter drops the rows below it.
func TestRowsFilters(t *testing.T) {
	if got := len(rows(0)); got != 12 {
		t.Fatalf("rows(0): got %d rows, want 12 (all rows)", got)
	}
	all := rows(0)
	kept := rows(50000)
	if len(kept) >= len(all) {
		t.Fatalf("rows(50000): got %d rows, want fewer than %d (the floor should drop rows)", len(kept), len(all))
	}
	for _, s := range kept {
		if s.Revenue < 50000 {
			t.Errorf("rows(50000) kept a row below the floor: %+v", s)
		}
	}
}

// TestByRegion checks the group-by cell through its rendered output (BarChart's
// fields are unexported): all four regions appear, in the fixed North/South/
// East/West order, when nothing is filtered out.
func TestByRegion(t *testing.T) {
	svg := string(byRegion(rows(0)).Render().Data)
	order := []string{"North", "South", "East", "West"}
	last := -1
	for _, region := range order {
		i := strings.Index(svg, region)
		if i < 0 {
			t.Fatalf("byRegion render is missing region %q", region)
		}
		if i < last {
			t.Errorf("byRegion render has %q out of the North/South/East/West order", region)
		}
		last = i
	}
}
