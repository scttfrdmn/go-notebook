package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// mainImportPath is the import path suffix under which the synthesized main
// package virtually lives. It is a directory that does not exist in the user's
// tree; the overlay maps it to a temp backing file.
const buildSubdir = ".notebook-build"

// Build is the full codegen + overlay pipeline. It synthesizes the registry
// (in the notebook's package, so it sees unexported cells) and a tiny main
// package that imports it, writes both to a temp directory as backing files,
// and returns an [Overlay] describing the go build -overlay mapping plus the
// import path of the main package to build.
//
// Nothing is written into the user's source tree — the whole point of the
// overlay. The returned Overlay owns a temp dir the caller must Cleanup.
func Build(g *graph.Graph, info analyze.PackageInfo, moduleRoot string) (*Overlay, error) {
	mainSrc, err := MainPackage(g, info)
	if err != nil {
		return nil, err
	}
	return buildWith(g, info, moduleRoot, mainSrc)
}

// BuildWASM is Build for the browser target: it synthesizes the same registry
// but a syscall/js entry point (via [WASMMainPackage]) instead of the server
// main. The engine, scheduler, head, and cache are byte-identical — only the
// transport differs. Build with GOOS=js GOARCH=wasm against the returned
// [Overlay.MainDir].
func BuildWASM(g *graph.Graph, info analyze.PackageInfo, moduleRoot string) (*Overlay, error) {
	mainSrc, err := WASMMainPackage(g, info)
	if err != nil {
		return nil, err
	}
	return buildWith(g, info, moduleRoot, mainSrc)
}

// buildWith is the shared codegen+overlay core: it lays down the registry (in
// the notebook package) and the given main file (in a virtual build dir) and
// returns the overlay. The main file's name carries its own build constraints
// (main.go for the server, main_wasm.go for the //go:build js && wasm entry).
func buildWith(g *graph.Graph, info analyze.PackageInfo, moduleRoot string, mainSrc GeneratedFile) (*Overlay, error) {
	reg, err := Registry(g, info)
	if err != nil {
		return nil, err
	}

	tmp, err := os.MkdirTemp("", "notebook-build-")
	if err != nil {
		return nil, fmt.Errorf("creating temp build dir: %w", err)
	}
	ov := &Overlay{tmpDir: tmp, replace: map[string]string{}}

	// The registry file appears to live inside the notebook package directory.
	regBacking := filepath.Join(tmp, "notebook_gen.go")
	if err := os.WriteFile(regBacking, reg.Content, 0o600); err != nil {
		ov.Cleanup()
		return nil, fmt.Errorf("writing registry backing file: %w", err)
	}
	regVirtual := filepath.Join(info.Dir, reg.Name)
	ov.replace[regVirtual] = regBacking

	// The main package appears to live under <moduleRoot>/.notebook-build/main.
	// The file name carries the build constraints (main.go for the server,
	// main_wasm.go for the //go:build js && wasm entry point).
	mainBacking := filepath.Join(tmp, mainSrc.Name)
	if err := os.WriteFile(mainBacking, mainSrc.Content, 0o600); err != nil {
		ov.Cleanup()
		return nil, fmt.Errorf("writing main backing file: %w", err)
	}
	mainVirtual := filepath.Join(moduleRoot, buildSubdir, "main", mainSrc.Name)
	ov.replace[mainVirtual] = mainBacking
	ov.MainDir = filepath.Join(moduleRoot, buildSubdir, "main")

	// Write the overlay JSON itself into the temp dir.
	ov.JSONPath = filepath.Join(tmp, "overlay.json")
	if err := ov.writeJSON(); err != nil {
		ov.Cleanup()
		return nil, err
	}
	return ov, nil
}

// Overlay is a go build -overlay configuration backed by a temp directory. The
// user's source tree is never touched; the generated files exist only virtually
// (mapped in the overlay JSON) and physically only in the temp dir.
type Overlay struct {
	// JSONPath is the path to pass as `go build -overlay=<JSONPath>`.
	JSONPath string
	// MainDir is the (virtual) package directory to name as the build target,
	// e.g. "<module>/.notebook-build/main".
	MainDir string

	tmpDir  string
	replace map[string]string
}

// writeJSON serializes the overlay in the format go build expects:
// {"Replace": {virtualPath: backingPath, ...}}.
func (o *Overlay) writeJSON() error {
	doc := struct {
		Replace map[string]string
	}{Replace: o.replace}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling overlay: %w", err)
	}
	if err := os.WriteFile(o.JSONPath, data, 0o600); err != nil {
		return fmt.Errorf("writing overlay JSON: %w", err)
	}
	return nil
}

// TempDir returns the temp directory backing the overlay, a safe place to write
// the built binary (it is cleaned up with the overlay).
func (o *Overlay) TempDir() string { return o.tmpDir }

// Cleanup removes the temp directory backing the overlay. Safe to call on a
// partially-constructed Overlay and safe to call more than once.
func (o *Overlay) Cleanup() {
	if o == nil || o.tmpDir == "" {
		return
	}
	_ = os.RemoveAll(o.tmpDir)
}
