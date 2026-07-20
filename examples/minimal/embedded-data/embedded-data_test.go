package embeddeddata

import (
	"strings"
	"testing"
)

// TestRowsParsesEmbedded exercises the parse cell against the go:embed'd file —
// the bytes are baked in at compile time, so this needs no fixture and no disk.
func TestRowsParsesEmbedded(t *testing.T) {
	sales := rows()
	if len(sales) != 12 {
		t.Fatalf("rows: got %d rows, want 12 (the embedded sales.csv, minus header)", len(sales))
	}
	if sales[0].Region != "North" || sales[0].Revenue != 54000 {
		t.Errorf("first row = %+v, want Region=North Revenue=54000", sales[0])
	}
}

// TestRowsTable confirms the downstream table cell builds from the parsed rows
// (chart.Table's fields are unexported, so assert through the rendered output).
func TestRowsTable(t *testing.T) {
	html := rowsTable(rows()).Render().Data
	if !strings.Contains(html, "Embedded sales.csv") {
		t.Errorf("rowsTable render is missing the title %q", "Embedded sales.csv")
	}
	if !strings.Contains(html, "North") {
		t.Errorf("rowsTable render is missing the embedded data (no North region)")
	}
}
