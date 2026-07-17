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

// WireEvent is the transport-facing projection of an [Event]: the flat,
// type-erased shape every transport puts on the wire — cell id, lifecycle state,
// and an optional rendered {mime, data} blob, no Go types. It lives here, beside
// Event, because it is the ONE shape all transports share; the SSE server and the
// WASM bridge previously each declared it separately and kept them in sync by
// hand. This is transport-agnostic on purpose — it names a JSON shape, not a
// protocol — so it crosses no boundary the foreclosure table protects (the engine
// still imports no net/http). The struct's JSON tags are the SSE wire contract;
// [WireEvent.Map] is the same data as a map[string]any for js.ValueOf, so the
// WASM bridge builds it from the same source instead of a parallel literal.
type WireEvent struct {
	Epoch uint64 `json:"epoch"`
	Cell  string `json:"cell"`
	State string `json:"state"`
	MIME  string `json:"mime,omitempty"`
	Data  string `json:"data,omitempty"`
	Err   string `json:"err,omitempty"`
}

// ToWire projects an Event onto its transport shape. The single place the wire
// event is constructed; every transport calls it.
func ToWire(ev Event) WireEvent {
	w := WireEvent{
		Epoch: uint64(ev.Epoch),
		Cell:  string(ev.Cell),
		State: ev.State.String(),
		Err:   ev.Err,
	}
	if ev.Out != nil {
		w.MIME = ev.Out.MIME
		w.Data = ev.Out.Data
	}
	return w
}

// Map renders the wire event as a map[string]any with the same keys and the same
// omit-empty rules as the JSON encoding — for transports (WASM's js.ValueOf) that
// cannot marshal a struct. Kept in lockstep with the struct tags above so the two
// projections never drift: this is the whole point of a single source.
func (w WireEvent) Map() map[string]any {
	m := map[string]any{
		"epoch": float64(w.Epoch),
		"cell":  w.Cell,
		"state": w.State,
	}
	if w.MIME != "" {
		m["mime"] = w.MIME
	}
	if w.Data != "" {
		m["data"] = w.Data
	}
	if w.Err != "" {
		m["err"] = w.Err
	}
	return m
}
