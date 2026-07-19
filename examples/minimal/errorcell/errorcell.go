//go:notebook
//
// errorcell — a cell that can fail, and a graph that fails partially.
//
// A trailing `error` result is NOT an edge. When a cell returns a non-nil error
// it is marked failed and its downstream cells show "blocked upstream" rather
// than a wrong number — errors are values in the graph, not process death. An
// independent branch that does not depend on the failed cell still computes.
//
// Here: negative coefficients give a negative discriminant, so `roots` fails —
// but `discriminant` (which roots depends on) still shows its value, and fixing
// the inputs restores `roots`. This is the animated graph as a debugger.
//
//	go tool notebook run ./examples/minimal/errorcell
//
// Demonstrates: (value, error) cells, partial failure, blocked-upstream.

package errorcell

import (
	"errors"
	"math"
)

// Coefficient a of a*x^2 + b*x + c.
//
//notebook:slider min=-10 max=10 step=1
func coefA() (a float64) { return 1 }

// Coefficient b.
//
//notebook:slider min=-10 max=10 step=1
func coefB() (b float64) { return 0 }

// Coefficient c.
//
//notebook:slider min=-10 max=10 step=1
func coefC() (c float64) { return -4 }

// The discriminant b^2 - 4ac — always computable, even when the roots are not.
func discriminant(a, b, c float64) (d float64) { return b*b - 4*a*c }

// The two real roots. Fails with a typed error when the discriminant is
// negative (no real roots) — the downstream view is then blocked, but the
// discriminant cell above still displays.
func roots(a, b, d float64) (pair Pair, err error) {
	if a == 0 {
		return Pair{}, errors.New("a = 0: not a quadratic")
	}
	if d < 0 {
		return Pair{}, errors.New("negative discriminant: no real roots")
	}
	s := math.Sqrt(d)
	return Pair{X1: (-b + s) / (2 * a), X2: (-b - s) / (2 * a)}, nil
}

// Pair is the two-root result.
type Pair struct{ X1, X2 float64 }
