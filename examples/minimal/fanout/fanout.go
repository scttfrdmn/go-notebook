//go:notebook
//
// fanout — one result feeding many cells.
//
// A single named result can feed any number of downstream parameters of the same
// name and type. The graph forks: change `r` once and every dependent recomputes.
// No fan-out is declared; it is just several cells taking the same parameter.
//
//	go tool notebook run ./examples/minimal/fanout
//
// Demonstrates: one edge, many consumers (a forking graph).

package fanout

import "math"

// Radius of a circle.
//
//notebook:slider min=0 max=20 step=1
func radius() (r float64) { return 5 }

// Circumference — one consumer of r.
func circumference(r float64) (c float64) { return 2 * math.Pi * r }

// Area — another consumer of r.
func area(r float64) (a float64) { return math.Pi * r * r }

// Diameter — a third consumer of r.
func diameter(r float64) (d float64) { return 2 * r }
