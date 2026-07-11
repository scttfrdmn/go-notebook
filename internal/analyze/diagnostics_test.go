package analyze

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// TestDiagnostics analyzes every fixture under testdata/errors and
// testdata/cycles and compares the rendered diagnostics against a .want.txt
// golden. Diagnostic message quality is a feature of this project, so it is
// pinned character-for-character — with fixture-relative paths so the golden is
// machine-independent.
func TestDiagnostics(t *testing.T) {
	roots := []string{"testdata/errors", "testdata/cycles", "testdata/notices"}
	for _, root := range roots {
		dirs, err := filepath.Glob(root + "/*")
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() {
				continue
			}
			dir := dir
			t.Run(filepath.Base(dir), func(t *testing.T) {
				_, diags, err := TypesAnalyzer{}.Analyze(dir)
				if err != nil {
					t.Fatalf("analyze: %v", err)
				}
				if len(diags) == 0 {
					t.Fatalf("expected diagnostics for %s, got none", dir)
				}
				got := renderDiagnostics(diags)

				goldenPath := dir + ".want.txt"
				if *update {
					if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("reading golden (run with -update to create): %v", err)
				}
				if got != string(want) {
					t.Errorf("diagnostics mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
						filepath.Base(dir), got, want)
				}
			})
		}
	}
}

// renderDiagnostics formats diagnostics deterministically for golden
// comparison, rewriting the absolute filename to just its base so the golden is
// portable across machines.
func renderDiagnostics(diags []graph.Diagnostic) string {
	var b bytes.Buffer
	for _, d := range diags {
		d.Pos.Filename = filepath.Base(d.Pos.Filename)
		if d.HintPos != nil {
			hp := *d.HintPos
			hp.Filename = filepath.Base(hp.Filename)
			d.HintPos = &hp
		}
		b.WriteString(d.String())
		b.WriteString("\n")
	}
	return b.String()
}

// TestDiagnosticFormat pins the exact rendered shape of the flagship
// missing-producer diagnostic — the "did you mean" message that is the
// difference between a tool and a toy.
func TestDiagnosticFormat(t *testing.T) {
	_, diags, err := TypesAnalyzer{}.Analyze("testdata/errors/missing")
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderDiagnostics(diags)
	for _, want := range []string{
		`cell "utilization" needs ` + "`a Erlangs`" + `, but no cell produces it.`,
		`Did you mean ` + "`offeredLoad`" + `, which produces ` + "`Erlangs`?",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("diagnostic missing %q\ngot:\n%s", want, rendered)
		}
	}
}
