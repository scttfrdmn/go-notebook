package graph

import (
	"fmt"
	"sort"
	"strings"
)

// Severity classifies how much a diagnostic blocks. An Error is a real defect
// in the notebook (a missing producer, a cycle, a type mismatch) and must stop
// a build. A Notice reports something the toolchain cannot do yet — a deferred
// feature such as a Prev[T] fold — which is not the author's mistake: the
// affected cell is skipped and the rest still builds.
type Severity int

const (
	// Error is a defect that blocks a build.
	Error Severity = iota
	// Notice is a non-blocking limitation (a deferred feature); the affected
	// cell is skipped but the notebook still builds.
	Notice
)

// String renders a Severity for diagnostic output.
func (s Severity) String() string {
	if s == Notice {
		return "notice"
	}
	return "error"
}

// Diagnostic is a single problem found in a notebook, carrying enough
// structure to render an actionable message.
//
// Diagnostic quality is a feature of this project, tested like one: the
// difference between a tool and a toy is whether "no cell produces `a Erlangs`"
// also tells you which cell you probably meant. The rendered form is:
//
//	capacity.go:31:19: cell "utilization" needs `a Erlangs`, but no cell produces it.
//	                   Did you mean `offeredLoad`, which produces `a Erlangs`? (capacity.go:26)
type Diagnostic struct {
	Pos      Position `json:"pos"`
	Severity Severity `json:"severity"`
	Msg      string   `json:"msg"`
	Hint     string   `json:"hint,omitempty"` // optional second line, aligned under Msg
	// HintPos, when set, is appended to the hint line as " (file:line:col)".
	HintPos *Position `json:"hintPos,omitempty"`
}

// String renders the diagnostic in the canonical compiler format, with any
// hint indented to align under the message text. An Error carries no severity
// prefix (matching the standard tool format); a Notice is marked "notice:" so a
// deferred-feature limitation reads distinctly from a real defect.
func (d Diagnostic) String() string {
	prefix := fmt.Sprintf("%s:%d:%d: ", d.Pos.Filename, d.Pos.Line, d.Pos.Column)
	if d.Severity == Notice {
		prefix += "notice: "
	}
	out := prefix + d.Msg
	if d.Hint != "" {
		hint := d.Hint
		if d.HintPos != nil {
			hint += fmt.Sprintf(" (%s:%d:%d)", d.HintPos.Filename, d.HintPos.Line, d.HintPos.Column)
		}
		out += "\n" + strings.Repeat(" ", len(prefix)) + hint
	}
	return out
}

// sortDiagnostics orders diagnostics deterministically by position then message
// so that golden output is stable.
func sortDiagnostics(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i].Pos, ds[j].Pos
		switch {
		case a.Filename != b.Filename:
			return a.Filename < b.Filename
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Column != b.Column:
			return a.Column < b.Column
		default:
			return ds[i].Msg < ds[j].Msg
		}
	})
}

// Index populates g.Producer from cell results and reports any symbol produced
// by more than one cell. The wiring rule keys on the result name, so two cells
// naming the same result are ambiguous regardless of type.
//
// Index is idempotent-ish: it rebuilds Producer from scratch each call.
func (g *Graph) Index() []Diagnostic {
	g.Producer = make(map[Symbol]CellID)
	var diags []Diagnostic

	for _, id := range g.Order {
		c := g.Cells[id]
		for _, r := range c.dataResults() {
			if prev, dup := g.Producer[r.Name]; dup {
				// Report on the later definition, pointing back at the first.
				firstPos := g.Cells[prev].resultPos(r.Name)
				diags = append(diags, Diagnostic{
					Pos:     r.Pos,
					Msg:     fmt.Sprintf("cell %q produces `%s %s`, but `%s` is already produced by cell %q.", c.ID, r.Name, r.Type, r.Name, prev),
					Hint:    "a result name is an edge, so it must be unique; first produced here",
					HintPos: &firstPos,
				})
				continue
			}
			g.Producer[r.Name] = c.ID
		}
	}
	sortDiagnostics(diags)
	return diags
}

// resultPos returns the position of the named result on this cell, or the
// cell's own position if not found.
func (c *Cell) resultPos(name Symbol) Position {
	for _, r := range c.Results {
		if r.Name == name {
			return r.Pos
		}
	}
	return c.Pos
}

// Check validates the whole graph and returns all diagnostics, ordered
// deterministically. An empty slice means the graph is well-formed.
//
// The checks, in order:
//
//  1. Single producer — two cells naming the same result (both positions).
//  2. Every Wired parameter has a producer (with a "did you mean" suggestion).
//  3. Type agreement — a Wired parameter's type equals its producer's type.
//  4. No cycles among non-Delayed edges — reported as a path.
//  5. A Delayed parameter requires a Tick parameter on the same cell.
//     (Deferred: folds are not implemented this milestone. The check is a
//     no-op with a single point to grow. See TODO(prev).)
func (g *Graph) Check() []Diagnostic {
	var diags []Diagnostic
	diags = append(diags, g.Index()...)
	diags = append(diags, g.checkWiring()...)
	diags = append(diags, g.checkCycles()...)
	diags = append(diags, g.checkDelayed()...)
	sortDiagnostics(diags)
	return diags
}

// checkWiring verifies that every wired parameter has a producer and that the
// producer's result type matches the parameter's type.
func (g *Graph) checkWiring() []Diagnostic {
	var diags []Diagnostic
	for _, id := range g.Order {
		c := g.Cells[id]
		for _, p := range c.wiredParams() {
			producerID, ok := g.Producer[p.Name]
			if !ok {
				diags = append(diags, g.missingProducer(c, p))
				continue
			}
			// Type agreement: producer must exist (checked above) and its
			// result type must equal the parameter's type.
			producer := g.Cells[producerID]
			rt := producer.resultType(p.Name)
			if rt != p.Type {
				rp := producer.resultPos(p.Name)
				diags = append(diags, Diagnostic{
					Pos:     p.Pos,
					Msg:     fmt.Sprintf("cell %q takes `%s %s`, but cell %q produces `%s` as `%s`.", c.ID, p.Name, p.Type, producerID, p.Name, rt),
					Hint:    "an edge matches on name and type; these disagree",
					HintPos: &rp,
				})
			}
		}
	}
	return diags
}

// missingProducer builds the "no cell produces X" diagnostic, including a
// suggestion of the closest cell that produces a value of the same type.
func (g *Graph) missingProducer(c *Cell, p Param) Diagnostic {
	d := Diagnostic{
		Pos: p.Pos,
		Msg: fmt.Sprintf("cell %q needs `%s %s`, but no cell produces it.", c.ID, p.Name, p.Type),
	}
	// Suggest a producer of the same type, if exactly the kind of near-miss
	// that makes the message useful.
	if sugg, pos, ok := g.suggestByType(p.Type); ok {
		d.Hint = fmt.Sprintf("Did you mean `%s`, which produces `%s`?", sugg, p.Type)
		d.HintPos = &pos
	}
	return d
}

// suggestByType finds the first cell (in source order) that produces a data
// result of the given type. It returns the producing cell's ID and the
// position of that result.
func (g *Graph) suggestByType(typ string) (CellID, Position, bool) {
	for _, id := range g.Order {
		c := g.Cells[id]
		for _, r := range c.dataResults() {
			if r.Type == typ {
				return c.ID, r.Pos, true
			}
		}
	}
	return "", Position{}, false
}

// resultType returns the type string of the named result, or "" if absent.
func (c *Cell) resultType(name Symbol) string {
	for _, r := range c.Results {
		if r.Name == name {
			return r.Type
		}
	}
	return ""
}

// checkDelayed enforces that a cell taking a Delayed (Prev[T]) parameter also
// takes a Tick — a register needs a clock.
//
// TODO(prev): folds are deferred this milestone; nothing produces a Delayed
// parameter yet (the analyzer emits an "unsupported" diagnostic upstream when
// it sees Prev[T]). When folds land, implement the Tick requirement here so it
// lives in exactly one place.
func (g *Graph) checkDelayed() []Diagnostic {
	return nil
}
