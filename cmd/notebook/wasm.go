package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/gen"
)

// buildWASM cross-compiles the notebook to GOOS=js GOARCH=wasm and writes a
// self-contained host directory: notebook.wasm + wasm_exec.js + index.html. The
// same registry/engine/scheduler/head as native — only the transport differs.
//
// It refuses a notebook that isn't WASM-able, deciding that from the graph (the
// WASMability analysis), never by hand: a cell that transitively touches
// net/os/cgo can't run client-side.
func buildWASM(res analyze.Analysis, moduleRoot, out string, timing bool) int {
	// Gate on WASM-ability, from the graph.
	pkg, err := analyze.LoadForPurity(res.Package.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}
	analyze.WASMability(pkg, res.Graph)
	if ok, blockers := analyze.NotebookWASMable(res.Graph); !ok {
		fmt.Fprintf(os.Stderr, "notebook build --target=wasm: %q is not WASM-able.\n", res.Package.Name)
		fmt.Fprintf(os.Stderr, "  These cells transitively touch net/os/cgo, which has no browser equivalent:\n    %v\n", blockers)
		fmt.Fprintln(os.Stderr, "  (A cell using fmt on numbers may be flagged conservatively via fmt→os; that is a lower bound, not a hard no.)")
		return 1
	}

	outDir := out
	if outDir == "" {
		outDir = "./" + res.Package.Name + "-wasm"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	genStart := time.Now()
	overlay, err := gen.BuildWASM(res.Graph, res.Package, moduleRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: codegen: %v\n", err)
		return 1
	}
	defer overlay.Cleanup()

	wasmPath := filepath.Join(outDir, "notebook.wasm")
	absWASM, _ := filepath.Abs(wasmPath)
	cmd := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", absWASM, overlay.MainDir)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stderr = os.Stderr
	buildStart := time.Now()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: go build (wasm): %v\n", err)
		return 1
	}
	buildElapsed := time.Since(buildStart)

	if err := writeHostFiles(outDir, res.Package.Name); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	info, _ := os.Stat(absWASM)
	fmt.Fprintf(os.Stderr, "built %s (%d KB)\n  serve %s/ over HTTP and open index.html\n",
		absWASM, info.Size()/1024, outDir)
	if timing {
		fmt.Fprintf(os.Stderr, "codegen+build: %v (build %v)\n", time.Since(genStart), buildElapsed)
	}
	return 0
}

// writeHostFiles copies Go's wasm_exec.js (the JS runtime shim, version-matched
// to the toolchain) and writes index.html next to the .wasm.
func writeHostFiles(outDir, name string) error {
	shim, err := wasmExecPath()
	if err != nil {
		return err
	}
	if err := copyFile(shim, filepath.Join(outDir, "wasm_exec.js")); err != nil {
		return fmt.Errorf("copying wasm_exec.js: %w", err)
	}
	// Insert the name by replace, not Sprintf: the shared webui CSS/JS contain
	// literal %, which a format verb would choke on.
	html := strings.ReplaceAll(indexHTMLWASM, "__NB_NAME__", name)
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0o644); err != nil {
		return fmt.Errorf("writing index.html: %w", err)
	}
	return nil
}

// wasmExecPath locates the toolchain's wasm_exec.js (its location moved between
// Go versions: lib/wasm in 1.24+, misc/wasm before).
func wasmExecPath() (string, error) {
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := string(root[:len(root)-1]) // trim newline
	for _, rel := range []string{"lib/wasm/wasm_exec.js", "misc/wasm/wasm_exec.js"} {
		p := filepath.Join(goroot, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("wasm_exec.js not found under %s", goroot)
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	// A Close error on the destination can mean a lost write, so surface it.
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
