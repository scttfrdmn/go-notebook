//go:notebook
//
// wrap-existing-package — you don't rewrite your code to get a notebook, you
// wrap it. A thin reactive view over a mature Go API.
//
// The pitch for a team that already has working Go: a notebook is not a
// rewrite. Point cells at the functions you already have, and the graph makes
// them reactive — drag an input, everything downstream recomputes. The cells add
// no logic of their own; they are a few-line adapter that surfaces an existing
// package's API.
//
// To make that concrete with no invented domain, the "existing package" here is
// the standard library's `regexp` — a mature, battle-tested API this notebook
// imports and does not reimplement. Every compute cell below is a direct call
// into it:
//
//   - compile  → regexp.Compile        (the one cell that can fail — a bad
//                                        pattern is a value error, not a panic)
//   - matches  → re.FindAllStringIndex  (where each match sits in the text)
//   - groups   → re.FindStringSubmatch  (the first match's capture groups)
//   - summary  → re.NumSubexp           (how many capture groups the pattern has)
//
// There is no regex engine in this file, and no parsing — regexp does all of it.
// The notebook contributes only the wiring (name+type edges) and the views. Swap
// `regexp` for your own package and the shape is identical: leaf cells for the
// inputs, one derived cell per function you want to surface, a Render for output.
//
//	go tool notebook run ./examples/minimal/wrap-existing-package     # interactive, in a browser
//	go tool notebook check ./examples/minimal/wrap-existing-package   # print the graph
//
// Demonstrates: wrapping an existing (stdlib) package as a reactive notebook —
// cells that call the real API, a (value, error) compile cell, no reimplementation.
//
//notebook:layout intro
//notebook:layout pattern | subject
//notebook:layout matches
//notebook:layout groups | summary

package wrapexistingpackage

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb"
)

// The pattern to search for. A plain string leaf → a text box. Edit it and the
// whole graph recompiles and re-searches. The default finds ISO dates with three
// capture groups (year, month, day).
func pattern() (expr string) { return `(\d{4})-(\d{2})-(\d{2})` }

// The text to search. Another string leaf.
func subject() (text string) {
	return "Releases: v1 on 2024-01-15, v2 on 2024-06-30, then v3 on 2025-02-10."
}

// compile is the one cell that can fail: it hands the pattern straight to
// regexp.Compile. An invalid pattern (say an unclosed group) returns a non-nil
// error, so this cell is marked failed and matches/groups/summary show
// "blocked upstream" — regexp's own error, surfaced as a graph value, never a
// panic. The compiled *regexp.Regexp is the edge every downstream cell consumes.
func compile(expr string) (re *regexp.Regexp, err error) {
	return regexp.Compile(expr)
}

// matches asks the compiled regexp where every match sits in the text. The cell
// body is one call into the wrapped package — re.FindAllStringIndex — and the
// resulting byte-offset spans are the edge. Highlight (below) turns them into
// marked-up HTML; the matching itself is entirely regexp's.
func matches(re *regexp.Regexp, text string) (marked Highlight) {
	return Highlight{Text: text, Spans: re.FindAllStringIndex(text, -1)}
}

// groups surfaces the capture groups of the FIRST match, via
// re.FindStringSubmatch. Element 0 is the whole match; the rest are the
// parenthesised groups. SubexpNames() lines them up with any (?P<name>…) labels.
func groups(re *regexp.Regexp, text string) (caps Captures) {
	return Captures{Names: re.SubexpNames(), Values: re.FindStringSubmatch(text)}
}

// summary is the headline card: how many matches, and how many capture groups
// the pattern declares (re.NumSubexp — a third wrapped call). strconv, not fmt,
// keeps this cell body on the WASM-able path.
func summary(re *regexp.Regexp, marked Highlight) (readout Readout) {
	n := len(marked.Spans)
	return Readout{
		Label: "matches · capture groups",
		Value: strconv.Itoa(n) + " · " + strconv.Itoa(re.NumSubexp()),
	}
}

// intro is the prose cell.
func intro() (md Markdown) {
	return "**Wrapping, not rewriting.** Every compute cell here is a direct " +
		"call into the standard library's `regexp` — `compile`, `matches`, " +
		"`groups`, `summary` each call one function and return its result. " +
		"The notebook adds no logic; it only wires the calls together and draws " +
		"the output.\n\n" +
		"Edit **pattern** or **subject** and the graph recompiles and re-searches. " +
		"Type an invalid pattern (drop a `)`) to watch `compile` fail and the rest " +
		"block upstream — `regexp`'s own error, shown as a value in the graph."
}

// ---------------------------------------------------------------------------
// View types (local to this file — the notebook owns its presentation)
// ---------------------------------------------------------------------------

// Highlight renders the text with every regexp match wrapped in <mark>. Spans is
// exactly what re.FindAllStringIndex returned (pairs of byte offsets); the Render
// interleaves matched and unmatched runs, escaping both so the subject can't
// inject markup.
type Highlight struct {
	Text  string
	Spans [][]int
}

func (h Highlight) Render() nb.Rendered {
	var b strings.Builder
	b.WriteString(`<div style="font:15px/1.7 ui-monospace,SFMono-Regular,Menlo,monospace;` +
		`padding:1rem;border:1px solid #e7ebf0;border-radius:10px;white-space:pre-wrap;word-break:break-word">`)
	prev := 0
	for _, s := range h.Spans {
		if len(s) != 2 || s[0] < prev || s[1] > len(h.Text) {
			continue // defensive: ignore any malformed span rather than slice out of range
		}
		b.WriteString(html.EscapeString(h.Text[prev:s[0]]))
		b.WriteString(`<mark style="background:#fdf0c8;border-radius:3px;padding:0 2px">`)
		b.WriteString(html.EscapeString(h.Text[s[0]:s[1]]))
		b.WriteString(`</mark>`)
		prev = s[1]
	}
	b.WriteString(html.EscapeString(h.Text[prev:]))
	b.WriteString(`</div>`)
	return nb.HTML(b.String())
}

// Captures renders the first match's groups as a small table. Names/Values line
// up index-for-index (both come straight from the regexp); a blank name is shown
// as its index.
type Captures struct {
	Names  []string
	Values []string
}

func (c Captures) Render() nb.Rendered {
	if len(c.Values) == 0 {
		return nb.HTML(`<div style="font-family:sans-serif;color:#5b6472;padding:0.5rem">no match</div>`)
	}
	var b strings.Builder
	b.WriteString(`<table style="font-family:sans-serif;border-collapse:collapse">`)
	b.WriteString(`<tr style="color:#5b6472;font-size:12px;text-align:left">` +
		`<th style="padding:2px 12px 2px 0">group</th><th style="padding:2px 0">value</th></tr>`)
	for i, v := range c.Values {
		name := "0 (whole match)"
		if i > 0 {
			name = strconv.Itoa(i)
			if i < len(c.Names) && c.Names[i] != "" {
				name = c.Names[i]
			}
		}
		b.WriteString(`<tr>` +
			`<td style="padding:2px 12px 2px 0;color:#1b3a6b;font-variant-numeric:tabular-nums">` + html.EscapeString(name) + `</td>` +
			`<td style="padding:2px 0;font-family:ui-monospace,monospace">` + html.EscapeString(v) + `</td>` +
			`</tr>`)
	}
	b.WriteString(`</table>`)
	return nb.HTML(b.String())
}

// Readout is a single stat card, rendered as a label/value pair.
type Readout struct{ Label, Value string }

func (r Readout) Render() nb.Rendered {
	return nb.HTML(`<div style="font-family:system-ui,-apple-system,sans-serif">` +
		`<div style="font-size:12px;color:#5b6472">` + r.Label + `</div>` +
		`<div style="font:600 22px/1.2 system-ui,sans-serif;color:#1b3a6b;font-variant-numeric:tabular-nums">` + r.Value + `</div>` +
		`</div>`)
}

// Markdown is a prose cell: the engine converts it to a safe HTML subset.
type Markdown string

func (m Markdown) Render() nb.Rendered { return nb.Markdown(string(m)) }

// Compile-time checks that the view types really are renderable (a misspelled
// Render becomes a build error, not a silently-blank cell).
var (
	_ nb.Renderable = Highlight{}
	_ nb.Renderable = Captures{}
	_ nb.Renderable = Readout{}
	_ nb.Renderable = Markdown("")
)
