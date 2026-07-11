//go:notebook
//
// A fixture pinning the cell-vs-helper boundary: a documented helper that names
// no result stays a helper (the doc comment is a label, not a marker), and a
// generic function is never a cell even though it names a result.

package helpers

// A raw input value.
func base() (n int) { return 10 }

// doubled is a real cell: it names its result, so it produces an edge.
func doubled(n int) (m int) { return n * 2 }

// clamp is a HELPER despite this doc comment and despite being called by cells:
// it names no result, so it produces no edge and cannot be a cell.
func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// pick is generic, so it is never a cell even though it names a result: there
// is no concrete result type to wire.
func pick[T any](a, b T, first bool) (chosen T) {
	if first {
		return a
	}
	return b
}

// bounded is a cell that uses both the helper and the generic func internally.
func bounded(m int) (result int) {
	return clamp(pick(m, 0, true), 0, 100)
}
