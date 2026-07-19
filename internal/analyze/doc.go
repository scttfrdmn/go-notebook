package analyze

import (
	"go/ast"
	"go/doc/comment"
	"strings"
)

// directivePrefix marks a notebook presentation directive. It sits in the same
// register as //go:generate: a convention any tool may honor, carrying no
// mention of this project in the notebook file.
const directivePrefix = "//notebook:"

// label returns the cell's display label: the first sentence (first paragraph)
// of its doc comment, or the function name if the comment is empty.
//
// It uses go/doc/comment rather than go/doc.Synopsis because Synopsis truncates
// at the first "." — including inside abbreviations like "Cost vs. latency",
// which is exactly the kind of label a systems notebook writes. The comment
// parser splits on paragraph boundaries instead, so the whole first sentence
// survives.
func label(fn *ast.FuncDecl) string {
	if fn.Doc == nil {
		return fn.Name.Name
	}
	// fn.Doc.Text() already strips directive comments (//notebook:...) and the
	// leading "// " markers, leaving clean prose.
	text := fn.Doc.Text()
	if strings.TrimSpace(text) == "" {
		return fn.Name.Name
	}

	var p comment.Parser
	doc := p.Parse(text)
	if len(doc.Content) == 0 {
		return fn.Name.Name
	}
	para, ok := doc.Content[0].(*comment.Paragraph)
	if !ok {
		// First block is not prose (e.g. a heading like "## Title"); fall back
		// to the first non-empty raw line.
		return firstNonEmptyLine(text)
	}

	var b strings.Builder
	for _, t := range para.Text {
		if plain, ok := t.(comment.Plain); ok {
			b.WriteString(string(plain))
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return fn.Name.Name
	}
	return firstSentence(s)
}

// firstSentence returns the first sentence of a single paragraph.
//
// The boundary is a sentence-terminating period, question mark, or exclamation
// point followed by whitespace and an upper-case letter. That heuristic keeps
// abbreviations intact — "Cost vs. latency" does not split at "vs." because
// "latency" is lower-case — while still ending the label at a real boundary:
// "Server utilization. Deliberately..." splits after "utilization.".
func firstSentence(s string) string {
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '.', '?', '!':
			// Look for whitespace then an upper-case letter.
			j := i + 1
			if j >= len(runes) || !isSpace(runes[j]) {
				continue
			}
			for j < len(runes) && isSpace(runes[j]) {
				j++
			}
			if j < len(runes) && isUpper(runes[j]) {
				return strings.TrimSpace(string(runes[:i+1]))
			}
		}
	}
	return s
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

// firstNonEmptyLine returns the first non-blank line of s, trimmed.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// directives extracts //notebook:key=value pairs from a cell's doc comment.
//
// A directive line looks like:
//
//	//notebook:slider min=0 max=5000 step=50
//
// The leading token after the prefix ("slider") is recorded as a key with an
// empty value — it names the presentation kind — and each subsequent "k=v"
// token becomes its own entry. Returns nil if there are no directives, so an
// undecorated cell carries no empty map.
func directives(fn *ast.FuncDecl) map[string]string {
	if fn.Doc == nil {
		return nil
	}
	var out map[string]string
	set := func(k, v string) {
		if out == nil {
			out = make(map[string]string)
		}
		out[k] = v
	}
	for _, c := range fn.Doc.List {
		text := c.Text
		if !strings.HasPrefix(text, directivePrefix) {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(text, directivePrefix))
		for _, field := range strings.Fields(body) {
			if k, v, ok := strings.Cut(field, "="); ok {
				if k == "" {
					continue // a malformed "=value" has no key to bind; skip it
				}
				set(k, v)
			} else {
				set(field, "") // a bare token names the kind, e.g. "slider"
			}
		}
	}
	return out
}

// layoutPrefix marks a package-level presentation-layout row.
const layoutPrefix = "//notebook:layout"

// parseLayout reads the notebook's presentation arrangement from the file's
// package doc comment. Each `//notebook:layout` line is one ROW; within a row,
// `|` separates columns, and each column token names an area=<name> group (or,
// failing that, a single cell). Returns nil when there is no layout, so an
// unarranged notebook renders in source order (degrade-to-linear).
//
// This is a SEPARATE parser from [directives] — and deliberately so. directives
// flattens per-function `k=v` tokens into a map; a layout row is an ORDERED list
// of columns that a map cannot hold, and it lives on the package doc, not a
// function. The one thing they share is the //notebook: register.
//
// Why per-row directive lines rather than one indented block: gofmt (which the
// project enforces) treats an indented, non-directive comment as prose and
// reflows it — reordering the lines and stripping indentation, which would
// destroy an ASCII-art block. A `//notebook:layout …` line is a directive
// (no space after //), so gofmt preserves it verbatim.
func parseLayout(file *ast.File) [][]string {
	// Read from the file's leading comment groups, NOT file.Doc: a blank line
	// between the layout lines and the `package` clause detaches file.Doc (Go
	// only attaches a doc comment immediately adjacent to `package`), yet the
	// comments still live in file.Comments. Scan every comment group that appears
	// BEFORE the package keyword — that is the package-level region where a layout
	// directive belongs, and it excludes per-cell doc comments below.
	var rows [][]string
	for _, cg := range file.Comments {
		if cg.Pos() >= file.Package {
			break // past the package clause — into cell bodies
		}
		for _, c := range cg.List {
			text := strings.TrimSpace(c.Text)
			if !strings.HasPrefix(text, layoutPrefix) {
				continue
			}
			body := strings.TrimSpace(strings.TrimPrefix(text, layoutPrefix))
			if body == "" {
				continue // a bare "//notebook:layout" with no row content is ignored
			}
			var cols []string
			for _, col := range strings.Split(body, "|") {
				if tok := strings.TrimSpace(col); tok != "" {
					cols = append(cols, tok)
				}
			}
			if len(cols) > 0 {
				rows = append(rows, cols)
			}
		}
	}
	return rows
}
