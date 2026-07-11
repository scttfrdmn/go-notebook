package graph

import (
	"strings"
	"testing"
)

func TestNoCycleInDAG(t *testing.T) {
	g := buildGraph(
		mkCell("a", 1, nil, [][2]string{{"x", "int"}}),
		mkCell("b", 2, [][2]string{{"x", "int"}}, [][2]string{{"y", "int"}}),
	)
	for _, d := range g.Check() {
		if strings.Contains(d.Msg, "cycle") {
			t.Errorf("unexpected cycle diagnostic on a DAG: %v", d)
		}
	}
}

func TestCycleDetectedAndRenderedAsPath(t *testing.T) {
	// a -> b -> c -> a: each cell consumes the previous one's output.
	g := buildGraph(
		mkCell("a", 1, [][2]string{{"z", "int"}}, [][2]string{{"x", "int"}}),
		mkCell("b", 2, [][2]string{{"x", "int"}}, [][2]string{{"y", "int"}}),
		mkCell("c", 3, [][2]string{{"y", "int"}}, [][2]string{{"z", "int"}}),
	)
	var cyc *Diagnostic
	for i := range g.Check() {
		d := g.Check()[i]
		if strings.Contains(d.Msg, "cycle") {
			cyc = &d
			break
		}
	}
	if cyc == nil {
		t.Fatal("expected a cycle diagnostic")
	}
	// The rendered path must name all three cells and close the loop.
	for _, name := range []string{"a", "b", "c"} {
		if !strings.Contains(cyc.Msg, name) {
			t.Errorf("cycle path %q missing cell %q", cyc.Msg, name)
		}
	}
	if strings.Count(cyc.Msg, "->") < 3 {
		t.Errorf("cycle should render as a closed path with >=3 arrows: %q", cyc.Msg)
	}
}

func TestDelayedEdgeIsNotACycle(t *testing.T) {
	// A self-edge through a Delayed param (a fold) must NOT be reported as a
	// cycle — this is the whole point of the Delayed kind shipping now.
	sim := mkCell("sim", 1,
		[][2]string{{"tick", "Tick"}, {"lambda", "PerHour"}},
		[][2]string{{"state", "Sim"}})
	// Add the delayed self-edge by hand: sim consumes its own `state`.
	sim.Params = append(sim.Params, Param{
		Name: "state", Type: "Sim", Kind: Delayed,
		Pos: Position{Filename: "n.go", Line: 1, Column: 5},
	})
	g := buildGraph(
		mkCell("arrivalRate", 2, nil, [][2]string{{"lambda", "PerHour"}}),
		mkCell("clock", 3, nil, [][2]string{{"tick", "Tick"}}),
		sim,
	)
	for _, d := range g.Check() {
		if strings.Contains(d.Msg, "cycle") {
			t.Errorf("delayed self-edge must not be a cycle, got: %v", d)
		}
	}
}
