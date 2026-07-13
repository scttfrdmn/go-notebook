package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
)

// BenchmarkKC4BuildWarm measures the dominant term of the KC4 edit loop: codegen
// + a warm-cache `go build` of the single notebook package. Analyze (KC2,
// ~0.5ms) and restart+head-reload (~0ms) are measured elsewhere; go build is
// the term that decides whether interactive editing works. Target for the whole
// KC4 loop is <500ms.
//
// The module and build caches are warmed by an initial build outside the timed
// loop, so this is the steady-state edit-rebuild cost, not a cold first build.
func BenchmarkKC4BuildWarm(b *testing.B) {
	benchBuildGate(b, "capacity")
}

// BenchmarkKC4BuildWarmLego is the scale point (#73): lego is the largest
// buildable notebook (575 lines, 16 cells vs capacity's 239/~15). The build
// gate is the dominant, transport-independent term of the KC4 loop; measuring it
// on the biggest notebook is what confirms the interactive tier holds at scale.
func BenchmarkKC4BuildWarmLego(b *testing.B) {
	benchBuildGate(b, "lego")
}

func benchBuildGate(b *testing.B, example string) {
	root := benchModuleRoot(b)
	dir := filepath.Join(root, "examples", example)

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		b.Fatal(err)
	}
	out := filepath.Join(b.TempDir(), "nb")

	build := func() {
		ov, err := Build(res.Graph, res.Package, root)
		if err != nil {
			b.Fatal(err)
		}
		defer ov.Cleanup()
		cmd := exec.Command("go", "build", "-overlay="+ov.JSONPath, "-o", out, ov.MainDir)
		cmd.Dir = root
		if combined, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("go build: %v\n%s", err, combined)
		}
	}

	build() // warm the caches
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		build()
	}
}

func benchModuleRoot(b *testing.B) string {
	b.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		b.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("no go.mod found")
		}
		dir = parent
	}
}
