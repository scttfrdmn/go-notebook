package multifilepackage

import (
	"strconv"
	"strings"
)

// The catalog, inlined as a string so the whole notebook stays WASM-able (no
// file, no socket — see the file-and-artifact recipe for the go:embed and
// os.Open variants of this same seam).
const catalogCSV = `title,year,pages,rating
The Go Programming Language,2015,380,4.6
Structure and Interpretation,1985,657,4.7
The Pragmatic Programmer,1999,352,4.3
Refactoring,1999,448,4.2
Clean Code,2008,464,3.9
Designing Data-Intensive Applications,2017,616,4.8
The Mythical Man-Month,1975,322,4.1
Code Complete,2004,960,4.4`

// catalog parses the inlined rows into typed Books. It is an ordinary package
// helper that the loadCatalog cell in nb.go calls.
//
// DELIBERATE GOTCHA — read this carefully: this func NAMES its result (`books`),
// which is the exact shape that makes a func a CELL in nb.go. Here it is NOT a
// cell, because cell discovery scans ONLY the //go:notebook file — this sibling
// file is invisible to discovery. So a helper in a non-notebook file is free to
// name its returns (which reads better) without being mistaken for a cell.
//
// Move this func into nb.go and discovery WOULD pick it up as a cell — the file
// is the boundary discovery honors, not the function. (As it happens, its result
// is named `books`, which loadCatalog already produces, so in nb.go it would also
// fail the "a result name is a unique edge" rule — a second reminder that in the
// notebook file the named result is not just a return value, it is a wire.)
func catalog() (books []Book) {
	lines := strings.Split(strings.TrimSpace(catalogCSV), "\n")
	for _, line := range lines[1:] { // skip the header row
		rec := strings.Split(line, ",")
		if len(rec) < 4 {
			continue
		}
		year, _ := strconv.Atoi(strings.TrimSpace(rec[1]))
		pages, _ := strconv.Atoi(strings.TrimSpace(rec[2]))
		rating, _ := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		books = append(books, Book{Title: rec[0], Year: year, Pages: pages, Rating: rating})
	}
	return books
}
