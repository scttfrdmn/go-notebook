package engine

import (
	"context"
	"strings"
	"testing"
)

// usd is a named type over float64 — a domain unit, no method. It must format
// through the scalar readout by its underlying kind, exactly like capacity's
// PerHour/Seconds, without the engine knowing the named type.
type usd float64

// unit is a domain type that satisfies Bounded structurally — no import of this
// package is needed for a value to be a slider.
type unit float64

func (unit) Bounds() (float64, float64) { return 0, 1 }

// rng is a Reconciler that clamps a saved value into its bounds.
type rng struct{ lo, hi float64 }

func (r rng) Reconcile(saved any) any {
	v, ok := saved.(float64)
	if !ok {
		return saved
	}
	if v < r.lo {
		return r.lo
	}
	if v > r.hi {
		return r.hi
	}
	return v
}

// markdown satisfies Renderable structurally.
type markdown string

func (m markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }

// TestCapabilityProbing confirms discovery is by structural probe, not a type
// switch: any value with the method shape is discovered, with no registration.
func TestBoundedProbe(t *testing.T) {
	if b, ok := AsBounded(unit(0.5)); !ok {
		t.Error("unit should be Bounded")
	} else if lo, hi := b.Bounds(); lo != 0 || hi != 1 {
		t.Errorf("bounds = %v,%v want 0,1", lo, hi)
	}
	if _, ok := AsBounded(42); ok {
		t.Error("a plain int should not be Bounded")
	}
}

func TestReconcilerClamps(t *testing.T) {
	r, ok := AsReconciler(rng{lo: 0, hi: 150})
	if !ok {
		t.Fatal("rng should be a Reconciler")
	}
	if got := r.Reconcile(200.0); got != 150.0 {
		t.Errorf("clamp 200 into [0,150] = %v, want 150", got)
	}
	if got := r.Reconcile(75.0); got != 75.0 {
		t.Errorf("75 within [0,150] should be unchanged, got %v", got)
	}
}

func TestRenderableProbe(t *testing.T) {
	// AsRendered converts text/markdown to safe text/html at this chokepoint (see
	// renderMarkdown), so a markdown source comes back as rendered HTML, not source.
	if r, ok := AsRendered(markdown("# hi")); !ok {
		t.Error("markdown should be Renderable")
	} else if r.MIME != "text/html" || r.Data != "<h1>hi</h1>\n" {
		t.Errorf("rendered = %+v", r)
	}
	if _, ok := AsRendered(42); ok {
		t.Error("a plain int should not be Renderable")
	}
}

// ownMulti defines its OWN WidgetView-shaped type and a WidgetView() method,
// exactly as a notebook does — importing nothing from this package. It proves
// AsWidgetView matches structurally across the zero-import boundary, like
// AsRendered does for Rendered.
type ownWidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}
type ownMulti struct{ sel, all []string }

func (m ownMulti) WidgetView() ownWidgetView {
	max := 5
	return ownWidgetView{Value: m.sel, Options: m.all, Max: &max}
}

// TestWidgetViewProbe confirms a widget states its state across the boundary:
// AsWidgetView reads a notebook's own WidgetView-shaped return by field name,
// carries the selection + options, and rejects a non-widget value. State only —
// there is no field for appearance to leak through.
func TestWidgetViewProbe(t *testing.T) {
	wv, ok := AsWidgetView(ownMulti{sel: []string{"a"}, all: []string{"a", "b", "c"}})
	if !ok {
		t.Fatal("ownMulti should be Viewable across the zero-import boundary")
	}
	got, _ := wv.Value.([]string)
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("view Value = %v, want [a]", wv.Value)
	}
	if len(wv.Options) != 3 || wv.Max == nil || *wv.Max != 5 {
		t.Errorf("view = %+v, want 3 options + Max 5", wv)
	}
	if _, ok := AsWidgetView(42); ok {
		t.Error("a plain int is not a widget view")
	}
	if _, ok := AsWidgetView(markdown("# hi")); ok {
		t.Error("a Renderable is not a widget view (different capability)")
	}
}

// TestDoneEventEmitsWidget confirms the wire path: a cell producing a widget
// value (Viewable, not Renderable, not scalar) reaches the client as an Out
// with the widget MIME carrying the JSON state — where before it was nil (the
// probe finding this round fixed). §8: assert the bytes on the seam.
func TestDoneEventEmitsWidget(t *testing.T) {
	rt := NewRuntime(Config{}, NewHead(), NewMemoStore())
	ev := rt.doneEvent(1, levelResult{
		id:  "picker",
		out: Outputs{"themes": ownMulti{sel: []string{"City"}, all: []string{"City", "Duplo"}}},
	})
	if ev.Out == nil {
		t.Fatal("a widget cell must reach the client, not be dropped to nil")
	}
	if ev.Out.MIME != WidgetMIME {
		t.Errorf("MIME = %q, want %q", ev.Out.MIME, WidgetMIME)
	}
	if !strings.Contains(ev.Out.Data, `"City"`) || !strings.Contains(ev.Out.Data, `"options"`) {
		t.Errorf("widget JSON missing selection/options: %s", ev.Out.Data)
	}
}

// TestScalarReadout pins the unit of the display degradation ladder: scalars
// (basic kinds and named types over them) format to a one-line readout; a
// composite does not. This is the pure function under doneEvent's fallback.
func TestScalarReadout(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
		ok   bool
	}{
		{"float", 0.75, "0.75", true},
		{"named-float", usd(1.006), "1.006", true},
		{"int", 80, "80", true},
		{"bool", true, "true", true},
		{"string", "hi", "hi", true},
		{"slice-composite", []int{1, 2}, "", false},
		{"struct-composite", struct{ X int }{1}, "", false},
		{"nil", nil, "", false},
	}
	for _, c := range cases {
		got, ok := scalarReadout(c.v)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: scalarReadout(%v) = %q,%v want %q,%v", c.name, c.v, got, ok, c.want, c.ok)
		}
	}
}

// TestDisplayLadder observes the effect the ladder names (spec §8): a scalar
// cell's committed event carries a text/plain readout of its value, a Renderable
// cell carries its rich MIME, and a composite-valued cell carries no output at
// all (so the transport hides it). Asserts the emitted Event.Out, not an
// internal — the display seam is the event.
func TestDisplayLadder(t *testing.T) {
	// scalar: a named-float over a leaf, so it recomputes and commits a readout.
	util := fnNode{
		id: "utilization", in: []Symbol{"c"}, out: []Symbol{"rho"},
		run: func(_ context.Context, in Inputs) (Outputs, error) {
			return Outputs{"rho": usd(float64(in["c"].(int)) / 100.0)}, nil
		},
	}
	// renderable: markdown has Render() → rich view.
	note := fnNode{
		id: "note", in: []Symbol{"c"}, out: []Symbol{"md"},
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			return Outputs{"md": markdown("# hi")}, nil
		},
	}
	// composite: a raw slice, no Render, not scalar → no output, stays hidden.
	pts := fnNode{
		id: "pts", in: []Symbol{"c"}, out: []Symbol{"xs"},
		run: func(_ context.Context, _ Inputs) (Outputs, error) {
			return Outputs{"xs": []int{1, 2, 3}}, nil
		},
	}
	cfg := Config{
		Nodes:  []Node{util, note, pts},
		Leaves: []LeafID{"c"},
		Levels: [][]CellID{{"utilization", "note", "pts"}},
	}
	head := NewHead()
	head.Set("c", 75)
	rt := NewRuntime(cfg, head, NewMemoStore())

	drain := collectEvents(rt)
	rt.RunAll(context.Background())
	events := drain()

	ev, ok := lastEvent(events, "utilization")
	if !ok || ev.Out == nil {
		t.Fatalf("scalar cell must carry a readout; got %+v", ev)
	}
	if ev.Out.MIME != "text/plain" || ev.Out.Data != "0.75" {
		t.Errorf("scalar readout = %q/%q, want text/plain/0.75", ev.Out.MIME, ev.Out.Data)
	}

	// The markdown cell's rich view arrives as text/html (converted from its
	// text/markdown source at the AsRendered chokepoint).
	if ev, ok := lastEvent(events, "note"); !ok || ev.Out == nil || ev.Out.MIME != "text/html" {
		t.Errorf("renderable cell must carry its rich view; got %+v", ev)
	}

	if ev, ok := lastEvent(events, "pts"); !ok || ev.Out != nil {
		t.Errorf("composite cell must carry NO output (stays hidden); got %+v", ev)
	}
}
