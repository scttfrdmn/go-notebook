package gen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
)

// TestWASMMainIsValidGo confirms the generated browser entry point parses as Go,
// carries the js/wasm build tag, and drives the engine over the wasm transport
// (not the server).
func TestWASMMainIsValidGo(t *testing.T) {
	root := moduleRoot(t)
	res, err := analyze.LoadPackage(filepath.Join(root, "examples", "capacity"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := WASMMainPackage(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("WASMMainPackage: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), m.Name, m.Content, parser.AllErrors); err != nil {
		t.Fatalf("generated wasm main is not valid Go: %v\n%s", err, m.Content)
	}
	src := string(m.Content)
	for _, want := range []string{
		"//go:build js && wasm",
		"engine/wasm",
		"wasm.RunNotebook(",
		"NotebookProvenance",
		"NotebookCells",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated wasm main missing %q", want)
		}
	}
	// It must NOT pull in the server.
	if strings.Contains(src, "engine/server") {
		t.Error("wasm main should not import engine/server")
	}
}

// TestBuildWASMLinks confirms the wasm build pipeline produces a binary that
// links under GOOS=js GOARCH=wasm — the topology test, end to end through
// codegen + overlay.
func TestBuildWASMLinks(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes go build; skipped in -short mode")
	}
	root := moduleRoot(t)
	res, err := analyze.LoadPackage(filepath.Join(root, "examples", "capacity"))
	if err != nil {
		t.Fatal(err)
	}
	ov, err := BuildWASM(res.Graph, res.Package, root)
	if err != nil {
		t.Fatalf("BuildWASM: %v", err)
	}
	defer ov.Cleanup()

	out := filepath.Join(t.TempDir(), "notebook.wasm")
	cmd := exec.Command("go", "build", "-overlay="+ov.JSONPath, "-o", out, ov.MainDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wasm build failed: %v\n%s", err, combined)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("expected a non-empty .wasm at %s: %v", out, err)
	}
}
