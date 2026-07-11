package analyze

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// notebookDirective is the file-level marker that declares a Go file a
// notebook. It names what the file is, not what reads it — the same register as
// //go:generate — so a notebook file carries no mention of this project.
const notebookDirective = "//go:notebook"

// TypesAnalyzer derives a notebook graph using go/packages and go/types. It is
// the only Analyzer implementation in this milestone.
//
// The zero value is ready to use. Analyze performs a cold, one-shot derivation.
// For the interactive re-analysis path (a one-cell edit), use a [Session],
// which primes the importer once and then re-typechecks only the notebook
// package — orders of magnitude faster than reloading.
type TypesAnalyzer struct{}

var _ Analyzer = TypesAnalyzer{}

// graphLoadMode is the package facts needed to derive the graph. Crucially it
// omits NeedDeps: dependency types are resolved from export data via
// NeedImports, which is cheap. NeedDeps (full dependency source) is an order of
// magnitude more expensive and is required only for purity's call graph — a
// separate, off-the-hot-path concern (see purity.go).
const graphLoadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// Analyze loads the package in dir (graph mode, no NeedDeps), finds the
// notebook file, and derives the graph. Purity is NOT computed here: cells
// default to impure, which is the always-safe default (a conservative impure
// verdict only costs a cache miss). Refine purity separately with
// [RefinePurity] when the cache needs it; never on an interactive edit.
//
// Load failures return an error; notebook-level problems are returned as
// diagnostics on the fullest graph the analyzer could build.
func (TypesAnalyzer) Analyze(dir string) (*graph.Graph, []graph.Diagnostic, error) {
	cfg := &packages.Config{Mode: graphLoadMode, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("loading package in %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no Go package found in %s", dir)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, nil, fmt.Errorf("package %s has errors: %w", pkg.Name, pkg.Errors[0])
	}
	return buildFromTypes(pkg.Fset, pkg.Syntax, pkg.Types)
}

// buildFromTypes derives the graph from an already-typechecked package. It is
// the shared core of both the cold [TypesAnalyzer.Analyze] path and the
// incremental [Session] path, so the two cannot diverge.
//
// Cells are looked up by name in the package scope (rather than via a
// TypesInfo.Defs map), which works identically whether the package came from
// go/packages or from an incremental re-typecheck.
func buildFromTypes(fset *token.FileSet, files []*ast.File, tpkg *types.Package) (*graph.Graph, []graph.Diagnostic, error) {
	file, err := findNotebookFile(files)
	if err != nil {
		return nil, nil, err
	}

	g := graph.New()
	var diags []graph.Diagnostic
	q := types.RelativeTo(tpkg)
	scope := tpkg.Scope()

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv != nil {
			continue // a method is neither a cell nor a listed helper
		}
		obj := scope.Lookup(fn.Name.Name)
		fnObj, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		cell, cellDiags, isCell := buildCell(fset, fn, fnObj, q)
		if !isCell {
			// A top-level func that produces no named result is a helper.
			// Record it (unless generic, which is a distinct exclusion) so
			// tooling can explain why it isn't a cell.
			if sig, ok := fnObj.Type().(*types.Signature); ok && sig.TypeParams().Len() == 0 {
				g.Helpers = append(g.Helpers, graph.CellID(fn.Name.Name))
			}
			continue
		}
		g.Add(cell)
		diags = append(diags, cellDiags...)
	}

	diags = append(diags, g.Check()...)
	return g, diags, nil
}

// findNotebookFile returns the single file carrying the //go:notebook
// directive. It errors if there is none or more than one.
func findNotebookFile(files []*ast.File) (*ast.File, error) {
	var found *ast.File
	for _, f := range files {
		if hasNotebookDirective(f) {
			if found != nil {
				return nil, fmt.Errorf("package has more than one %s file", notebookDirective)
			}
			found = f
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no %s file found", notebookDirective)
	}
	return found, nil
}

// hasNotebookDirective reports whether the file carries the //go:notebook
// marker as one of its comment lines.
func hasNotebookDirective(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if c.Text == notebookDirective {
				return true
			}
		}
	}
	return false
}

// buildCell turns a function declaration into a graph cell, if it is one.
//
// The cell-detection rule is the wiring rule doing double duty: a cell is a
// top-level func that produces at least one named, non-error result. The named
// result IS the edge, so a function that names no result produces no edge and
// therefore cannot be a cell — it is an ordinary helper, regardless of whether
// it has a doc comment. The doc comment is the label, not the marker.
//
// Two exclusions:
//
//   - Methods are never cells (they are not in package scope by bare name; the
//     caller's scope lookup already filters them, but we guard anyway).
//   - Generic functions are never cells: a func with type parameters has no
//     concrete result type to wire.
//
// The "unnamed result" diagnostic fires only for the genuinely ambiguous case:
// a cell that names some results but leaves a non-error result unnamed.
func buildCell(fset *token.FileSet, fn *ast.FuncDecl, fnObj *types.Func, q types.Qualifier) (*graph.Cell, []graph.Diagnostic, bool) {
	if fn.Recv != nil {
		return nil, nil, false // methods are never cells
	}
	sig, ok := fnObj.Type().(*types.Signature)
	if !ok {
		return nil, nil, false
	}
	if sig.TypeParams().Len() > 0 {
		return nil, nil, false // generic funcs have no concrete result type to wire
	}

	results := sig.Results()
	named, hasUnnamedData := classifyResults(results)
	if named == 0 {
		return nil, nil, false // produces no named result → a helper, not a cell
	}

	cell := &graph.Cell{
		ID:         graph.CellID(fn.Name.Name),
		Pos:        position(fset, fn.Name.Pos()),
		Doc:        docText(fn),
		Label:      label(fn),
		Directives: directives(fn),
		Params:     buildParams(fset, sig.Params(), q),
		Results:    buildResults(fset, results, q),
		Pure:       false, // safe default; refined by RefinePurity, never on the hot path
		IsLeaf:     isLeafCell(sig),
	}

	var diags []graph.Diagnostic
	if hasUnnamedData {
		diags = append(diags, graph.Diagnostic{
			Pos:  cell.Pos,
			Msg:  fmt.Sprintf("cell %q has an unnamed result; cell results must be named — the name is the edge.", cell.ID),
			Hint: "name every result, or leave all results unnamed to make this an ordinary helper",
		})
	}
	// A cell whose output has a Render() method must return the shape the
	// engine's reflection probe reads; a typo would silently not render.
	diags = append(diags, checkRenderShape(fset, cell.ID, cell.Pos, results)...)
	// Prev[T] folds are deferred this milestone. The Delayed edge kind and the
	// analyzer branch ship now (the seam), but a notebook that actually uses a
	// fold is reported as unsupported rather than silently mis-scheduled.
	for _, p := range cell.Params {
		if p.Kind == graph.Delayed {
			diags = append(diags, graph.Diagnostic{
				Pos:      p.Pos,
				Severity: graph.Notice,
				Msg:      fmt.Sprintf("cell %q takes `%s %s`; Prev[T] folds are not supported in this milestone.", cell.ID, p.Name, p.Type),
				Hint:     "stateful cells (a Tick-clocked fold) are a later milestone; this cell will be skipped",
			})
		}
	}
	return cell, diags, true
}

// docText returns the cell's full doc comment text, or "" if it has none.
func docText(fn *ast.FuncDecl) string {
	if fn.Doc == nil {
		return ""
	}
	return fn.Doc.Text()
}

// classifyResults counts named data results and reports whether any non-error
// result is unnamed. A blank identifier (_) counts as unnamed: it names no
// symbol and so cannot be an edge.
func classifyResults(results *types.Tuple) (named int, hasUnnamedData bool) {
	for i := 0; i < results.Len(); i++ {
		v := results.At(i)
		if isErrorType(v.Type()) {
			continue // a trailing error is the failure channel, not an edge
		}
		if isBlank(v.Name()) {
			hasUnnamedData = true
		} else {
			named++
		}
	}
	return named, hasUnnamedData
}

// isBlank reports whether a result/parameter name is absent or the blank
// identifier — either way it names no dataflow symbol.
func isBlank(name string) bool { return name == "" || name == "_" }

// buildParams renders each parameter into the IR, classifying its kind.
func buildParams(fset *token.FileSet, params *types.Tuple, q types.Qualifier) []graph.Param {
	var out []graph.Param
	for i := 0; i < params.Len(); i++ {
		v := params.At(i)
		out = append(out, graph.Param{
			Name: graph.Symbol(v.Name()),
			Type: types.TypeString(v.Type(), q),
			Kind: paramKind(v.Type()),
			Pos:  position(fset, v.Pos()),
		})
	}
	return out
}

// buildResults renders each result into the IR, marking a trailing error and
// normalizing a blank result name to the empty string (it names no symbol).
func buildResults(fset *token.FileSet, results *types.Tuple, q types.Qualifier) []graph.Result {
	var out []graph.Result
	for i := 0; i < results.Len(); i++ {
		v := results.At(i)
		name := v.Name()
		if isBlank(name) {
			name = ""
		}
		out = append(out, graph.Result{
			Name:       graph.Symbol(name),
			Type:       types.TypeString(v.Type(), q),
			Underlying: basicKind(v.Type()),
			IsError:    isErrorType(v.Type()),
			Pos:        position(fset, v.Pos()),
		})
	}
	return out
}

// paramKind classifies a parameter by its type:
//
//   - context.Context → Injected (supplied by the runtime, not an edge)
//   - Prev[T]         → Delayed  (a previous-epoch self-edge; folds are
//     deferred, so the caller also emits an "unsupported" diagnostic)
//   - anything else   → Wired    (an ordinary edge)
func paramKind(t types.Type) graph.ParamKind {
	switch {
	case isContextType(t):
		return graph.Injected
	case isPrevType(t):
		return graph.Delayed
	default:
		return graph.Wired
	}
}

// isErrorType reports whether t is the builtin error interface.
func isErrorType(t types.Type) bool { return types.Identical(t, errorType) }

var errorType = types.Universe.Lookup("error").Type()

// isContextType reports whether t is context.Context.
func isContextType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == "Context" &&
		obj.Pkg() != nil && obj.Pkg().Path() == "context"
}

// isPrevType reports whether t is the notebook's Prev[T] fold marker. Prev is
// defined in the notebook package itself, so matching the type name is
// sufficient and needs no import.
func isPrevType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == "Prev"
}

// position converts a token.Pos to the IR's plain-data Position.
func position(fset *token.FileSet, pos token.Pos) graph.Position {
	p := fset.Position(pos)
	return graph.Position{Filename: p.Filename, Line: p.Line, Column: p.Column}
}
