//go:notebook
//
// fanin — many results feeding one cell.
//
// A cell can take several parameters, each wired from a different producer by
// name and type. The graph joins: `bmi` depends on both `weight` and `height`
// and recomputes when either changes. This is the mirror of fanout.
//
//	go tool notebook run ./examples/minimal/fanin
//
// Demonstrates: many edges into one consumer (a joining graph).

package fanin

// Body mass in kilograms.
//
//notebook:slider min=30 max=150 step=1
func weight() (kg float64) { return 70 }

// Height in metres.
//
//notebook:slider min=1 max=2.2 step=0.01
func height() (m float64) { return 1.75 }

// Body-mass index — joins weight and height.
func bmi(kg, m float64) (index float64) { return kg / (m * m) }
