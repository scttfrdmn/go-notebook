package analyze

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// Session is the interactive re-analysis path: it loads the notebook package
// once, caches its dependency types, and thereafter re-derives the graph by
// re-typechecking only the notebook package's source against that cache.
//
// This is the number the whole milestone exists to produce (KC2: re-analysis
// after a one-cell edit). A full packages.Load reruns `go list` and reloads the
// world (~hundreds of ms); a re-typecheck against a cached importer is
// sub-millisecond, because the dependency graph has not changed on an edit —
// only the notebook's own file contents have.
//
// A Session is not safe for concurrent use; drive it from a single edit loop.
type Session struct {
	pkgPath  string
	goFiles  []string       // absolute paths of the package's Go files
	importer types.Importer // cached dependency *types.Package values
	sizes    types.Sizes
}

// NewSession primes a Session for the notebook package in dir. It performs one
// graph-mode load (no NeedDeps) to obtain the dependency types and the file
// list, then holds them for fast re-analysis. The prime cost is the ~cold-load
// number; every subsequent Reanalyze is the interactive number.
func NewSession(dir string) (*Session, error) {
	cfg := &packages.Config{Mode: graphLoadMode, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("priming session in %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go package found in %s", dir)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package %s has errors: %w", pkg.Name, pkg.Errors[0])
	}

	imp := cachedImporter{}
	for path, ip := range pkg.Imports {
		if ip.Types != nil {
			imp[path] = ip.Types
		}
	}

	s := &Session{
		pkgPath:  pkg.PkgPath,
		goFiles:  append([]string(nil), pkg.GoFiles...),
		importer: imp,
		sizes:    types.SizesFor("gc", "amd64"),
	}
	return s, nil
}

// Reanalyze re-parses and re-typechecks the notebook package from disk and
// derives the graph. Dependency types are served from the primed cache, so no
// packages.Load and no `go list` runs. This is the KC2 path.
func (s *Session) Reanalyze() (*graph.Graph, []graph.Diagnostic, error) {
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(s.goFiles))
	for _, gf := range s.goFiles {
		f, err := parser.ParseFile(fset, gf, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", gf, err)
		}
		files = append(files, f)
	}

	conf := types.Config{
		Importer: s.importer,
		Sizes:    s.sizes,
		// Errors are collected rather than aborting; a notebook mid-edit may
		// not typecheck, and we still want to report what we can.
		Error: func(error) {},
	}
	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
	}
	tpkg, err := conf.Check(s.pkgPath, fset, files, info)
	if err != nil {
		// Type errors are surfaced by go build against the user's real file;
		// for graph derivation we proceed with whatever type info resolved.
		if tpkg == nil {
			return nil, nil, fmt.Errorf("re-typechecking %s: %w", s.pkgPath, err)
		}
	}

	return buildFromTypes(fset, files, tpkg)
}

// cachedImporter serves dependency packages from a fixed map captured at prime
// time. On an edit the dependency set does not change, so a miss means the
// notebook grew a genuinely new import — which requires a re-prime.
type cachedImporter map[string]*types.Package

func (c cachedImporter) Import(path string) (*types.Package, error) {
	if p, ok := c[path]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("package %q not in the primed importer cache; re-prime the session", path)
}
