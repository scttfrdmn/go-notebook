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
