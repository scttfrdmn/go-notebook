//go:build js && wasm

// Package wasm is the browser transport: it drives an [engine.Runtime] over the
// syscall/js boundary instead of an HTTP/SSE server. It is the WASM sibling of
// engine/server — the ONLY new thing the browser topology needs.
//
// Everything below the transport is unchanged: the same registry, engine,
// scheduler, head, and cache the SSE path uses. This package subscribes to the
// engine's Event channel and pushes each event into a JS callback, and it
// funnels edits through the head's single Set chokepoint. If anything here
// required changing engine's public API, that would be a finding (the
// transport-independence claim would be false); it does not.
//
// # The port
//
// The whole host-facing surface is ONE named object, globalThis.notebook — the
// component API a stranger's page holds. It is plain data both directions; JS
// never sees a Go type. A host that wants its own layout imports nothing of ours
// (no internal/webui, no NB); it reads notebook.meta, calls notebook.set to edit
// a leaf, and notebook.subscribe to receive values — that is the entire contract.
//
//	notebook.meta          []CellMeta — the graph, labels, leaf symbols, widget kinds
//	notebook.provenance    build identity (or null) — what produced this .wasm
//	notebook.set(leaf, v)  edit a leaf (data in); v is a JS scalar/array
//	notebook.subscribe(fn) fn(ev) per cell update (data out); returns an unsubscribe fn
//	                       ev = {epoch, cell, state, mime, data, err}
//	notebook.values()      snapshot {leaf: value} of every cell's latest value (Finals)
//	notebook.start()       run the first wave, so cells paint their defaults
//
// The value channel IS the subscription: every cell's value (a rendered picture
// for eyes, a scalar readout, a widget's state) arrives as an event. There is no
// separate seed channel to poll — subscribe before start and the defaults come
// on the stream. notebook.values() is the same information as a synchronous
// snapshot, for a program that wants to pull rather than subscribe.
package wasm

import (
	"context"
	"encoding/json"
	"sync"
	"syscall/js"

	"github.com/scttfrdmn/go-notebook/engine"
)

// SetFunc coerces a raw JS leaf value to the leaf's static Go type and applies
// it. Generated code supplies it (only codegen knows each leaf's type); a nil
// SetFunc writes the raw value, which is fine when values are already typed.
type SetFunc func(ctx context.Context, rt *engine.Runtime, leaf string, raw any)

// Run wires a runtime to the browser and blocks forever (a wasm main must not
// return). It publishes the port (globalThis.notebook) and pumps engine events
// to every subscriber. It publishes no provenance; use [RunNotebook] to show
// build identity.
func Run(rt *engine.Runtime, meta []engine.CellMeta, set SetFunc) {
	RunNotebook(rt, meta, engine.Provenance{}, set)
}

// RunNotebook is [Run] plus build provenance, exposed as notebook.provenance so
// the host can show what produced this .wasm — the content identity a fixed URL
// cannot convey. Run delegates here with an empty Provenance, so the older
// signature is unchanged.
func RunNotebook(rt *engine.Runtime, meta []engine.CellMeta, prov engine.Provenance, set SetFunc) {
	if set == nil {
		set = func(ctx context.Context, rt *engine.Runtime, leaf string, raw any) {
			rt.Set(ctx, engine.LeafID(leaf), raw)
		}
	}

	p := &port{rt: rt, meta: meta, set: set}

	// One goroutine reads the engine's channel and fans each event out to every
	// JS subscriber. Started before the port is published, so no event a host
	// subscribes for (it subscribes before start()) is lost.
	go p.pump(rt.Subscribe())

	// meta and provenance are published as PARSED JS values (arrays/objects), not
	// JSON strings the host must re-parse: the port hands data, not encodings.
	obj := map[string]any{
		"meta":       jsonToJS(meta),
		"provenance": jsonToJS(prov),
		"set": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) != 2 {
				return nil
			}
			leaf := args[0].String()
			raw := fromJS(args[1])
			go set(context.Background(), rt, leaf, raw)
			return nil
		}),
		"subscribe": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) != 1 || args[0].Type() != js.TypeFunction {
				return nil
			}
			return p.subscribe(args[0])
		}),
		"values": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			// Round-trip through JSON, not js.ValueOf directly: a leaf value is
			// often a NAMED numeric type (PerHour, USD, Probability over float64),
			// which js.ValueOf rejects — its type switch matches only the exact
			// basic kinds. JSON flattens the named type to a plain number, exactly
			// as meta/provenance are published, and can never panic here.
			return jsonToJS(p.values())
		}),
		// The initial wave runs only when the host says so — NOT on a timer. If it
		// ran eagerly its events would race the host building its UI and the first
		// render (the initial chart) would be dropped before its DOM element
		// existed. start() closes that race: the host subscribes, builds, then
		// calls start(), and only then does the first wave paint.
		"start": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			go rt.RunAll(context.Background())
			return nil
		}),
	}
	js.Global().Set("notebook", js.ValueOf(obj))

	select {} // block forever; the JS event loop drives us from here
}

// port holds the JS-facing state: the runtime, the notebook metadata, the leaf
// coercer, and the set of live event subscribers.
type port struct {
	rt   *engine.Runtime
	meta []engine.CellMeta
	set  SetFunc

	mu   sync.Mutex
	subs map[int]js.Value // subscriber id -> JS callback
	next int
}

// subscribe registers a JS callback to receive every subsequent event and
// returns a JS function that unregisters it. Multiple subscribers are supported
// so a host can drive the notebook and observe it from independent listeners;
// the engine's own Subscribe channel stays a single reader (this goroutine).
func (p *port) subscribe(fn js.Value) any {
	p.mu.Lock()
	if p.subs == nil {
		p.subs = map[int]js.Value{}
	}
	id := p.next
	p.next++
	p.subs[id] = fn
	p.mu.Unlock()

	return js.FuncOf(func(_ js.Value, _ []js.Value) any {
		p.mu.Lock()
		delete(p.subs, id)
		p.mu.Unlock()
		return nil
	})
}

// pump forwards engine events to every JS subscriber, one call per event. The
// wire shape is built from the shared [engine.ToWire] projection — the SAME
// source the SSE server encodes — via its Map form, because js.ValueOf cannot
// marshal a struct. This is the single source of truth the two transports share.
func (p *port) pump(sub <-chan engine.Event) {
	for ev := range sub {
		obj := js.ValueOf(engine.ToWire(ev).Map())
		p.mu.Lock()
		subs := make([]js.Value, 0, len(p.subs))
		for _, fn := range p.subs {
			subs = append(subs, fn)
		}
		p.mu.Unlock()
		for _, fn := range subs {
			fn.Invoke(obj)
		}
	}
}

// values returns a snapshot of every leaf's current value, keyed by leaf symbol.
// It reads from Finals (public), so it adds no engine surface, and it is a live
// getter — a host can pull the current state at any time after start() has run a
// wave, with no separate seed channel to poll. The seed values a control starts
// at are the same values that arrive on subscribe; this is the pull form of that
// one channel. The caller (values in the port object) hands the result through
// jsonToJS, which flattens named numeric types to plain numbers.
func (p *port) values() map[string]any {
	finals := p.rt.Finals()
	vals := map[string]any{}
	for _, m := range p.meta {
		if m.Leaf == "" {
			continue
		}
		if v, ok := finals[m.Leaf]; ok {
			vals[string(m.Leaf)] = v
		}
	}
	return vals
}

// jsonToJS round-trips a Go value through JSON into a plain JS value (object,
// array, number, string), so the port hands the host parsed data rather than a
// JSON string it must decode. meta and provenance are the callers; both are
// json-marshalable by construction. A marshal failure yields JS null.
func jsonToJS(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		return nil
	}
	return js.ValueOf(decoded)
}

// fromJS converts a JS value to the plain Go value the head stores. JS numbers
// arrive as float64 and bools as bool — the same shapes the SSE /set path sees,
// so the generated coercer treats them identically.
func fromJS(v js.Value) any {
	switch v.Type() {
	case js.TypeNumber:
		return v.Float()
	case js.TypeBoolean:
		return v.Bool()
	case js.TypeString:
		return v.String()
	case js.TypeObject:
		// A JS array becomes []any (the same shape the SSE path's JSON decode
		// produces), so a widget selection — a Multi's labels, a Draggable's flat
		// point floats — reaches engine.CoerceWire and homogenizes like it does on
		// the server. Without this a selection array stringified and the widget
		// write path silently did nothing (it worked over SSE, not in the browser).
		if v.InstanceOf(js.Global().Get("Array")) {
			n := v.Length()
			out := make([]any, n)
			for i := 0; i < n; i++ {
				out[i] = fromJS(v.Index(i))
			}
			return out
		}
		return v.String()
	default:
		return v.String()
	}
}
