package multifilepackage

import (
	"strings"
	"testing"
)

// These tests are the recipe's anti-pass: each one exercises a cell in nb.go
// that reaches ACROSS a file boundary — to catalog() in data.go, or to a Book
// method in model.go. If the multi-file wiring the example teaches ever silently
// broke (a cell stopped seeing a sibling type, a method changed behavior), the
// composed result would change and the matching test would go red. They pass
// only because it really is one package: the cells call sibling code directly,
// as ordinary Go.

// TestLoadCatalogReadsSiblingData confirms the loadCatalog cell (nb.go) produces
// its edge from catalog() (data.go): the parser in the other file runs and yields
// the whole catalog. Empty out catalog() and this goes red.
func TestLoadCatalogReadsSiblingData(t *testing.T) {
	books := loadCatalog()
	if len(books) != 8 {
		t.Fatalf("loadCatalog() = %d books, want 8 (does catalog() in data.go parse the rows?)", len(books))
	}
	if books[0].Title != "The Go Programming Language" || books[0].Pages != 380 {
		t.Errorf("first book = %+v, want the Go book with 380 pages", books[0])
	}
}

// TestSummaryUsesBookLong is the cross-file anti-pass on the OUT side: the
// summary cell (nb.go) counts long reads via Book.Long (model.go). The count is a
// function of BOTH files — the catalog from data.go and the predicate from
// model.go — so the exact "5 of 8" pins the whole cross-file chain. Break
// Book.Long's threshold or empty the catalog and this fails.
func TestSummaryUsesBookLong(t *testing.T) {
	// pages ≥ 400: 657, 448, 464, 616, 960 → 5 of the 8 titles.
	if got := summary(loadCatalog(), 400).Value; got != "5 of 8" {
		t.Fatalf("summary at floor 400 = %q, want %q (Book.Long from model.go over the catalog from data.go)", got, "5 of 8")
	}
	// The floor really drives the count — every book is a long read at 0.
	if got := summary(loadCatalog(), 0).Value; got != "8 of 8" {
		t.Errorf("summary at floor 0 = %q, want %q", got, "8 of 8")
	}
	// And none qualifies past the longest book (960 pages).
	if got := summary(loadCatalog(), 1000).Value; got != "0 of 8" {
		t.Errorf("summary at floor 1000 = %q, want %q", got, "0 of 8")
	}
}

// TestByDecadeUsesBookDecade pins the other sibling method: byDecade (nb.go)
// buckets titles with Book.Decade (model.go). The rendered bars must carry every
// decade the catalog spans; a broken Decade() would drop or mislabel them, and
// the render would no longer contain these strings.
func TestByDecadeUsesBookDecade(t *testing.T) {
	svg := byDecade(loadCatalog()).Render().Data
	// Years 1975/1985/1999/1999/2004/2008/2015/2017 → these five decades.
	for _, decade := range []string{"1970s", "1980s", "1990s", "2000s", "2010s"} {
		if !strings.Contains(svg, decade) {
			t.Errorf("byDecade render is missing %q — is Book.Decade (model.go) bucketing correctly?", decade)
		}
	}
}

// TestLongReadsFiltersByFloor guards the longReads cell end to end: at a high
// floor it keeps strictly fewer rows than at floor 0, and the render still
// carries a book that clears the floor. Filtering is Book.Long (model.go) applied
// to the catalog (data.go) inside a cell (nb.go) — three files, one pipeline.
func TestLongReadsFiltersByFloor(t *testing.T) {
	all := longReads(loadCatalog(), 0).Render().Data
	few := longReads(loadCatalog(), 600).Render().Data
	if len(few) >= len(all) {
		t.Fatalf("longReads at floor 600 rendered %d bytes, not fewer than %d at floor 0 — the floor should drop rows", len(few), len(all))
	}
	// "Code Complete" (960 pages) clears the 600 floor and must survive.
	if !strings.Contains(few, "Code Complete") {
		t.Error("longReads at floor 600 dropped a 960-page book that clears the floor")
	}
}
