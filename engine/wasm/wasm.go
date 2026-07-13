//go:build js && wasm

// Package wasm is the browser transport: it drives an [engine.Runtime] over the
// syscall/js boundary instead of an HTTP/SSE server. It is the WASM sibling of
// engine/server — the ONLY new thing the browser topology needs.
//
// Everything below the transport is unchanged: the same registry, engine,
// scheduler, head, and cache the SSE path uses. This package subscribes to the
// engine's Event channel and pushes each event into a JS callback, and it
// registers a JS-callable function that funnels edits through the head's single
// Set chokepoint. If anything here required changing engine's public API, that
// would be a finding (the transport-independence claim would be false); it does
// not.
//
// The JS contract (both directions are plain data — JS never sees a Go type):
//
//	globalThis.__notebook_meta   → JSON string of []CellMeta, set once at start
//	globalThis.__notebook_event(ev)  ← called per cell update: {epoch,cell,state,mime,data,err}
//	globalThis.notebookSet(leaf, value)  → JS calls this to edit a leaf
package wasm

import (
	"context"
	"encoding/json"
	"syscall/js"

	"github.com/scttfrdmn/go-notebook/engine"
)

// SetFunc coerces a raw JS leaf value to the leaf's static Go type and applies
// it. Generated code supplies it (only codegen knows each leaf's type); a nil
// SetFunc writes the raw value, which is fine when values are already typed.
type SetFunc func(ctx context.Context, rt *engine.Runtime, leaf string, raw any)

// Run wires a runtime to the browser and blocks forever (a wasm main must not
// return). It publishes the cell metadata, starts pumping events to JS, runs an
// initial wave so the page renders, and installs the JS-callable set function.
// It publishes no provenance; use [RunNotebook] to show build identity.
func Run(rt *engine.Runtime, meta []engine.CellMeta, set SetFunc) {
	RunNotebook(rt, meta, engine.Provenance{}, set)
}

// RunNotebook is [Run] plus build provenance, published to JS as
// __notebook_provenance so the page can show what produced this .wasm — the
// content identity that a fixed URL cannot convey. Run delegates here with an
// empty Provenance, so the older signature is unchanged.
func RunNotebook(rt *engine.Runtime, meta []engine.CellMeta, prov engine.Provenance, set SetFunc) {
	if set == nil {
		set = func(ctx context.Context, rt *engine.Runtime, leaf string, raw any) {
			rt.Set(ctx, engine.LeafID(leaf), raw)
		}
	}

	// Publish metadata once (labels, leaf symbols, directives) as JSON.
	if b, err := json.Marshal(meta); err == nil {
		js.Global().Set("__notebook_meta", string(b))
	}
	// Publish provenance for the page footer (best-effort; empty is fine).
	if b, err := json.Marshal(prov); err == nil {
		js.Global().Set("__notebook_provenance", string(b))
	}

	// Pump events → JS. A goroutine reads the engine's channel and calls the
	// JS sink for each event; JS renders {cell, mime, data}.
	go pump(rt.Subscribe())

	// Install the edit entry point: JS calls notebookSet(leaf, value).
	js.Global().Set("notebookSet", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 {
			return nil
		}
		leaf := args[0].String()
		raw := fromJS(args[1])
		go set(context.Background(), rt, leaf, raw)
		return nil
	}))

	// The initial wave runs only when the client says its cell elements exist —
	// NOT on a timer. If we ran it on __notebook_ready, its events would race
	// buildUI() and the first render (the initial chart) would be dropped before
	// the DOM element existed. notebookStart() closes that race: the client
	// calls it after building the UI, and only then does the first wave paint.
	js.Global().Set("notebookStart", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		go func() {
			rt.RunAll(context.Background())
			// The initial wave ran every leaf's body for its default; those values
			// are now in Finals under each leaf symbol. Publish them so the client
			// can show each control's starting value instead of a blank readout.
			// Finals is already public — no new engine surface.
			publishLeaves(rt, meta)
		}()
		return nil
	}))

	// Signal that meta + functions are installed; the client builds its UI and
	// then calls notebookStart to trigger the first wave.
	js.Global().Set("__notebook_ready", js.ValueOf(true))

	select {} // block forever; the JS event loop drives us from here
}

// pump forwards engine events to the JS sink, one call per event.
func pump(sub <-chan engine.Event) {
	sink := js.Global().Get("__notebook_event")
	for ev := range sub {
		obj := map[string]any{
			"epoch": float64(ev.Epoch),
			"cell":  string(ev.Cell),
			"state": ev.State.String(),
		}
		if ev.Out != nil {
			obj["mime"] = ev.Out.MIME
			obj["data"] = ev.Out.Data
		}
		if ev.Err != "" {
			obj["err"] = ev.Err
		}
		// Re-fetch the sink each time in case JS installed it after start.
		if !sink.Truthy() {
			sink = js.Global().Get("__notebook_event")
		}
		if sink.Type() == js.TypeFunction {
			sink.Invoke(js.ValueOf(obj))
		}
	}
}

// publishLeaves hands the client each leaf's current value, keyed by leaf
// symbol, via globalThis.__notebook_leaves. The client reads it to seed each
// control's initial position and readout — otherwise a slider sits at a browser
// default and the readout is blank until first drag. Read from Finals (public),
// so this adds no engine surface.
func publishLeaves(rt *engine.Runtime, meta []engine.CellMeta) {
	finals := rt.Finals()
	vals := map[string]any{}
	for _, m := range meta {
		if m.Leaf == "" {
			continue
		}
		if v, ok := finals[m.Leaf]; ok {
			vals[string(m.Leaf)] = jsValue(v)
		}
	}
	if b, err := json.Marshal(vals); err == nil {
		js.Global().Set("__notebook_leaves", string(b))
	}
	if fn := js.Global().Get("__notebook_leaves_ready"); fn.Type() == js.TypeFunction {
		fn.Invoke()
	}
}

// jsValue reduces a leaf value to a JSON-friendly scalar. Leaf defaults are the
// scalar controls (numbers, bools, strings); a named numeric type like PerHour
// is an untyped number to JSON, which is exactly what the control wants.
func jsValue(v any) any {
	switch x := v.(type) {
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return x
	}
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
	default:
		return v.String()
	}
}
