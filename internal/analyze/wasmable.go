package analyze

import (
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// WASM-ability is NOT the same question as purity, and conflating them would be
// a bug. Purity asks "is this cell's output a function of its inputs alone" —
// it disqualifies time, rand, and I/O, because all of those break caching.
// WASM-ability asks "can this cell's code run client-side in a browser" — and
// there, time and randomness are FINE (js/wasm has working clocks and
// crypto/rand); only genuine host access is disqualifying: the network, the
// filesystem, and cgo. A notebook can be impure yet perfectly WASM-able.
//
// So this reuses purity's CHA-walk machinery (see reaches) with a different
// primitive set. It does not reuse RefinePurity's verdict — that was the
// finding: purity cannot answer this question, because math/rand and time.Now
// are impure but WASM-able.

// nonPortablePackages are packages whose use makes a cell unable to run in the
// browser. Conservative (over-rejection is the safe direction, as with purity):
// the network and the filesystem have no browser equivalent, and cgo can't
// cross-compile to wasm at all.
var nonPortablePackages = map[string]bool{
	"os":       true, // filesystem / process — no browser equivalent
	"net":      true, // sockets
	"net/http": true, // real HTTP client/server
	"os/exec":  true, // subprocesses
	"syscall":  true, // raw host syscalls (the js shim is a different package)
}

// WASMability marks every cell in g with whether it can run under GOOS=js
// GOARCH=wasm, from the call graph — not by hand, and not from the purity pass.
// A cell is WASM-able iff it transitively reaches none of the non-portable
// primitives. Results are written to graph.Cell.WASMable.
//
// Best-effort like RefinePurity: a cell whose SSA function can't be found keeps
// the safe default (false — not provably portable). Requires a package loaded
// with full dependency source (use [LoadForPurity]).
func WASMability(pkg *packages.Package, g *graph.Graph) {
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
			continue
		}
		cell.WASMable = !reaches(cg, fn, isNonPortable)
	}
}

// isNonPortable reports whether an SSA function belongs to a package that can't
// run client-side. (cgo is caught upstream: a cgo notebook won't cross-compile
// to wasm at all, which the build surfaces directly.)
func isNonPortable(fn *ssa.Function) bool {
	if fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return false
	}
	return nonPortablePackages[fn.Pkg.Pkg.Path()]
}

// NotebookWASMable reports whether the whole notebook can run in the browser:
// every runnable cell must be WASM-able. It returns the offending cells (not
// WASM-able) so the caller can name them. A notebook with a fold or other
// deferred feature is judged on its runnable cells only.
func NotebookWASMable(g *graph.Graph) (ok bool, blockers []graph.CellID) {
	for _, id := range g.Order {
		if !g.Cells[id].WASMable {
			blockers = append(blockers, id)
		}
	}
	return len(blockers) == 0, blockers
}
