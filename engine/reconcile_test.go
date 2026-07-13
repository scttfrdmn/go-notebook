package engine

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestCoerceWire pins the write-path coercer: decoded JSON selections (the
// shapes a control POSTs) homogenize to the clean Go primitive a notebook's
// Reconcile asserts — []string / []float64 / string / bool — and a mixed array
// fails visibly (never a silent passthrough, the bug that made reconcile
// no-op). json.Number is preserved as a number. §8: assert the coerced shape.
func TestCoerceWire(t *testing.T) {
	num := func(s string) json.Number { return json.Number(s) }
	cases := []struct {
		name string
		in   any
		want any
		ok   bool
	}{
		{"multi labels", []any{"Duplo", "City"}, []string{"Duplo", "City"}, true},
		{"range endpoints", []any{num("20"), num("150")}, []float64{20, 150}, true},
		{"select label", "pieces", "pieces", true},
		{"bool", true, true, true},
		{"scalar number", num("1200"), 1200.0, true},
		{"empty selection", []any{}, []string{}, true},
		{"mixed array fails", []any{"a", num("1")}, nil, false},
	}
	for _, c := range cases {
		got, ok := CoerceWire(c.in)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
}

// The four reconcile behaviors, as local widget types defining their own
// Reconcile — the notebook's shape, no import. reconcileLeaf must dispatch to
// each (a Reconciler) and be the identity for a scalar (not a Reconciler).

// rangeW clamps a saved [from,to] into fresh [lo,hi].
type rangeW struct{ lo, hi, from, to float64 }

func (r rangeW) Reconcile(saved any) any {
	s, ok := saved.([]float64)
	if !ok || len(s) != 2 {
		return r
	}
	clamp := func(v float64) float64 {
		if v < r.lo {
			return r.lo
		}
		if v > r.hi {
			return r.hi
		}
		return v
	}
	r.from, r.to = clamp(s[0]), clamp(s[1])
	return r
}

// multiW filters saved labels to those still in options.
type multiW struct {
	options []string
	value   []string
}

func (m multiW) Reconcile(saved any) any {
	sel, ok := saved.([]string)
	if !ok {
		return m
	}
	valid := map[string]bool{}
	for _, o := range m.options {
		valid[o] = true
	}
	var kept []string
	for _, s := range sel {
		if valid[s] {
			kept = append(kept, s)
		}
	}
	m.value = kept
	return m
}

// selectW falls back to its default when the saved choice vanished.
type selectW struct {
	options []string
	value   string
}

func (s selectW) Reconcile(saved any) any {
	label, ok := saved.(string)
	if !ok {
		return s
	}
	for _, o := range s.options {
		if o == label {
			s.value = label
			return s
		}
	}
	return s // vanished → default stands
}

// dragW resets wholesale when the saved arity differs from the fresh schema's.
type dragW struct{ points []float64 }

func (d dragW) Reconcile(saved any) any {
	s, ok := saved.([]float64)
	if !ok || len(s) != len(d.points) {
		return d // arity changed (or unusable) → reset to the fresh schema
	}
	d.points = s
	return d
}

// TestReconcileTaxonomy drives KC14: each widget kind reconciles a saved
// selection against a freshly-computed schema in its OWN way, and a scalar is
// the identity. §8 — assert the reconciled value, the effect the taxonomy
// names, not merely that reconcile ran. This is the correction curvefit forced:
// a universal reconcile rule is wrong; four kinds, four behaviors.
func TestReconcileTaxonomy(t *testing.T) {
	rt := NewRuntime(Config{}, NewHead(), NewMemoStore())

	// Range CLAMPS: bounds shrank to [0,100] under a saved [20,150].
	got := rt.reconcileLeaf(rangeW{lo: 0, hi: 100}, []float64{20, 150}, true)
	if r := got.(rangeW); r.from != 20 || r.to != 100 {
		t.Errorf("range clamp: from/to = %v/%v, want 20/100", r.from, r.to)
	}

	// Multi FILTERS: "City" vanished from the data; keep the survivors.
	got = rt.reconcileLeaf(multiW{options: []string{"Duplo", "Star Wars"}},
		[]string{"Duplo", "City", "Star Wars"}, true)
	if m := got.(multiW); !reflect.DeepEqual(m.value, []string{"Duplo", "Star Wars"}) {
		t.Errorf("multi filter: value = %v, want [Duplo, Star Wars]", m.value)
	}

	// Select FALLS BACK: the saved choice is gone; the schema default stands.
	got = rt.reconcileLeaf(selectW{options: []string{"price", "pieces"}, value: "price"}, "year", true)
	if s := got.(selectW); s.value != "price" {
		t.Errorf("select fallback: value = %q, want the default \"price\"", s.value)
	}
	// Select KEEPS a still-valid choice.
	got = rt.reconcileLeaf(selectW{options: []string{"price", "pieces"}, value: "price"}, "pieces", true)
	if s := got.(selectW); s.value != "pieces" {
		t.Errorf("select keep: value = %q, want \"pieces\"", s.value)
	}

	// Draggable RESETS on arity change: schema has 3 points, saved has 2.
	got = rt.reconcileLeaf(dragW{points: []float64{1, 2, 3}}, []float64{9, 9}, true)
	if d := got.(dragW); !reflect.DeepEqual(d.points, []float64{1, 2, 3}) {
		t.Errorf("draggable reset: points = %v, want the fresh [1 2 3] (arity changed)", d.points)
	}
	// Draggable KEEPS edits when arity matches.
	got = rt.reconcileLeaf(dragW{points: []float64{1, 2, 3}}, []float64{9, 8, 7}, true)
	if d := got.(dragW); !reflect.DeepEqual(d.points, []float64{9, 8, 7}) {
		t.Errorf("draggable keep: points = %v, want the edited [9 8 7]", d.points)
	}

	// Scalar is the IDENTITY: no Reconcile method → the saved selection is the
	// value (a slider's float). One rule, boring case boring — not special-cased.
	got = rt.reconcileLeaf(1200.0, 3400.0, true)
	if got != 3400.0 {
		t.Errorf("scalar identity: got %v, want the saved 3400", got)
	}
	// No saved value → the schema (default) stands, for every kind.
	got = rt.reconcileLeaf(rangeW{lo: 0, hi: 100, from: 10, to: 90}, nil, false)
	if r := got.(rangeW); r.from != 10 || r.to != 90 {
		t.Errorf("no-selection: the default should stand, got from/to %v/%v", r.from, r.to)
	}
}
