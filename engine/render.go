package engine

// Rendered is a MIME-tagged output blob the transport can display without
// knowing any Go types: the client receives {cell, mime, data} and is entirely
// ignorant of the value that produced it.
type Rendered struct {
	// MIME is the content type, e.g. "image/svg+xml" or "text/markdown".
	MIME string
	// Data is the rendered content.
	Data string
	// Grips carries declarative direct-manipulation handles. Always empty this
	// milestone; the field exists so grips are an additive change (a renderer
	// emits them, the runtime binds them to leaf writes) rather than a
	// signature change to every Renderable.
	Grips []Handle
}

// Handle is a direct-manipulation grip: a renderer-emitted, runtime-bound
// reference to a leaf a drag should write. Unused this milestone — the type
// exists only so [Rendered] can carry it without a later breaking change.
type Handle struct {
	// Leaf is the input this grip writes when manipulated.
	Leaf LeafID
}

// Renderable is the output capability: a value that knows how to render itself
// to a display blob. Discovered by structural probe (see [Probe]/[AsRendered]),
// never by a type switch, so a value renders richly without importing anything
// from this package.
type Renderable interface {
	Render() Rendered
}

// AsRendered probes v for the Renderable capability and returns its rendered
// form. The bool reports whether v was renderable at all — a scalar that does
// not implement Render falls back to a caller-chosen default readout.
//
// This is the same structural-probe pattern used for widget capabilities: the
// registry is keyed by capability, not concrete type, so every Renderable
// renders for free.
func AsRendered(v any) (Rendered, bool) {
	if r, ok := v.(Renderable); ok {
		return r.Render(), true
	}
	return Rendered{}, false
}
