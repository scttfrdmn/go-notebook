package multifilepackage

import "strconv"

// Book is the domain type — one title in the catalog. It lives in its own file,
// the way a real package keeps its model separate from the code that reads it.
//
// Nothing here mentions notebooks. This is ordinary package code, and the
// notebook's cells (in nb.go) use it freely: cell DISCOVERY only scans the
// //go:notebook file, but the Go TYPE CHECKER spans the whole package, so a cell
// can name Book, call its methods, and wire a []Book edge — no import, no
// re-declaration. It is one package.
type Book struct {
	Title  string
	Year   int
	Pages  int
	Rating float64
}

// Decade returns the book's decade as a label, e.g. "1990s". A method on the
// domain type, called from the byDecade cell in nb.go — the cell reaches across
// files for it, which is exactly the point. strconv (not fmt) keeps every cell
// that transitively calls this on the WASM-able path.
func (b Book) Decade() string {
	d := (b.Year / 10) * 10
	return strconv.Itoa(d) + "s"
}

// Long reports whether the book is a long read. The filter predicate lives with
// the type it tests, not spelled out inline in the cell — again, the cell in
// nb.go simply calls b.Long(pages) across the file boundary.
func (b Book) Long(minPages int) bool { return b.Pages >= minPages }
