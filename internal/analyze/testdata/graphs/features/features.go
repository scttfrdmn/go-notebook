//go:notebook
//
// A synthetic fixture exercising IR features the gallery examples don't all
// show together: a multi-output cell, a trailing-error failure channel, and an
// injected context.Context parameter.

package features

import "context"

// A raw seed value.
func seed() (n int) { return 42 }

// Splits a seed into two independent streams. A multi-output cell: two named
// results, two edges.
func split(n int) (lo int, hi int) { return n / 2, n - n/2 }

// Loads rows, and may fail. The trailing error is not an edge; it is the
// failure channel that blocks downstream cells.
func load(ctx context.Context, lo int) (rows []int, err error) {
	return []int{lo}, nil
}

// Sums the loaded rows against the high stream.
func total(rows []int, hi int) (sum int) {
	s := hi
	for _, r := range rows {
		s += r
	}
	return s
}
