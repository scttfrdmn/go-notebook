//go:notebook
//
// A notebook using a deferred feature (a Prev[T] fold). This is not an error:
// the fold cell is skipped with a "notice", and the rest of the notebook still
// builds. Pins the Notice-severity diagnostic rendering.

package fold

// Prev holds a cell's own previous output — the fold marker.
type Prev[T any] struct{ Value T }

// Tick is the clock a fold steps on.
type Tick uint64

// A clock leaf.
func clock() (tick Tick) { return 0 }

// A plain input.
func rate() (r int) { return 10 }

// counter is a fold: it reads its own previous output. Unsupported this
// milestone — expect a notice, not an error.
func counter(prev Prev[int], tick Tick, r int) (total int) {
	return prev.Value + r
}
