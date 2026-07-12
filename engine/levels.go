package engine

import "sort"

// computeLevels derives topological levels from the nodes' declared inputs and
// outputs. Cells within a level are mutually independent and run concurrently;
// level i+1 depends only on levels <= i.
//
// The engine computes this at startup rather than having codegen emit it, which
// keeps the generated registry dumb (no topology knowledge) — the graph shape
// is fully recoverable from each Node's In()/Out(). Delayed (Prev[T]) edges do
// not exist as inputs here (folds are deferred and their cells are omitted from
// the registry), so ordinary Kahn layering is correct.
//
// If a cycle somehow reaches this point (it cannot for a graph that passed
// graph.Check), the cyclic remainder is dropped from the levels; the analyzer
// is the authority that prevents cycles, not the runtime.
func computeLevels(nodes []Node, producer map[Symbol]CellID) [][]CellID {
	deps := cellDeps(nodes, producer)

	indeg := make(map[CellID]int, len(nodes))
	order := make([]CellID, 0, len(nodes)) // stable id order for determinism
	for _, n := range nodes {
		indeg[n.ID()] = len(deps[n.ID()])
		order = append(order, n.ID())
	}
	rank := make(map[CellID]int, len(order))
	for i, id := range order {
		rank[id] = i
	}

	var levels [][]CellID
	remaining := len(nodes)
	done := make(map[CellID]bool, len(nodes))

	for remaining > 0 {
		var level []CellID
		for _, id := range order {
			if !done[id] && indeg[id] == 0 {
				level = append(level, id)
			}
		}
		if len(level) == 0 {
			break // cycle remainder; analyzer prevents this in practice
		}
		sort.Slice(level, func(i, j int) bool { return rank[level[i]] < rank[level[j]] })
		levels = append(levels, level)
		for _, id := range level {
			done[id] = true
			remaining--
		}
		decrementDependents(order, level, deps, indeg, done)
	}
	return levels
}

// cellDeps maps each cell to the set of producing cells it depends on (its own
// outputs and non-produced inputs — bare leaves — don't count).
func cellDeps(nodes []Node, producer map[Symbol]CellID) map[CellID]map[CellID]bool {
	deps := make(map[CellID]map[CellID]bool, len(nodes))
	for _, n := range nodes {
		set := make(map[CellID]bool)
		for _, in := range n.In() {
			if p, ok := producer[in]; ok && p != n.ID() {
				set[p] = true
			}
		}
		deps[n.ID()] = set
	}
	return deps
}

// decrementDependents lowers the in-degree of every not-yet-done cell for each
// dependency it had on a cell in the just-completed level.
func decrementDependents(order, level []CellID, deps map[CellID]map[CellID]bool, indeg map[CellID]int, done map[CellID]bool) {
	for _, n := range order {
		if done[n] {
			continue
		}
		for _, id := range level {
			if deps[n][id] {
				indeg[n]--
			}
		}
	}
}
