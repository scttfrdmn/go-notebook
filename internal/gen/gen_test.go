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

// moduleRoot walks up from the test's working directory to the go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found")
		}
		dir = parent
	}
}

// TestRegistryIsValidGo confirms the generated registry parses as Go and
// contains the expected declarations. This catches codegen producing malformed
// source without needing a full build.
func TestRegistryIsValidGo(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "examples", "capacity")

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, reg.Name, reg.Content, parser.AllErrors); err != nil {
		t.Fatalf("generated registry is not valid Go: %v\n%s", err, reg.Content)
	}

	src := string(reg.Content)
	for _, want := range []string{
		"package capacity",
		"var NotebookCells = []engine.Node{",
		"var NotebookMeta = []engine.CellMeta{",
		`id:   "utilization"`,
		`in["a"].(Erlangs)`, // type-asserted input, safe by construction
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated registry missing %q", want)
		}
	}
}

// TestMetaCarriesDependencyEdges confirms the graph view's data: each cell's
// CellMeta.In lists the cells whose output it consumes, derived from the wiring
// (§8 — assert the actual edges, not that a field exists). utilization(a, c)
// reads a from offeredLoad and c from servers; a source leaf like arrivalRate
// has no upstream.
func TestMetaCarriesDependencyEdges(t *testing.T) {
	root := moduleRoot(t)
	res, err := analyze.LoadPackage(filepath.Join(root, "examples", "capacity"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	src := string(reg.Content)

	// utilization consumes offeredLoad (a) and servers (c).
	for _, want := range []string{
		`{ID: "utilization", Leaf: "", Label: "Server utilization.", Directives: nil, In: []engine.CellID{"offeredLoad", "servers"}}`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("meta missing expected edge line:\n  want substring: %s", want)
		}
	}
	// A source leaf has no upstream — In is nil, not an empty slice literal.
	if !strings.Contains(src, `{ID: "arrivalRate", Leaf: "lambda", Label: "Incoming jobs per hour.", Directives: map[string]string{"max": "5000", "min": "0", "slider": "", "step": "50"}, In: nil}`) {
		t.Errorf("a source leaf should have In: nil (no upstream)")
	}
}

// TestBuildProducesBinaryAndLeavesTreeClean is the M2 done-condition: build the
// capacity example to a binary, and assert the user's source directory is
// untouched (no notebook_gen.go, no .notebook-build).
func TestBuildProducesBinaryAndLeavesTreeClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build (invokes go build) in -short mode")
	}
	root := moduleRoot(t)
	dir := filepath.Join(root, "examples", "capacity")

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	ov, err := Build(res.Graph, res.Package, root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ov.Cleanup()

	out := filepath.Join(t.TempDir(), "capacity_nb")
	cmd := exec.Command("go", "build", "-overlay="+ov.JSONPath, "-o", out, ov.MainDir)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, combined)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected a binary at %s: %v", out, err)
	}

	// The user's source tree must be untouched: neither the registry nor the
	// build dir may exist on disk.
	if _, err := os.Stat(filepath.Join(dir, "notebook_gen.go")); !os.IsNotExist(err) {
		t.Error("notebook_gen.go leaked into the user's source directory")
	}
	if _, err := os.Stat(filepath.Join(root, buildSubdir)); !os.IsNotExist(err) {
		t.Errorf("%s leaked into the module root", buildSubdir)
	}
}

// TestFoldCellOmittedFromRegistry confirms a cell with a Prev[T] parameter is
// left out of the executable registry (it cannot run yet) while the runnable
// cells remain, so a notebook using a deferred feature still builds its subset.
func TestFoldCellOmittedFromRegistry(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "examples", "queue")

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatal(err)
	}
	src := string(reg.Content)

	// sim (the fold) must NOT appear as a registered cell...
	if strings.Contains(src, `id:   "sim"`) {
		t.Error("fold cell sim should be omitted from the executable registry")
	}
	// ...but a runnable cell must.
	if !strings.Contains(src, `id:   "depth"`) {
		t.Error("runnable cell depth should be in the registry")
	}
	// sim still gets metadata (the view layer wants it).
	if !strings.Contains(src, `ID: "sim"`) {
		t.Error("fold cell sim should still appear in NotebookMeta")
	}
}
