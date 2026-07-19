package chart

import (
	"strings"
	"testing"
)

// These tests exercise every form's render path and assert structural facts
// about the SVG/HTML they emit — valid root element, expected labels and marks
// present. They don't judge pixels (that's the browser preview during
// development); they guard against a form silently emitting nothing or malformed
// markup, and they cover the categorical framing the point-based tests don't.

func TestLineRender(t *testing.T) {
	out := LineWith(Opts{Title: "T", XLabel: "x", YLabel: "y"},
		Series{Name: "a", XY: []Pt{{1, 2}, {2, 4}, {3, 3}}},
		Series{Name: "b", XY: []Pt{{1, 1}, {2, 2}, {3, 5}}},
	).Render()
	svg := out.Data
	for _, want := range []string{"<svg", "</svg>", "<path", ">T<", ">a<", ">b<", "var(--grid)"} {
		if !strings.Contains(svg, want) {
			t.Errorf("line svg missing %q", want)
		}
	}
	// A single series draws no legend/direct label but still a path.
	solo := Line(Series{XY: []Pt{{0, 0}, {1, 1}}}).Render().Data
	if !strings.Contains(solo, "<path") {
		t.Error("single-series line missing path")
	}
}

func TestScatterRender(t *testing.T) {
	svg := ScatterWith(Opts{Title: "S", Fit: true},
		Series{XY: []Pt{{1, 2.1}, {2, 3.9}, {3, 6.2}, {4, 7.8}}},
	).Render().Data
	for _, want := range []string{"<circle", "stroke-dasharray", "y = "} { // dots + fit line + equation
		if !strings.Contains(svg, want) {
			t.Errorf("scatter+fit svg missing %q", want)
		}
	}
	// Two named series → legend (dot key), no fit.
	two := Scatter(
		Series{Name: "A", XY: []Pt{{1, 1}}},
		Series{Name: "B", XY: []Pt{{2, 2}}},
	).Render().Data
	if !strings.Contains(two, ">A<") || !strings.Contains(two, ">B<") {
		t.Error("scatter legend missing series names")
	}
}

func TestBarRender(t *testing.T) {
	cats := []string{"Q1", "Q2", "Q3"}
	a := Series2{Name: "rev", Values: []float64{10, 20, 15}}
	b := Series2{Name: "cost", Values: []float64{6, 9, 8}}

	grouped := BarWith(Opts{Title: "G", YLabel: "$"}, cats, a, b).Render().Data
	for _, want := range []string{"<svg", ">Q1<", ">Q2<", ">rev<", ">cost<"} {
		if !strings.Contains(grouped, want) {
			t.Errorf("grouped bar missing %q", want)
		}
	}

	stacked := Bar(cats, a, b).Stacked().Render().Data
	if !strings.Contains(stacked, "<svg") {
		t.Error("stacked bar produced no svg")
	}

	horiz := BarWith(Opts{XLabel: "%"}, cats, a).Horizontal().Render().Data
	if !strings.Contains(horiz, ">Q1<") {
		t.Error("horizontal bar missing category label")
	}

	// Single series → no legend, one color.
	single := Bar(cats, a).Render().Data
	if strings.Contains(single, ">cost<") {
		t.Error("single-series bar should have no second-series legend")
	}
}

func TestHistogramRender(t *testing.T) {
	var xs []float64
	for i := 0; i < 100; i++ {
		xs = append(xs, float64(i%10)) // 10 evenly-populated bins
	}
	svg := HistWith(Opts{Title: "H", XLabel: "v", YLabel: "count"}, 10, xs).Render().Data
	for _, want := range []string{"<svg", ">H<", "var(--grid)"} {
		if !strings.Contains(svg, want) {
			t.Errorf("histogram missing %q", want)
		}
	}
	if !strings.Contains(svg, "<rect") && !strings.Contains(svg, "<path") {
		t.Error("histogram drew no bars")
	}
	// Auto-bin path (Bins=0) must not panic and must draw.
	auto := Hist(xs).Render().Data
	if !strings.Contains(auto, "<svg") {
		t.Error("auto-binned histogram produced no svg")
	}
	// Empty input degrades cleanly.
	empty := Hist(nil).Render().Data
	if !strings.Contains(empty, "<svg") {
		t.Error("empty histogram should still render a frame")
	}
}

func TestEmptyAndEdgeCases(t *testing.T) {
	// No series at all.
	if !strings.Contains(Line().Render().Data, "<svg") {
		t.Error("empty line should render a frame")
	}
	if !strings.Contains(Scatter().Render().Data, "<svg") {
		t.Error("empty scatter should render a frame")
	}
	if !strings.Contains(Bar(nil).Render().Data, "<svg") {
		t.Error("empty bar should render a frame")
	}
	// Degenerate value range (all equal) must not divide by zero.
	flat := Line(Series{XY: []Pt{{1, 5}, {2, 5}, {3, 5}}}).Render().Data
	if !strings.Contains(flat, "<path") {
		t.Error("flat line should still draw")
	}
	// YLog path.
	logc := LineWith(Opts{YLog: true}, Series{XY: []Pt{{1, 1}, {2, 100}, {3, 10000}}}).Render().Data
	if !strings.Contains(logc, "<svg") {
		t.Error("log-scale line produced no svg")
	}
}

func TestTableShapes(t *testing.T) {
	// Slice of maps.
	maps := []map[string]any{{"a": 1, "b": "x"}, {"a": 2, "b": "y"}}
	if !strings.Contains(Rows(maps).Render().Data, "<table") {
		t.Error("map table produced no table")
	}
	// [][]string.
	ss := [][]string{{"h1", "h2"}, {"1", "foo"}, {"2", "bar"}}
	tbl := Rows(ss).Render().Data
	if !strings.Contains(tbl, "h1") || !strings.Contains(tbl, "foo") {
		t.Error("[][]string table missing header/body")
	}
	// Empty slice.
	if !strings.Contains(Rows([]struct{ X int }{}).Render().Data, "<table") {
		t.Error("empty struct slice should still render an (empty) table")
	}
	// chart:"-" omits a field; chart:"Label" renames.
	type Row struct {
		Keep   int `chart:"Kept"`
		Hidden int `chart:"-"`
	}
	r := Rows([]Row{{1, 2}}).Render().Data
	if !strings.Contains(r, "Kept") {
		t.Error("chart tag rename not applied")
	}
	if strings.Contains(r, "Hidden") {
		t.Error("chart:\"-\" field not omitted")
	}
}
