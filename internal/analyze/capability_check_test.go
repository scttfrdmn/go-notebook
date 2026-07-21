package analyze

import (
	"strings"
	"testing"
)

// TestCapabilityShapeCheck pins the near-miss control-capability validation: a
// correctly-shaped Bounds/Options/Reconcile is clean, a right-named but
// wrong-shaped one is diagnosed with the actual-vs-required signatures, Grip is
// never signature-checked (no engine contract), and a type with none of these is
// never flagged. This is the input-side sibling of TestRenderShapeCheck; it uses
// the same writeNotebook temp-dir harness.
func TestCapabilityShapeCheck(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantDiag   bool
		wantSubstr string
	}{
		{
			name: "correct Bounds is clean",
			src: `
type Rate struct{ Value float64 }
func (r Rate) Bounds() (float64, float64) { return 0, 1 }
// rate leaf.
func rate() (r Rate) { return Rate{} }
func use(r Rate) (o float64) { return r.Value }
`,
			wantDiag: false,
		},
		{
			name: "wrong Bounds (int) is diagnosed",
			src: `
type Rate struct{ Value int }
func (r Rate) Bounds() (int, int) { return 0, 100 }
// rate leaf.
func rate() (r Rate) { return Rate{} }
func use(r Rate) (o int) { return r.Value }
`,
			wantDiag:   true,
			wantSubstr: "declares Bounds() (int, int), but a control requires Bounds() (float64, float64)",
		},
		{
			name: "correct Options is clean",
			src: `
type Mode struct{ Value string }
func (m Mode) Options() []string { return []string{"a", "b"} }
// mode leaf.
func mode() (m Mode) { return Mode{} }
func use(m Mode) (o string) { return m.Value }
`,
			wantDiag: false,
		},
		{
			name: "wrong Options ([]int) is diagnosed",
			src: `
type Mode struct{ Value int }
func (m Mode) Options() []int { return []int{1, 2} }
// mode leaf.
func mode() (m Mode) { return Mode{} }
func use(m Mode) (o int) { return m.Value }
`,
			wantDiag:   true,
			wantSubstr: "declares Options() []int, but a control requires Options() []string",
		},
		{
			name: "wrong Reconcile is diagnosed",
			src: `
type W struct{ Value int }
func (w W) Reconcile(saved int) int { return saved }
// w leaf.
func w() (x W) { return W{} }
func use(x W) (o int) { return x.Value }
`,
			wantDiag:   true,
			wantSubstr: "but a control requires Reconcile(saved any) any",
		},
		{
			name: "Grip is not signature-checked (no engine contract)",
			src: `
type Pts struct{ Value []float64 }
// A Grip with an arbitrary notebook-defined signature must NOT be flagged.
func (p Pts) Grip(i int) string { return "" }
// pts leaf.
func pts() (p Pts) { return Pts{} }
func use(p Pts) (n int) { return len(p.Value) }
`,
			wantDiag: false,
		},
		{
			name: "no capability method is clean",
			src: `
type Plain struct{ Value int }
// plain leaf.
func plain() (p Plain) { return Plain{} }
func use(p Plain) (o int) { return p.Value }
`,
			wantDiag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeNotebook(t, tt.src)
			_, diags, err := TypesAnalyzer{}.Analyze(dir)
			if err != nil {
				t.Fatalf("analyze: %v", err)
			}
			rendered := renderDiagnostics(diags)
			if !tt.wantDiag {
				if strings.Contains(rendered, "render as a control") {
					t.Errorf("expected no capability diagnostic, got:\n%s", rendered)
				}
				return
			}
			if !strings.Contains(rendered, tt.wantSubstr) {
				t.Errorf("diagnostic missing %q, got:\n%s", tt.wantSubstr, rendered)
			}
		})
	}
}
