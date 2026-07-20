package fileandartifact

import (
	"strings"
	"testing"
)

// TestRowsReadsEmbeddedData confirms the IN boundary: go:embed folded
// downloads.csv into the binary, so Rows() parses it with no file open. Twelve
// months, each with a positive count.
func TestRowsReadsEmbeddedData(t *testing.T) {
	rows := Rows()
	if len(rows) != 12 {
		t.Fatalf("Rows() = %d rows, want 12 (is downloads.csv embedded?)", len(rows))
	}
	if rows[0].Month != "Jan" || rows[0].Downloads <= 0 {
		t.Errorf("first row = %+v, want Jan with a positive count", rows[0])
	}
}

// TestChartIsPure is the recipe's whole point on the OUT side: Chart is a pure
// function, so the SAME rows must render byte-identical SVG every time. If this
// ever fails, some impurity (a clock, a random layout jitter, a global) has leaked
// into a cell — and the "./report can run headless and get a stable file" claim,
// the reason artifact-writing lives OUTSIDE the graph rather than in a cell, is a
// lie. A pure cell that isn't pure is exactly the bug the graph exists to prevent.
func TestChartIsPure(t *testing.T) {
	rows := Rows()
	first := Chart(rows).Render().Data
	second := Chart(rows).Render().Data
	if first != second {
		t.Fatalf("Chart(rows).Render() is not deterministic: two renders of the same "+
			"rows differ (%d vs %d bytes). A cell must be a pure function of its inputs.",
			len(first), len(second))
	}
}

// TestReportPipelineProducesSVG exercises exactly what ./report does — call the
// exported cells directly and read the SVG bytes — and confirms the result is a
// standalone SVG artifact (has the <svg> root and carries the data). This is the
// "a notebook is an ordinary Go package" claim as a test: no runtime, no server.
func TestReportPipelineProducesSVG(t *testing.T) {
	svg := Chart(Rows()).Render().Data
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "</svg>") {
		t.Fatalf("Chart output is not a standalone SVG (got %.40q…)", svg)
	}
	if !strings.Contains(svg, "Jan") || !strings.Contains(svg, "Monthly downloads") {
		t.Error("SVG should carry the data (a month label and the title)")
	}
}

// TestTotalSumsRows guards the second downstream cell: the headline number is the
// sum of the embedded counts, not a placeholder.
func TestTotalSumsRows(t *testing.T) {
	got := Total(Rows()).Value
	// 4200+4850+5100+4700+5600+6100+5900+6400+7200+6800+7500+8100 = 72450
	if got != "72450" {
		t.Errorf("Total = %q, want %q", got, "72450")
	}
}
