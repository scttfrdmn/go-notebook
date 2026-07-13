package main

import (
	"crypto/sha256"
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

	// Build to a temp path, then rename to notebook-<hash>.wasm where the hash is
	// of the COMPILED BYTES (the true artifact identity — the thing actually
	// served). A content-addressed filename gives an immutable URL: GitHub Pages
	// serves .wasm with max-age=600 and no revalidation, so a fixed name is served
	// stale for 10 min after a redeploy; a different name for different bytes
	// cannot be. A path is not a handle; the filename now IS the handle.
	tmpWASM := filepath.Join(outDir, ".notebook.wasm.tmp")
	absTmp, _ := filepath.Abs(tmpWASM)
	cmd := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", absTmp, overlay.MainDir)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stderr = os.Stderr
	buildStart := time.Now()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: go build (wasm): %v\n", err)
		return 1
	}
	buildElapsed := time.Since(buildStart)

	wasmName, err := contentAddress(absTmp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}
	absWASM := filepath.Join(outDir, wasmName)
	if err := os.Rename(absTmp, absWASM); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	if err := writeHostFiles(outDir, res.Package.Name, wasmName); err != nil {
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

// contentAddress returns the content-addressed filename for a built wasm file:
// notebook-<sha256[:12]>.wasm, hashing the compiled bytes. The bytes are the
// artifact's true identity, so the filename changes iff the served program does.
func contentAddress(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("hashing wasm: %w", err)
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("notebook-%x.wasm", sum[:6]), nil
}

// writeHostFiles copies Go's wasm_exec.js (the JS runtime shim, version-matched
// to the toolchain) and writes index.html next to the .wasm. wasmName is the
// content-addressed filename the page must fetch.
func writeHostFiles(outDir, name, wasmName string) error {
	shim, err := wasmExecPath()
	if err != nil {
		return err
	}
	if err := copyFile(shim, filepath.Join(outDir, "wasm_exec.js")); err != nil {
		return fmt.Errorf("copying wasm_exec.js: %w", err)
	}
	// Insert the name + content-addressed wasm filename by replace, not Sprintf:
	// the shared webui CSS/JS contain literal %, which a format verb would choke on.
	html := strings.ReplaceAll(indexHTMLWASM, "__NB_NAME__", name)
	html = strings.ReplaceAll(html, "__NB_WASM__", wasmName)
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
