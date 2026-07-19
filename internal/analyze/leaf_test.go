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

// TestCellMarkerIsNamedResult pins the rule that the *named result* — not the
// doc comment — is what makes a top-level function a cell. The docs once claimed
// "undocumented functions are not cells" (design.md), but the analyzer marks a
// cell by its named results and treats the doc comment as the label only. This
// test is the guard so that claim cannot silently re-drift from the code.
func TestCellMarkerIsNamedResult(t *testing.T) {
	src := `
//notebook:slider min=0 max=100
func base() (x int) { return 20 }

// This one HAS a doc comment and a named result → a cell (with a label).
func documented(x int) (y int) { return x * 2 }

func undocumented(x int) (z int) { return x + 1 }

// This one HAS a doc comment but UNNAMED returns → a helper, not a cell.
func documentedHelper(x int) int { return x - 1 }

func undocumentedHelper(x int) int { return x }
`
	dir := writeNotebook(t, src)
	g, _, err := TypesAnalyzer{}.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Named results → cells, regardless of documentation.
	for _, id := range []string{"base", "documented", "undocumented"} {
		if _, ok := g.Cells[graph.CellID(id)]; !ok {
			t.Errorf("cell %q not found — a named result must make a cell whether or not it is documented", id)
		}
	}
	// Unnamed returns → helpers, regardless of documentation.
	for _, id := range []string{"documentedHelper", "undocumentedHelper"} {
		if _, ok := g.Cells[graph.CellID(id)]; ok {
			t.Errorf("%q became a cell — an unnamed-return function is a helper even when documented", id)
		}
	}
	// The doc comment is the label: documented has one, undocumented does not.
	if got := g.Cells["documented"].Label; got == "" {
		t.Error("documented cell has no label; the doc comment should supply it")
	}
	if got := g.Cells["undocumented"].Label; got != "undocumented" {
		t.Errorf("undocumented cell label = %q, want the function name %q as the fallback", got, "undocumented")
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
