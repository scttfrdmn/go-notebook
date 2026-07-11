//go:notebook
//
// Two cells produce the same result symbol `x`. A result name is an edge, so it
// must be unique; the diagnostic points at the second definition and back at
// the first.

package duplicate

// The first producer of x.
func first() (x int) { return 1 }

// The second producer of x — a conflict.
func second() (x int) { return 2 }
