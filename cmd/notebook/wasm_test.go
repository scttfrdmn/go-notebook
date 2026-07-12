package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildWASMCapacity drives the full wasm build command on capacity and
// asserts the self-contained host dir: notebook.wasm + wasm_exec.js +
// index.html, the wasm non-empty.
func TestBuildWASMCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes go build (wasm); skipped in -short mode")
	}
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "cap-wasm")
	code := cmdBuild([]string{"--target=wasm", "-o", outDir, filepath.Join(root, "examples", "capacity")})
	if code != 0 {
		t.Fatalf("build --target=wasm returned %d", code)
	}
	for _, f := range []string{"notebook.wasm", "wasm_exec.js", "index.html"} {
		fi, err := os.Stat(filepath.Join(outDir, f))
		if err != nil {
			t.Errorf("missing host file %s: %v", f, err)
			continue
		}
		if f == "notebook.wasm" && fi.Size() == 0 {
			t.Error("notebook.wasm is empty")
		}
	}
	// The host page must reference the wasm and the JS bridge.
	html, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"notebook.wasm", "wasm_exec.js", "notebookSet", "__notebook_event"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}

// TestBuildWASMRejectsNonPortable confirms the command refuses a notebook that
// isn't WASM-able (seam fetches an image over net/http), decided from the graph.
func TestBuildWASMRejectsNonPortable(t *testing.T) {
	if testing.Short() {
		t.Skip("loads full deps; skipped in -short mode")
	}
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "seam-wasm")
	code := cmdBuild([]string{"--target=wasm", "-o", outDir, filepath.Join(root, "examples", "seam")})
	if code == 0 {
		t.Error("build --target=wasm should refuse seam (touches net/http), got exit 0")
	}
	// It must not have produced a wasm binary.
	if _, err := os.Stat(filepath.Join(outDir, "notebook.wasm")); err == nil {
		t.Error("a non-WASM-able notebook should not produce notebook.wasm")
	}
}

// TestWASMExecPathResolves confirms the toolchain's wasm_exec.js is found.
func TestWASMExecPathResolves(t *testing.T) {
	p, err := wasmExecPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("wasmExecPath returned %q which does not exist: %v", p, err)
	}
}
