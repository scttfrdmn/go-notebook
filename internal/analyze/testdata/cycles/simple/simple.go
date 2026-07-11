//go:notebook
//
// A dependency cycle among non-delayed edges: a -> b -> c -> a. This is
// structurally unschedulable; a genuine feedback loop would need Prev[T].

package simple

// Consumes z, produces x.
func a(z int) (x int) { return z + 1 }

// Consumes x, produces y.
func b(x int) (y int) { return x + 1 }

// Consumes y, produces z — closing the loop.
func c(y int) (z int) { return y + 1 }
