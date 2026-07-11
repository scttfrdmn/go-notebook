package analyze

import (
	"fmt"

	"golang.org/x/tools/go/packages"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// PackageInfo is the identity of the notebook package that codegen needs but
// the graph IR deliberately does not carry: where the package lives and what it
// is called. The graph is about dataflow; this is about where to write the
// generated registry so it can see the notebook's unexported cells.
type PackageInfo struct {
	// ImportPath is the notebook package's import path, e.g.
	// "github.com/scttfrdmn/go-notebook/examples/capacity".
	ImportPath string
	// Name is the package clause name, e.g. "capacity".
	Name string
	// Dir is the absolute directory containing the package's source.
	Dir string
	// GoFiles are the absolute paths of the package's non-generated Go files.
	GoFiles []string
}

// Analysis is the full result of analyzing a notebook package for codegen: the
// dependency graph, the package identity, and any diagnostics.
type Analysis struct {
	Graph       *graph.Graph
	Package     PackageInfo
	Diagnostics []graph.Diagnostic
}

// LoadPackage is the codegen entry point: it derives the graph exactly as
// [TypesAnalyzer.Analyze] does, and additionally returns the package identity
// codegen needs. The graph and diagnostics are identical to Analyze; this just
// surfaces the package metadata that the Analyzer interface intentionally omits
// (the interface is the gopls seam and stays graph-only).
func LoadPackage(dir string) (Analysis, error) {
	cfg := &packages.Config{Mode: graphLoadMode, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return Analysis{}, fmt.Errorf("loading package in %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return Analysis{}, fmt.Errorf("no Go package found in %s", dir)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return Analysis{}, fmt.Errorf("package %s has errors: %w", pkg.Name, pkg.Errors[0])
	}

	g, diags, err := buildFromTypes(pkg.Fset, pkg.Syntax, pkg.Types)
	if err != nil {
		return Analysis{}, err
	}
	return Analysis{
		Graph: g,
		Package: PackageInfo{
			ImportPath: pkg.PkgPath,
			Name:       pkg.Name,
			Dir:        dir,
			GoFiles:    append([]string(nil), pkg.GoFiles...),
		},
		Diagnostics: diags,
	}, nil
}
