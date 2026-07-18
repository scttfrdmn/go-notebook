package analyze

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// parseSrc parses a Go source string into an *ast.File with comments.
func parseSrc(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// TestParseLayoutRows pins the layout grammar: one //notebook:layout directive
// per row, `|` splitting a row into columns of area-or-cell tokens.
func TestParseLayoutRows(t *testing.T) {
	src := `//go:notebook
//notebook:layout notes
//notebook:layout controls | readouts
//notebook:layout curve

package p
`
	got := parseLayout(parseSrc(t, src))
	want := [][]string{{"notes"}, {"controls", "readouts"}, {"curve"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLayout = %v, want %v", got, want)
	}
}

// TestParseLayoutDetachedFromPackage is the regression for the bug the vertical
// slice hit: a blank line between the layout lines and `package` detaches
// file.Doc (Go only attaches a doc comment immediately adjacent to package), so
// the parser must read from file.Comments, not file.Doc. Both adjacent and
// blank-line-separated forms must yield the same layout.
func TestParseLayoutDetachedFromPackage(t *testing.T) {
	adjacent := `//notebook:layout a | b
package p
`
	separated := `//notebook:layout a | b

package p
`
	ga := parseLayout(parseSrc(t, adjacent))
	gs := parseLayout(parseSrc(t, separated))
	want := [][]string{{"a", "b"}}
	if !reflect.DeepEqual(ga, want) || !reflect.DeepEqual(gs, want) {
		t.Errorf("adjacent=%v separated=%v, both want %v", ga, gs, want)
	}
}

// TestParseLayoutNoneIsNil confirms a notebook with no layout parses to nil, so
// the client falls back to source order (degrade-to-linear).
func TestParseLayoutNoneIsNil(t *testing.T) {
	src := "//go:notebook\npackage p\n"
	if got := parseLayout(parseSrc(t, src)); got != nil {
		t.Errorf("parseLayout with no layout = %v, want nil", got)
	}
}

// TestParseLayoutIgnoresCellDirectives confirms per-cell //notebook: directives
// below the package clause are NOT read as layout rows — only the package-level
// region (before `package`) contributes.
func TestParseLayoutIgnoresCellDirectives(t *testing.T) {
	src := `//notebook:layout inputs | chart
package p

//notebook:layout SHOULD_BE_IGNORED
func f() (x int) { return 0 }
`
	got := parseLayout(parseSrc(t, src))
	want := [][]string{{"inputs", "chart"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLayout = %v, want %v (cell-level directive must not count)", got, want)
	}
}

// TestLayoutSurvivesGofmt is the finding that shaped the syntax: a
// //notebook:layout line is a directive (no space after //), so gofmt preserves
// it verbatim — unlike an indented ASCII-art block, which gofmt reflows and
// reorders. This test is the guard that would have caught the block-syntax flaw.
func TestLayoutSurvivesGofmt(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	src := `//go:notebook
//notebook:layout notes
//notebook:layout controls | readouts
//notebook:layout curve

package p
`
	out, err := gofmtString(src)
	if err != nil {
		t.Fatalf("gofmt: %v", err)
	}
	// The layout must parse identically before and after gofmt — i.e. the
	// directive lines were preserved verbatim and not reflowed/reordered.
	before := parseLayout(parseSrc(t, src))
	after := parseLayout(parseSrc(t, out))
	if !reflect.DeepEqual(before, after) {
		t.Errorf("layout changed across gofmt:\nbefore %v\nafter  %v\n--- gofmt output ---\n%s", before, after, out)
	}
	if len(after) != 3 {
		t.Errorf("after gofmt, layout has %d rows, want 3 — gofmt mangled the directives:\n%s", len(after), out)
	}
}

// gofmtString runs gofmt over a source string and returns the formatted result.
func gofmtString(src string) (string, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.Output()
	return string(out), err
}
