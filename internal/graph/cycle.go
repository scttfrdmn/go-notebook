package graph

import (
	"fmt"
	"strings"
)

// deps returns the cells that a cell directly depends on: the producers of its
// wired (non-Injected, non-Delayed) parameters. Delayed self-edges are skipped
// by construction — wiredParams excludes them — which is precisely how folds
// avoid being reported as cycles.
//
// A producer that does not exist (a missing edge) is omitted here; that is
// reported separately by checkWiring, and cycle detection should not also trip
// over it.
func (g *Graph) deps(id CellID) []CellID {
	c := g.Cells[id]
	seen := make(map[CellID]bool)
	var out []CellID
	for _, p := range c.wiredParams() {
		producer, ok := g.Producer[p.Name]
		if !ok || producer == id || seen[producer] {
			continue
		}
		seen[producer] = true
		out = append(out, producer)
	}
	return out
}

// checkCycles reports the first cycle found among non-Delayed edges, rendered
// as a path (a -> b -> c -> a). One diagnostic is enough: a cycle makes the
// whole graph unschedulable and the user fixes them one at a time.
func (g *Graph) checkCycles() []Diagnostic {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make(map[CellID]int, len(g.Cells))

	var stack []CellID
	var cycle []CellID

	var visit func(id CellID) bool
	visit = func(id CellID) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range g.deps(id) {
			switch color[dep] {
			case gray:
				// Found a back edge: extract the cycle from the stack.
				cycle = extractCycle(stack, dep)
				return true
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	// Visit in source order for deterministic cycle reporting.
	for _, id := range g.Order {
		if color[id] == white {
			if visit(id) {
				break
			}
		}
	}

	if cycle == nil {
		return nil
	}

	// deps() points from a consumer to its producer, so the DFS path runs
	// downstream-to-upstream; reverse it to read as dataflow (upstream first).
	reversed := make([]CellID, len(cycle))
	for i, id := range cycle {
		reversed[len(cycle)-1-i] = id
	}

	pos := g.Cells[reversed[0]].Pos
	return []Diagnostic{{
		Pos:  pos,
		Msg:  fmt.Sprintf("cycle among cells: %s.", renderCycle(reversed)),
		Hint: "cells cannot depend on each other in a loop; a stateful feedback loop needs Prev[T] (not supported yet)",
	}}
}

// extractCycle returns the slice of the DFS stack from the first occurrence of
// `start` to the top — the cells forming the cycle.
func extractCycle(stack []CellID, start CellID) []CellID {
	for i, id := range stack {
		if id == start {
			cyc := make([]CellID, len(stack)-i)
			copy(cyc, stack[i:])
			return cyc
		}
	}
	return nil
}

// renderCycle formats a cycle path, closing the loop back to the first cell:
// "a -> b -> c -> a".
func renderCycle(cycle []CellID) string {
	if len(cycle) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cycle)+1)
	for _, id := range cycle {
		parts = append(parts, string(id))
	}
	parts = append(parts, string(cycle[0])) // close the loop
	return strings.Join(parts, " -> ")
}
