package engine

// State is the lifecycle state of a cell within a wave, as reported on the
// event stream.
type State int

const (
	// StateRunning means the cell has started executing in the current wave.
	StateRunning State = iota
	// StateDone means the cell completed and produced a value.
	StateDone
	// StateError means the cell returned an error or panicked; its downstream
	// is blocked rather than fed a wrong value.
	StateError
	// StateBlocked means an upstream cell failed, so this cell did not run. It
	// shows "blocked upstream" rather than a stale or wrong number.
	StateBlocked
	// StateStale means the cell's wave was superseded by a newer epoch before
	// it committed. Its result is discarded.
	StateStale
)

// String renders a State for logs and tests.
func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateDone:
		return "done"
	case StateError:
		return "error"
	case StateBlocked:
		return "blocked"
	case StateStale:
		return "stale"
	default:
		return "unknown"
	}
}

// Event is a single cell state transition within a wave. The engine emits these
// on the channel returned by [Runtime.Subscribe]; engine/server (and headless
// drivers) consume them. The engine itself never imports a transport — this
// channel is the whole seam that keeps headless, WASM, and batch modes free.
type Event struct {
	Epoch Epoch
	Cell  CellID
	State State
	// Out is the cell's rendered output, non-nil only when the cell's value is
	// Renderable and the cell reached StateDone.
	Out *Rendered
	// Err is the error message when State is StateError.
	Err string
}
