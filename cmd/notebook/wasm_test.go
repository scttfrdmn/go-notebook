package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildWASMCapacity drives the full wasm build command on capacity and
// asserts the self-contained host dir: a content-addressed notebook-<hash>.wasm
// + wasm_exec.js + index.html, the wasm non-empty, and the page fetching the
// hashed filename (KC11 — the immutable-URL invariant that a redeploy cannot be
// served stale).
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

	// The wasm is content-addressed: notebook-<hash>.wasm, non-empty, exactly one.
	matches, _ := filepath.Glob(filepath.Join(outDir, "notebook-*.wasm"))
	if len(matches) != 1 {
		t.Fatalf("want exactly one content-addressed notebook-<hash>.wasm, got %v", matches)
	}
	wasmName := filepath.Base(matches[0])
	if fi, err := os.Stat(matches[0]); err != nil || fi.Size() == 0 {
		t.Errorf("content-addressed wasm missing or empty: %v", err)
	}
	// A fixed notebook.wasm must NOT exist — a fixed name is the stale-cache bug.
	if _, err := os.Stat(filepath.Join(outDir, "notebook.wasm")); err == nil {
		t.Error("a fixed notebook.wasm exists; the filename must be content-addressed")
	}
	for _, f := range []string{"wasm_exec.js", "index.html"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("missing host file %s: %v", f, err)
		}
	}

	// The host page must fetch the HASHED wasm filename (not a fixed one) and the
	// JS bridge.
	html, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), wasmName) {
		t.Errorf("index.html must reference the content-addressed %q", wasmName)
	}
	for _, want := range []string{"wasm_exec.js", "notebookSet", "__notebook_event"} {
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
	// It must not have produced a wasm binary (any name).
	if m, _ := filepath.Glob(filepath.Join(outDir, "notebook*.wasm")); len(m) > 0 {
		t.Errorf("a non-WASM-able notebook should not produce a wasm binary, got %v", m)
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
