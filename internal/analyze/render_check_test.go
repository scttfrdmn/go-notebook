package analyze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderShapeCheck pins the render-shape validation across the cases that
// matter: a correct shape is clean, common typos are diagnosed, and a cell with
// no Render() method is never flagged.
func TestRenderShapeCheck(t *testing.T) {
	// Each case is a full notebook package written to a temp dir and analyzed.
	tests := []struct {
		name       string
		src        string
		wantDiag   bool
		wantSubstr string
	}{
		{
			name: "correct shape is clean",
			src: `
type Rendered struct { MIME string; Data string }
type Chart struct{}
func (Chart) Render() Rendered { return Rendered{} }
// A chart.
func chart() (c Chart) { return Chart{} }
`,
			wantDiag: false,
		},
		{
			name: "Mime typo is diagnosed",
			src: `
type Rendered struct { Mime string; Data string }
type Chart struct{}
func (Chart) Render() Rendered { return Rendered{} }
// A chart.
func chart() (c Chart) { return Chart{} }
`,
			wantDiag:   true,
			wantSubstr: "MIME",
		},
		{
			name: "Data wrong type is diagnosed",
			src: `
type Rendered struct { MIME string; Data []byte }
type Chart struct{}
func (Chart) Render() Rendered { return Rendered{} }
// A chart.
func chart() (c Chart) { return Chart{} }
`,
			wantDiag:   true,
			wantSubstr: "Data",
		},
		{
			name: "no Render method is fine",
			src: `
// A plain scalar cell.
func n() (x int) { return 1 }
`,
			wantDiag: false,
		},
		{
			name: "pointer-receiver Render is checked",
			src: `
type Rendered struct { MIME string; Data string }
type Chart struct{}
func (*Chart) Render() Rendered { return Rendered{} }
// A chart.
func chart() (c Chart) { return Chart{} }
`,
			wantDiag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeNotebook(t, tt.src)
			_, diags, err := TypesAnalyzer{}.Analyze(dir)
			if err != nil {
				t.Fatal(err)
			}
			var renderDiags []string
			for _, d := range diags {
				if strings.Contains(d.Msg, "Render()") {
					renderDiags = append(renderDiags, d.String())
				}
			}
			if tt.wantDiag && len(renderDiags) == 0 {
				t.Errorf("expected a render-shape diagnostic, got none (all diags: %v)", diags)
			}
			if !tt.wantDiag && len(renderDiags) > 0 {
				t.Errorf("expected no render-shape diagnostic, got: %v", renderDiags)
			}
			if tt.wantSubstr != "" {
				found := false
				for _, d := range renderDiags {
					if strings.Contains(d, tt.wantSubstr) {
						found = true
					}
				}
				if !found {
					t.Errorf("diagnostic missing %q, got: %v", tt.wantSubstr, renderDiags)
				}
			}
		})
	}
}

// writeNotebook writes a minimal //go:notebook package (with a go.mod so it
// loads standalone) to a temp dir and returns the dir.
func writeNotebook(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, dir, "go.mod", "module nbtest\n\ngo 1.25\n")
	mustWrite(t, dir, "nb.go", "//go:notebook\npackage nbtest\n"+body)
	return dir
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
