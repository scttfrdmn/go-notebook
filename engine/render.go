package engine

import "reflect"

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

// AsRendered probes v for renderability and returns its rendered form. The bool
// reports whether v was renderable at all — a scalar that does not render falls
// back to a caller-chosen default readout.
//
// The probe is structural and uses reflection, and it must, because of the
// design's central property: a notebook file imports nothing from this project,
// so a cell that renders defines its OWN Rendered-shaped struct (e.g.
// capacity.Rendered) and returns that. Go's interface satisfaction requires an
// exact return-type match, so a static `interface{ Render() engine.Rendered }`
// could never match `Render() capacity.Rendered` — the two named types differ.
// Reflection is therefore not a shortcut here; it is the only way to honor
// "structural probe across independently-defined types."
//
// A value is renderable iff it has a method `Render()` taking no arguments and
// returning a single struct value with string fields named MIME and Data. That
// is the shape the design specifies; any type matching it renders for free,
// with no import and no registration. The reflect cost is one method call per
// rendered output — negligible beside building the SVG/markdown it returns.
func AsRendered(v any) (Rendered, bool) {
	if v == nil {
		return Rendered{}, false
	}
	m := reflect.ValueOf(v).MethodByName("Render")
	if !m.IsValid() {
		return Rendered{}, false
	}
	mt := m.Type()
	if mt.NumIn() != 0 || mt.NumOut() != 1 {
		return Rendered{}, false
	}
	out := m.Call(nil)[0]
	if out.Kind() != reflect.Struct {
		return Rendered{}, false
	}
	mime := out.FieldByName("MIME")
	data := out.FieldByName("Data")
	if !mime.IsValid() || !data.IsValid() ||
		mime.Kind() != reflect.String || data.Kind() != reflect.String {
		return Rendered{}, false
	}
	return Rendered{MIME: mime.String(), Data: data.String()}, true
}
