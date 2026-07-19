package analyze

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// FuzzParseLayout drives the package-level layout parser with arbitrary comment
// text placed where a //notebook:layout block lives. The parser reads untrusted
// author input from a file's leading comments; its only obligation is to never
// panic and to return well-formed rows (no empty rows, no empty tokens) whatever
// the bytes are.
func FuzzParseLayout(f *testing.F) {
	for _, seed := range []string{
		"controls | curve",
		"a | b | c",
		"", "|", "||", "   |   ",
		"one\n//notebook:layout two",
		"a|b\tc | d",
		"日本語 | 変数",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body string) {
		// Build a source file whose package doc carries the fuzzed content as a
		// //notebook:layout directive. A newline in the body would start a new
		// comment line, so fold it — the point is to fuzz the token/`|` splitting.
		line := strings.ReplaceAll(body, "\n", " ")
		src := "//go:notebook\n//notebook:layout " + line + "\npackage p\n"
		file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ParseComments)
		if err != nil {
			return // fuzzer produced something unparseable as Go — not our path
		}

		rows := parseLayout(file) // must not panic
		for _, row := range rows {
			if len(row) == 0 {
				t.Fatalf("parseLayout produced an empty row from %q", body)
			}
			for _, tok := range row {
				if tok == "" || tok != strings.TrimSpace(tok) {
					t.Fatalf("parseLayout produced an untrimmed/empty token %q from %q", tok, body)
				}
			}
		}
	})
}

// FuzzDirectives drives the per-cell //notebook: directive parser with arbitrary
// directive bodies. It reads untrusted author input; it must never panic and
// must return either nil or a map with no empty keys.
func FuzzDirectives(f *testing.F) {
	for _, seed := range []string{
		"slider min=0 max=100 step=1",
		"area=controls", "slider", "k=v=w", "=bad", "  ", "a=b c=d",
		"height=320 area=readouts",
		"key=", "===", "\t\tslider\t",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body string) {
		line := strings.ReplaceAll(body, "\n", " ")
		src := "package p\n\n//notebook:" + line + "\nfunc c() (x int) { return 0 }\n"
		file, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ParseComments)
		if err != nil {
			return
		}
		// Find the func decl and hand it to directives().
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				m := directives(fn) // must not panic
				for k := range m {
					if k == "" {
						t.Fatalf("directives produced an empty key from %q", body)
					}
				}
			}
		}
	})
}
