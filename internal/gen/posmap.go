package gen

import "github.com/scttfrdmn/go-notebook/internal/graph"

// PosMap maps a line in the generated registry back to the cell it came from.
// It is the seam for browser-IDE diagnostics: when a future hosted editor
// surfaces a build error against the synthesized file, this lets it be
// remapped to the cell the author actually wrote.
//
// It is unused this milestone and deliberately minimal. go build errors inside
// cell *bodies* already point at the user's real source (cells are ordinary
// functions in an ordinary package); only the machine-generated registry would
// ever need remapping, and it should never fail to compile. The map exists now
// so the capability is additive later, not a retrofit.
type PosMap struct {
	// Entries maps a 1-based line number in the generated file to a cell.
	Entries map[int]graph.CellID
}

// NewPosMap returns an empty position map.
func NewPosMap() *PosMap {
	return &PosMap{Entries: map[int]graph.CellID{}}
}

// Record notes that generated line points at cell id.
func (p *PosMap) Record(line int, id graph.CellID) {
	p.Entries[line] = id
}

// Lookup returns the cell a generated line maps to, if any.
func (p *PosMap) Lookup(line int) (graph.CellID, bool) {
	id, ok := p.Entries[line]
	return id, ok
}
