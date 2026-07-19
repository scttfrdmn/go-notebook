package chart

import (
	"strings"
	"testing"
)

type sale struct {
	Region  string
	Units   int
	Revenue float64
}

// TestTableEqualNoPanic guards the engine-integration bug: Table holds `data
// any`, which is statically comparable, so the engine's change-detection would
// reach for == and panic when the interface holds a slice. Table.Equal must take
// that path instead. This mirrors what engine.changed does.
func TestTableEqualNoPanic(t *testing.T) {
	rows := []sale{{"North", 1200, 54000}, {"South", 890, 40050}}
	a := Rows(rows)
	b := Rows(rows)

	// The engine calls Equal(any) when the value provides it; must not panic and
	// must report equal for identical data.
	if !a.Equal(b) {
		t.Error("Equal: identical tables reported unequal")
	}
	changed := Rows([]sale{{"North", 1200, 54000}})
	if a.Equal(changed) {
		t.Error("Equal: different tables reported equal")
	}
}

func TestTableRender(t *testing.T) {
	rows := []sale{{"North", 1200, 54000.5}, {"South", 890, 40050}}
	html := RowsWith(Opts{Title: "Sales"}, rows).Render().Data
	for _, want := range []string{
		"Sales",           // caption
		"Region", "Units", // headers
		"54,000.5",    // fractional revenue not rounded, grouped
		"1,200",       // grouped int
		`class="num"`, // numeric columns aligned
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Render output missing %q", want)
		}
	}
}
