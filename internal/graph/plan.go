package graph

import "sort"

// consumers returns the cells that directly depend on a producer cell: those
// with a wired parameter whose producer is `id`. It is the reverse of deps and
// is used to walk the dirty set downstream.
func (g *Graph) consumers(id CellID) []CellID {
	var out []CellID
	seen := make(map[CellID]bool)
	for _, other := range g.Order {
		if other == id {
			continue
		}
		for _, p := range g.Cells[other].wiredParams() {
			if g.Producer[p.Name] == id && !seen[other] {
				seen[other] = true
				out = append(out, other)
			}
		}
	}
	return out
}

// Dirty returns the transitive downstream closure of the cells that produce the
// changed symbols: every cell that must be recomputed when those symbols move.
// The producers of the changed symbols are themselves included.
//
// Delayed edges are not followed — a fold reads the previous epoch, so changing
// an input does not dirty it through the delayed edge (it advances only on a
// Tick). Since wiredParams excludes Delayed, consumers() already respects this.
func (g *Graph) Dirty(changed []Symbol) map[CellID]bool {
	dirty := make(map[CellID]bool)
	var stack []CellID

	for _, sym := range changed {
		if producer, ok := g.Producer[sym]; ok && !dirty[producer] {
			dirty[producer] = true
			stack = append(stack, producer)
		}
	}

	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range g.consumers(id) {
			if !dirty[c] {
				dirty[c] = true
				stack = append(stack, c)
			}
		}
	}
	return dirty
}

// Levels partitions the dirty cells into topological levels. Cells within a
// level have no dependencies on one another and so MUST be run concurrently;
// level i+1 depends only on levels <= i.
//
// Returning levels rather than a flat order is deliberate: it makes the
// parallelism structural — the scheduler fans a level out onto goroutines — so
// it cannot be quietly forgotten as an optimization.
//
// Only edges among dirty cells are considered; an edge from a clean upstream
// cell imposes no ordering because that value is already available. Delayed
// edges are ignored (they cross epochs). Cells within a level are sorted in
// source order for deterministic output.
func (g *Graph) Levels(dirty map[CellID]bool) [][]CellID {
	// Count in-degree from dirty producers only.
	indeg := make(map[CellID]int, len(dirty))
	for id := range dirty {
		indeg[id] = 0
	}
	for id := range dirty {
		for _, dep := range g.deps(id) {
			if dirty[dep] {
				indeg[id]++
			}
		}
	}

	var levels [][]CellID
	remaining := len(dirty)

	for remaining > 0 {
		// Every dirty cell with no remaining dirty dependency forms this level.
		var level []CellID
		for id := range dirty {
			if _, done := indeg[id]; done && indeg[id] == 0 {
				level = append(level, id)
			}
		}
		if len(level) == 0 {
			// A cycle among dirty cells; Check reports it separately. Stop
			// rather than loop forever.
			break
		}
		g.sortSourceOrder(level)
		levels = append(levels, level)

		// Remove this level and decrement its consumers' in-degrees.
		for _, id := range level {
			delete(indeg, id)
			remaining--
			for _, c := range g.consumers(id) {
				if _, ok := indeg[c]; ok {
					indeg[c]--
				}
			}
		}
	}
	return levels
}

// sortSourceOrder sorts a slice of cell IDs by their index in g.Order.
func (g *Graph) sortSourceOrder(ids []CellID) {
	rank := make(map[CellID]int, len(g.Order))
	for i, id := range g.Order {
		rank[id] = i
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return rank[ids[i]] < rank[ids[j]]
	})
}
