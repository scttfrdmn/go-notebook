//go:notebook
//
// A type mismatch across an edge: producer emits `x int`, consumer wants
// `x string`. Name-matching alone would wire these; type agreement catches it.

package mismatch

// Produces x as an int.
func producer() (x int) { return 1 }

// Consumes x as a string — the types disagree.
func consumer(x string) (y int) { return len(x) }
