package analyze

import (
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// TestLeafRule pins the type-driven leaf rule against the corpus's cases — the
// property that a comment must NOT decide whether something is an editable
// input. Each case is a minimal notebook; we assert which cells are leaves.
func TestLeafRule(t *testing.T) {
	src := `
type Probability float64
func (Probability) Bounds() (float64, float64) { return 0, 1 }
type Range struct{ Lo, Hi, From, To float64 }
func (Range) Reconcile(saved any) any { return saved }
type Markdown string
func (m Markdown) Render() Rendered { return Rendered{} }
type Rendered struct{ MIME, Data string }
type Set struct{ Price float64 }

// slaTarget: parameterless + Bounds() → leaf (no directive!).
func slaTarget() (target Probability) { return 0.2 }
// rate: parameterless + named-float scalar → leaf (text field).
func rate() (r float64) { return 1 }
// enabled: parameterless + bool scalar → leaf (checkbox).
func enabled() (on bool) { return false }
// name: parameterless + string scalar → leaf (text field).
func name() (s string) { return "" }
// waitProb: HAS PARAMS + Bounds() but no Reconcile → NOT a leaf (computed).
func waitProb(target Probability) (p Probability) { return target }
// notes: Render() → NOT a leaf (a view, though its type is a string scalar).
func notes() (md Markdown) { return "" }
// rows: parameterless but a slice → NOT a leaf (computed root).
func rows() (all []Set) { return nil }
// priceRange: HAS PARAMS + Reconcile → leaf (data-derived selection widget).
func priceRange(all []Set) (prices Range) { return Range{} }
`
	dir := writeNotebook(t, src)
	g, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"slaTarget":  true,
		"rate":       true,
		"enabled":    true,
		"name":       true,
		"waitProb":   false,
		"notes":      false,
		"rows":       false,
		"priceRange": true,
	}
	for id, wantLeaf := range want {
		c, ok := g.Cells[graph.CellID(id)]
		if !ok {
			t.Errorf("cell %q not found", id)
			continue
		}
		if c.IsLeaf != wantLeaf {
			t.Errorf("cell %q: IsLeaf = %v, want %v", id, c.IsLeaf, wantLeaf)
		}
	}
}

// TestWidgetKind pins the static, type-derived control descriptor: each leaf's
// Kind is decided by capability (Options→multi/select, Bounds→range, bool→bool,
// value-slice→table/draggable), and a Table carries its row type's column schema
// (the property of T that the runtime value cannot supply). §8: assert the
// derived Kind + columns, the thing five downstream PRs dispatch on.
func TestWidgetKind(t *testing.T) {
	src := `
type Theme string
func (t Theme) Label() string { return string(t) }
type Axis string
func (a Axis) Label() string { return string(a) }
type Multi[T interface{ Label() string }] struct { All []T; Value []T; Max int }
func (m Multi[T]) Options() []string { return nil }
type Select[T interface{ Label() string }] struct { All []T; Value T }
func (s Select[T]) Options() []string { return nil }
type Range struct { Lo, Hi, From, To float64 }
func (Range) Bounds() (float64, float64) { return 0, 0 }
type Lot struct { Date string; Ticker string; Amount float64 }
type Table[T any] struct { Value []T }
type Set struct{ Theme Theme }

func themePicker(all []Set) (themes Multi[Theme]) { return Multi[Theme]{} }
func axis() (x Select[Axis]) { return Select[Axis]{} }
func years() (r Range) { return Range{} }
func enabled() (on bool) { return false }
func holdings() (lots Table[Lot]) { return Table[Lot]{} }
func rate() (n float64) { return 1 }
`
	dir := writeNotebook(t, src)
	g, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	kind := func(id string) string {
		c := g.Cells[graph.CellID(id)]
		if c == nil || c.Widget == nil {
			return ""
		}
		return c.Widget.Kind
	}
	for id, want := range map[string]string{
		"themePicker": "multi",
		"axis":        "select",
		"years":       "range",
		"enabled":     "bool",
		"holdings":    "table",
		"rate":        "", // a bare scalar — no widget meta, the default rung
	} {
		if got := kind(id); got != want {
			t.Errorf("%s: Kind = %q, want %q", id, got, want)
		}
	}

	// A Table carries its row type's columns — the schema a grid needs and the
	// runtime value can't supply. This is the answer to "does Kind survive
	// Table[T]": no, it carries T's schema, derived at codegen.
	tbl := g.Cells["holdings"].Widget
	if tbl == nil || len(tbl.Columns) != 3 {
		t.Fatalf("holdings should carry 3 columns, got %+v", tbl)
	}
	wantCols := map[string]string{"Date": "string", "Ticker": "string", "Amount": "number"}
	for _, col := range tbl.Columns {
		if wantCols[col.Name] != col.Type {
			t.Errorf("column %s: type %q, want %q", col.Name, col.Type, wantCols[col.Name])
		}
	}
}
