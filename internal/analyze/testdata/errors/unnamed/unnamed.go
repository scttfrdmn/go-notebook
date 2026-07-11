//go:notebook
//
// A documented cell that names one result but leaves another unnamed. This is
// the ambiguous case the "unnamed result" diagnostic exists for: the name is
// the edge, so a partially-named cell can't be wired reliably.
//
// (A cell whose results are ALL unnamed is treated as a helper, not flagged —
// see the erlangC/svg helpers in the capacity fixture.)

package unnamed

// Produces a named value and an unnamed one.
func partial() (named int, _ string) { return 1, "x" }
