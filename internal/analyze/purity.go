package analyze

import (
	"fmt"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// Purity is off the interactive path by design.
//
// Purity is consumed by exactly one thing: the cache. And a conservative
// verdict is always safe — marking a pure cell impure only costs a cache hit,
// while marking an impure cell pure would give wrong answers. So cells default
// to impure (see buildCell) and are refined here, at build time or in the
// background, never blocking an edit.
//
// This is also why purity, not graph derivation, is what needs NeedDeps: the
// call graph requires full dependency source. Keeping it on a separate pass is
// what lets the interactive load drop NeedDeps and run in ~85ms.

// impurePackages are standard-library packages whose use makes a cell impure: a
// cell that transitively touches randomness or I/O is not a pure function of
// its inputs and must never be cached.
//
// Cacheability is derived from the call graph, never declared — there is
// deliberately no //notebook:nocache directive.
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

// RefinePurity computes the Pure flag for every cell in g from the package call
// graph, mutating the cells in place. It requires a package loaded with
// NeedDeps (full dependency source) — use [LoadForPurity].
//
// It uses a CHA call graph rather than VTA: purity needs only a sound
// over-approximation of "does this reach time.Now / rand / os / net", and CHA
// provides that far more cheaply. CHA over-approximates interface dispatch, so
// it may occasionally mark a pure cell impure — which costs a cache hit and
// nothing else, the safe direction.
//
// RefinePurity is best-effort: if SSA cannot be built or a cell's function
// cannot be found, that cell keeps its (safe, impure) default and no error is
// returned.
func RefinePurity(pkg *packages.Package, g *graph.Graph) {
	prog, ssaPkgs := ssautil.AllPackages([]*packages.Package{pkg}, ssa.InstantiateGenerics)
	prog.Build()

	var nbPkg *ssa.Package
	for _, p := range ssaPkgs {
		if p != nil && p.Pkg == pkg.Types {
			nbPkg = p
			break
		}
	}
	if nbPkg == nil {
		return
	}

	cg := cha.CallGraph(prog)
	for id, cell := range g.Cells {
		fn := nbPkg.Func(string(id))
		if fn == nil {
			continue // keep the safe impure default
		}
		cell.Pure = !reachesImpure(cg, fn)
	}
}

// LoadForPurity loads the notebook package with full dependency source
// (LoadAllSyntax implies NeedDeps) so its call graph can be built. This is the
// heavy load; keep it off the interactive path.
func LoadForPurity(dir string) (*packages.Package, error) {
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("loading package for purity in %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go package found in %s", dir)
	}
	if len(pkgs[0].Errors) > 0 {
		return nil, fmt.Errorf("package %s has errors: %w", pkgs[0].Name, pkgs[0].Errors[0])
	}
	return pkgs[0], nil
}

// reachesImpure reports whether fn transitively calls any impure function.
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
				continue
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
