package graph

import (
	"reflect"
	"testing"
)

// diamondGraph: a -> {b, c} -> d. b and c are independent siblings.
func diamondGraph() *Graph {
	return buildGraph(
		mkCell("a", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("b", 2, [][2]string{{"x", "int"}}, [][2]string{{"y", "int"}}),
		mkCell("c", 3, [][2]string{{"x", "int"}}, [][2]string{{"z", "int"}}),
		mkCell("d", 4, [][2]string{{"y", "int"}, {"z", "int"}}, [][2]string{{"w", "int"}}),
	)
}

func cellSet(ids ...CellID) map[CellID]bool {
	m := make(map[CellID]bool)
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestDirtyTransitiveClosure(t *testing.T) {
	g := diamondGraph()
	// Changing x (produced by a) dirties everything downstream.
	got := g.Dirty([]Symbol{"x"})
	want := cellSet("a", "b", "c", "d")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Dirty(x) = %v, want %v", got, want)
	}
}

func TestDirtyPartial(t *testing.T) {
	g := diamondGraph()
	// Changing y (produced by b) dirties b and d, but not a or c.
	got := g.Dirty([]Symbol{"y"})
	want := cellSet("b", "d")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Dirty(y) = %v, want %v", got, want)
	}
}

func TestLevelsGroupsIndependentSiblings(t *testing.T) {
	g := diamondGraph()
	dirty := g.Dirty([]Symbol{"x"})
	levels := g.Levels(dirty)

	want := [][]CellID{
		{"a"},
		{"b", "c"}, // independent siblings, same level, source order
		{"d"},
	}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("Levels = %v, want %v", levels, want)
	}
}

func TestLevelsRespectsCleanUpstream(t *testing.T) {
	// If only y and z change (b and c recompute), d depends on both but a is
	// clean. d must still land in a later level than b and c.
	g := diamondGraph()
	dirty := g.Dirty([]Symbol{"y", "z"})
	levels := g.Levels(dirty)
	want := [][]CellID{
		{"b", "c"},
		{"d"},
	}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("Levels = %v, want %v", levels, want)
	}
}

func TestLevelsChain(t *testing.T) {
	// A linear chain a -> b -> c yields three singleton levels.
	g := buildGraph(
		mkCell("a", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("b", 2, [][2]string{{"x", "int"}}, [][2]string{{"y", "int"}}),
		mkCell("c", 3, [][2]string{{"y", "int"}}, [][2]string{{"z", "int"}}),
	)
	levels := g.Levels(g.Dirty([]Symbol{"x"}))
	want := [][]CellID{{"a"}, {"b"}, {"c"}}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("Levels = %v, want %v", levels, want)
	}
}
