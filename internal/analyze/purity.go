package analyze

import (
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// impurePackages are standard-library packages whose use makes a cell impure:
// a cell that transitively touches time, randomness, or I/O is not a pure
// function of its inputs and must never be cached.
//
// This is the design's correction made concrete: cacheability is derived from
// the call graph, never declared. There is deliberately no //notebook:nocache
// directive — a comment that changes whether the answer is correct would
// violate the type-vs-comment rule.
var impurePackages = map[string]bool{
	"os":           true,
	"net":          true,
	"net/http":     true,
	"math/rand":    true,
	"math/rand/v2": true,
	"crypto/rand":  true,
}

// impureFuncs are specific functions (in otherwise-pure packages) that
// introduce ambient state or nondeterminism.
var impureFuncs = map[string]bool{
	"time.Now":   true,
	"time.Since": true,
	"time.Sleep": true,
	"time.Tick":  true,
	"time.After": true,
}

// annotatePurity derives the Pure flag for every cell in g from the package
// call graph, mutating the cells in place.
//
// It builds SSA for the whole program and a VTA call graph. VTA rather than CHA
// is deliberate: CHA over-approximates interface dispatch, so a cell that
// merely calls fmt.Fprintf into an in-memory strings.Builder gets linked to
// *os.File.Write and is falsely marked impure — which would make nearly every
// render cell uncacheable. VTA prunes those spurious edges using type-flow, and
// correctly reports such cells pure.
//
// Purity is best-effort: if SSA cannot be built, cells are left pure=false
// (the conservative default), and analysis does not fail — an inability to
// prove purity is not a notebook error.
func annotatePurity(pkg *packages.Package, g *graph.Graph) {
	prog, ssaPkgs := ssautil.AllPackages([]*packages.Package{pkg}, ssa.InstantiateGenerics)
	prog.Build()

	// Locate the SSA package matching the notebook's types package.
	var nbPkg *ssa.Package
	for _, p := range ssaPkgs {
		if p != nil && p.Pkg == pkg.Types {
			nbPkg = p
			break
		}
	}
	if nbPkg == nil {
		return // cannot map cells to SSA functions; leave pure=false
	}

	cg := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))

	for id, cell := range g.Cells {
		fn := nbPkg.Func(string(id))
		if fn == nil {
			continue
		}
		cell.Pure = !reachesImpure(cg, fn)
	}
}

// reachesImpure reports whether fn transitively calls any impure function.
//
// Package init functions are skipped: a package's init pulling in os (as fmt's
// does) says nothing about whether a cell's own logic is pure.
func reachesImpure(cg *callgraph.Graph, fn *ssa.Function) bool {
	root := cg.Nodes[fn]
	if root == nil {
		return false
	}
	seen := make(map[*callgraph.Node]bool)
	stack := []*callgraph.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true

		if callee := n.Func; callee != nil {
			if callee.Name() == "init" {
				continue // package initialization is not part of a cell's purity
			}
			if isImpureFunc(callee) {
				return true
			}
		}
		for _, e := range n.Out {
			stack = append(stack, e.Callee)
		}
	}
	return false
}

// isImpureFunc reports whether an SSA function is one of the impure primitives,
// either by belonging to an impure package or by being a named impure function.
func isImpureFunc(fn *ssa.Function) bool {
	if fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return false
	}
	path := fn.Pkg.Pkg.Path()
	if impurePackages[path] {
		return true
	}
	return impureFuncs[path+"."+fn.Name()]
}
