package engine

import "testing"

func TestRenderMarkdownBlocks(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"h1", "# Title", "<h1>Title</h1>\n"},
		{"h2", "## Sub", "<h2>Sub</h2>\n"},
		{"h3", "### Small", "<h3>Small</h3>\n"},
		{"too-deep heading degrades to text", "#### Deep", "<p>#### Deep</p>\n"},
		{"paragraph", "hello world", "<p>hello world</p>\n"},
		{"two lines join into one paragraph", "line one\nline two", "<p>line one line two</p>\n"},
		{"blank line splits paragraphs", "one\n\ntwo", "<p>one</p>\n<p>two</p>\n"},
		{"dash list", "- a\n- b", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"star list", "* a\n* b", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderMarkdown(c.in); got != c.want {
				t.Errorf("renderMarkdown(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

func TestRenderMarkdownInline(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bold", "a **b** c", "<p>a <strong>b</strong> c</p>\n"},
		{"italic", "a *b* c", "<p>a <em>b</em> c</p>\n"},
		{"code", "use `x` here", "<p>use <code>x</code> here</p>\n"},
		{"link", "see [docs](https://go.dev)", `<p>see <a href="https://go.dev">docs</a></p>` + "\n"},
		{"anchor link", "[top](#top)", `<p><a href="#top">top</a></p>` + "\n"},
		{"code is not parsed inside", "`**not bold**`", "<p><code>**not bold**</code></p>\n"},
		// Multi-byte runes must survive byte-wise escaping intact (an em-dash is 3
		// bytes; a naive string(byte) loop would mangle it into replacement chars).
		{"em dash and unicode", "scrub — it works · café", "<p>scrub — it works · café</p>\n"},
		{"unicode inside bold", "**café —**", "<p><strong>café —</strong></p>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renderMarkdown(c.in); got != c.want {
				t.Errorf("renderMarkdown(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

// TestRenderMarkdownSafety is the load-bearing test: the reason the client refused
// to inject text/markdown was XSS, and this renderer now produces the HTML that
// WILL be injected. The invariant is that no executable markup reaches the output —
// so we assert on the dangerous SHAPES (a live tag, a script-bearing href), not on
// whether a substring appears anywhere: a rejected javascript: URL correctly shows
// up as escaped plain text, and that literal text is harmless.
func TestRenderMarkdownSafety(t *testing.T) {
	cases := []struct {
		name, in       string
		mustNotContain []string
	}{
		{"script tag in text", "hello <script>alert(1)</script>", []string{"<script>", "</script>"}},
		{"img onerror", `<img src=x onerror=alert(1)>`, []string{"<img"}},
		{"javascript: url is not linked", "[x](javascript:alert(1))", []string{`href="javascript:`, "<a "}},
		{"data: url is not linked", "[x](data:text/html,foo)", []string{`href="data:`, "<a "}},
		{"html in link text is escaped", "[<b>x</b>](https://ok.com)", []string{"<b>", "</b>"}},
		{"angle brackets in code escaped", "`<script>`", []string{"<script>"}},
		{"quote cannot break out of href", `[x](https://a" onmouseover="alert(1))`, []string{`onmouseover="`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderMarkdown(c.in)
			for _, bad := range c.mustNotContain {
				if contains(got, bad) {
					t.Errorf("renderMarkdown(%q) = %q\n MUST NOT contain %q (XSS)", c.in, got, bad)
				}
			}
		})
	}
}

// TestAsRenderedConvertsMarkdown confirms the wiring: a value whose Render() emits
// text/markdown comes back through AsRendered as text/html, so the whole pipeline
// (schedule → event → client) delivers formatted prose.
func TestAsRenderedConvertsMarkdown(t *testing.T) {
	r, ok := AsRendered(mdValue("# Hi\n\nsome **text**."))
	if !ok {
		t.Fatal("markdown value was not renderable")
	}
	if r.MIME != "text/html" {
		t.Errorf("MIME = %q, want text/html (markdown should be converted)", r.MIME)
	}
	if !contains(r.Data, "<h1>Hi</h1>") || !contains(r.Data, "<strong>text</strong>") {
		t.Errorf("converted HTML missing expected tags: %q", r.Data)
	}
}

// TestAsRenderedPassesNonMarkdown confirms svg/html/plain are untouched.
func TestAsRenderedPassesThrough(t *testing.T) {
	r, ok := AsRendered(svgValue(`<svg/>`))
	if !ok || r.MIME != "image/svg+xml" || r.Data != "<svg/>" {
		t.Errorf("svg passed through wrong: %+v ok=%v", r, ok)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// test render values with their own Rendered-shaped struct (as a notebook would).
type testRendered struct{ MIME, Data string }
type mdValue string
type svgValue string

func (m mdValue) Render() testRendered  { return testRendered{"text/markdown", string(m)} }
func (s svgValue) Render() testRendered { return testRendered{"image/svg+xml", string(s)} }
