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
	mimeStr, dataStr := mime.String(), data.String()
	// text/markdown is converted to safe HTML here, at the one chokepoint every
	// rendered output passes through. A notebook returns text/markdown (its
	// intro() does), but the client paints only text/html and image/svg as markup
	// and everything else as raw text — so without this the prose would show its
	// literal **asterisks**. Doing it here means all notebooks get formatted intros
	// with no notebook change, and the client stays a dumb painter. See
	// renderMarkdown for the safe subset and why it is stdlib-only.
	if mimeStr == "text/markdown" {
		return Rendered{MIME: "text/html", Data: renderMarkdown(dataStr)}, true
	}
	return Rendered{MIME: mimeStr, Data: dataStr}, true
}

// AsWidgetView probes v for the Viewable capability and returns its state view.
// Like AsRendered, the probe is structural via reflection, because a notebook
// defines its OWN widget types (e.g. lego.Multi) and imports nothing from this
// package — so a static interface{ WidgetView() engine.WidgetView } could never
// match WidgetView() lego.WidgetView across the two named types. The match is on
// the method name, no args, and one struct result carrying the field shape of
// [WidgetView]; the returned fields are copied by name so the notebook's own
// WidgetView-shaped struct maps onto the engine's.
func AsWidgetView(v any) (WidgetView, bool) {
	if v == nil {
		return WidgetView{}, false
	}
	m := reflect.ValueOf(v).MethodByName("WidgetView")
	if !m.IsValid() {
		return WidgetView{}, false
	}
	mt := m.Type()
	if mt.NumIn() != 0 || mt.NumOut() != 1 {
		return WidgetView{}, false
	}
	out := m.Call(nil)[0]
	if out.Kind() != reflect.Struct {
		return WidgetView{}, false
	}
	// Require at least the Value field to consider it a widget view; the rest are
	// optional per kind.
	vf := out.FieldByName("Value")
	if !vf.IsValid() {
		return WidgetView{}, false
	}
	wv := WidgetView{Value: vf.Interface()}
	if f := out.FieldByName("Options"); f.IsValid() && f.Kind() == reflect.Slice {
		if opts, ok := f.Interface().([]string); ok {
			wv.Options = opts
		}
	}
	wv.Lo = floatPtrField(out, "Lo")
	wv.Hi = floatPtrField(out, "Hi")
	wv.Max = intPtrField(out, "Max")
	return wv, true
}

// floatPtrField reads a *float64 struct field by name (matching the notebook's
// own WidgetView shape), nil if absent or nil. The notebook defines its own
// WidgetView-shaped type, so the field is matched by name and pointer kind.
func floatPtrField(s reflect.Value, name string) *float64 {
	f := s.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.Pointer || f.IsNil() || !f.Elem().CanFloat() {
		return nil
	}
	v := f.Elem().Float()
	return &v
}

// intPtrField reads a *int struct field by name, nil if absent or nil.
func intPtrField(s reflect.Value, name string) *int {
	f := s.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.Pointer || f.IsNil() || !f.Elem().CanInt() {
		return nil
	}
	v := int(f.Elem().Int())
	return &v
}
