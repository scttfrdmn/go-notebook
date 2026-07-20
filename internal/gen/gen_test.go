package gen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// TestRegistryImportsWiredEdgePackages is the regression for the check≠build gap
// that the wrap-existing-package example surfaced: a cell can take a wired input
// whose type comes from an imported package (there, a *regexp.Regexp produced by
// a compile cell). The registry asserts every wired input as `in[...].(T)`, so it
// must IMPORT that package or the generated file won't compile — even though the
// notebook itself type-checks fine. Before the fix the registry imported only
// context+engine, so this notebook passed `check` and failed `build` with
// "undefined: regexp". This pins both halves: the import is emitted, and the
// assertion that needs it is present.
func TestRegistryImportsWiredEdgePackages(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "examples", "minimal", "wrap-existing-package")

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	src := string(reg.Content)

	// The wired-input assertion that names the imported type...
	if !strings.Contains(src, `in["re"].(*regexp.Regexp)`) {
		t.Errorf("registry should assert the *regexp.Regexp edge; got:\n%s", src)
	}
	// ...and the import that makes it compile. Without this line the generated
	// file is invalid Go ("undefined: regexp") — the bug this test guards.
	if !regexp.MustCompile(`(?m)^\s*"regexp"\s*$`).MatchString(src) {
		t.Errorf("registry must import \"regexp\" for the wired *regexp.Regexp edge; got:\n%s", src)
	}

	// And it really is valid Go end to end (the assertion + its import agree).
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, reg.Name, reg.Content, parser.AllErrors); err != nil {
		t.Fatalf("generated registry is not valid Go: %v\n%s", err, reg.Content)
	}

	// A RESULT-only imported type must NOT drag in an import: results are assigned,
	// never asserted, so they need nothing. The view types here (Highlight, …) are
	// local, and no result type is a foreign package — so the ONLY edge import is
	// regexp. This pins that we import for wired inputs, not for every type seen.
	if got := len(res.Graph.Imports); got != 1 {
		t.Errorf("expected exactly one wired-edge import (regexp), got %d: %+v", got, res.Graph.Imports)
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

	// utilization consumes offeredLoad (a) and servers (c). Match the edge
	// substring (the line also carries Source, which we don't pin here). It now
	// also carries area=readouts (its presentation region under the layout), so
	// the directive map is no longer nil — the edge is what this test pins.
	if !strings.Contains(src, `ID: "utilization", Leaf: "", Label: "Server utilization.", Directives: map[string]string{"area": "readouts"}, In: []engine.CellID{"offeredLoad", "servers"}, Source:`) {
		t.Errorf("utilization's In should be {offeredLoad, servers}")
	}
	// A source leaf has no upstream — In is nil, not an empty slice literal. It
	// now also carries area=controls (its presentation region).
	if !strings.Contains(src, `ID: "arrivalRate", Leaf: "lambda", Label: "Incoming jobs per hour.", Directives: map[string]string{"area": "controls", "max": "5000", "min": "0", "slider": "", "step": "50"}, In: nil, Source:`) {
		t.Errorf("a source leaf should have In: nil (no upstream)")
	}
}

// TestMetaCarriesLeafType pins B4b: a leaf's CellMeta.Type surfaces its Go result
// type in two coordinates — the declared Name and the resolved Underlying kind —
// so a client can validate a set() value's shape without knowing Go. Two shapes:
// a NAMED type over a basic kind (lambda is PerHour over float64), and a BARE
// basic type (c is a plain int). A non-leaf cell carries no Type (nil).
func TestMetaCarriesLeafType(t *testing.T) {
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

	// A named type resolves to its underlying basic kind — the value that makes
	// set() coercible is the float64, but the client also learns it's a PerHour.
	// Anchor Leaf and Type to the same record so the type is pinned to lambda,
	// not merely present somewhere in the file.
	lambdaLine := regexp.MustCompile(`Leaf: "lambda",[^\n]*Type: &engine.LeafType{Name: "PerHour", Underlying: "float64"}}`)
	if !lambdaLine.MatchString(src) {
		t.Errorf("lambda leaf should carry Type PerHour/float64 on its own record")
	}
	// A bare int: Name and Underlying are both "int".
	cLine := regexp.MustCompile(`Leaf: "c",[^\n]*Type: &engine.LeafType{Name: "int", Underlying: "int"}}`)
	if !cLine.MatchString(src) {
		t.Errorf("c leaf should carry Type int/int on its own record")
	}
	// A non-leaf cell has no Type. Anchor to utilization's own line (Widget and
	// Type together on the same emitted record) so the assertion can't be
	// satisfied by some other cell's trailing "Widget: nil, Type: nil}".
	utilLine := regexp.MustCompile(`ID: "utilization",[^\n]*Widget: nil, Type: nil}`)
	if !utilLine.MatchString(src) {
		t.Errorf("a non-leaf cell (utilization) should carry Widget: nil, Type: nil on its own record")
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
