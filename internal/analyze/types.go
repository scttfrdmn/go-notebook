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
// The zero value is ready to use.
type TypesAnalyzer struct{}

var _ Analyzer = TypesAnalyzer{}

// loadMode is the set of package facts the analyzer needs. Types and TypesInfo
// give cell signatures; Syntax gives doc comments and positions; Deps lets the
// type checker resolve imported types (context.Context, and any domain types).
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps

// Analyze loads the package in dir, finds the notebook file, and builds the
// graph. Load failures return an error; notebook-level problems (missing
// producers, cycles, unnamed results, unsupported folds) are returned as
// diagnostics on the fullest graph the analyzer could build.
func (TypesAnalyzer) Analyze(dir string) (*graph.Graph, []graph.Diagnostic, error) {
	cfg := &packages.Config{Mode: loadMode, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("loading package in %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no Go package found in %s", dir)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		// A package that does not typecheck cannot yield a reliable graph.
		return nil, nil, fmt.Errorf("package %s has errors: %w", pkg.Name, pkg.Errors[0])
	}

	file, err := findNotebookFile(pkg)
	if err != nil {
		return nil, nil, err
	}

	g := graph.New()
	var diags []graph.Diagnostic

	qualifier := types.RelativeTo(pkg.Types)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		cell, cellDiags, isCell := buildCell(pkg, fn, qualifier)
		if !isCell {
			continue
		}
		g.Add(cell)
		diags = append(diags, cellDiags...)
	}

	// Purity is derived from the call graph, never declared. Best-effort: a
	// failure to compute it leaves cells pure=false conservatively and does not
	// fail analysis (it is not a notebook error).
	annotatePurity(pkg, g)

	diags = append(diags, g.Check()...)
	return g, diags, nil
}

// findNotebookFile returns the single file in the package carrying the
// //go:notebook directive. It errors if there is none or more than one.
func findNotebookFile(pkg *packages.Package) (*ast.File, error) {
	var found *ast.File
	for _, f := range pkg.Syntax {
		if hasNotebookDirective(f) {
			if found != nil {
				return nil, fmt.Errorf("package %s has more than one %s file", pkg.Name, notebookDirective)
			}
			found = f
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no %s file found in package %s", notebookDirective, pkg.Name)
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
// The cell-detection rule: a cell is a top-level (non-method) func with a doc
// comment and at least one named, non-error result. A documented func whose
// results are all unnamed produces no symbols — it cannot be wired to anything
// — and is therefore an ordinary helper, invisible to the graph. This keeps the
// design's "the name is the edge" honest while letting documented helpers
// (capacity.go's erlangC, svg) stay helpers.
//
// The "unnamed result" diagnostic fires only for the genuinely ambiguous case:
// a cell that names some results but leaves a non-error result unnamed.
func buildCell(pkg *packages.Package, fn *ast.FuncDecl, q types.Qualifier) (*graph.Cell, []graph.Diagnostic, bool) {
	if fn.Recv != nil || fn.Doc == nil {
		return nil, nil, false // methods and undocumented funcs are never cells
	}
	obj := pkg.TypesInfo.Defs[fn.Name]
	if obj == nil {
		return nil, nil, false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return nil, nil, false
	}

	results := sig.Results()
	named, hasUnnamedData := classifyResults(results)
	if named == 0 {
		return nil, nil, false // no edges produced → a helper, not a cell
	}

	cell := &graph.Cell{
		ID:         graph.CellID(fn.Name.Name),
		Pos:        position(pkg.Fset, fn.Name.Pos()),
		Doc:        fn.Doc.Text(),
		Label:      label(fn),
		Directives: directives(fn),
		Params:     buildParams(pkg, sig.Params(), q),
		Results:    buildResults(pkg, results, q),
		Pure:       false, // set later by annotatePurity
	}

	var diags []graph.Diagnostic
	if hasUnnamedData {
		diags = append(diags, graph.Diagnostic{
			Pos:  cell.Pos,
			Msg:  fmt.Sprintf("cell %q has an unnamed result; cell results must be named — the name is the edge.", cell.ID),
			Hint: "name every result, or drop the doc comment to make this an ordinary helper",
		})
	}
	// Prev[T] folds are deferred this milestone. The Delayed edge kind and the
	// analyzer branch ship now (the seam), but a notebook that actually uses a
	// fold is reported as unsupported rather than silently mis-scheduled.
	for _, p := range cell.Params {
		if p.Kind == graph.Delayed {
			diags = append(diags, graph.Diagnostic{
				Pos:  p.Pos,
				Msg:  fmt.Sprintf("cell %q takes `%s %s`; Prev[T] folds are not supported in this milestone.", cell.ID, p.Name, p.Type),
				Hint: "stateful cells (a Tick-clocked fold) are a later milestone; this cell will not run yet",
			})
		}
	}
	return cell, diags, true
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
func buildParams(pkg *packages.Package, params *types.Tuple, q types.Qualifier) []graph.Param {
	var out []graph.Param
	for i := 0; i < params.Len(); i++ {
		v := params.At(i)
		out = append(out, graph.Param{
			Name: graph.Symbol(v.Name()),
			Type: types.TypeString(v.Type(), q),
			Kind: paramKind(v.Type()),
			Pos:  position(pkg.Fset, v.Pos()),
		})
	}
	return out
}

// buildResults renders each result into the IR, marking a trailing error.
func buildResults(pkg *packages.Package, results *types.Tuple, q types.Qualifier) []graph.Result {
	var out []graph.Result
	for i := 0; i < results.Len(); i++ {
		v := results.At(i)
		name := v.Name()
		if isBlank(name) {
			name = "" // a blank result names no symbol; do not index it
		}
		out = append(out, graph.Result{
			Name:    graph.Symbol(name),
			Type:    types.TypeString(v.Type(), q),
			IsError: isErrorType(v.Type()),
			Pos:     position(pkg.Fset, v.Pos()),
		})
	}
	return out
}

// paramKind classifies a parameter by its type:
//
//   - context.Context      → Injected (supplied by the runtime, not an edge)
//   - Prev[T]              → Delayed  (a previous-epoch self-edge; folds are
//     deferred, so the caller also emits an "unsupported" diagnostic)
//   - anything else        → Wired    (an ordinary edge)
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
func isErrorType(t types.Type) bool {
	return types.Identical(t, errorType)
}

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
// defined in the notebook package itself (a struct with a single Value field),
// so matching the type name is sufficient and needs no import.
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
