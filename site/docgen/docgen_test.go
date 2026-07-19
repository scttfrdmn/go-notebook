package main

import "testing"

// TestRewriteLinksScope pins the intra-doc-only contract of rewriteLinks: a bare
// sibling slug like design.md becomes design.html (so intra-doc links point at
// the generated pages), but an EXTERNAL URL that merely ends in .md must be left
// alone. Rewriting an external GitHub blob URL's .md to .html produced a real
// 404 on the live site (a docs re-review caught it); this is the regression
// guard so it cannot come back.
func TestRewriteLinksScope(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "intra-doc slug is rewritten",
			in:   `see <a href="design.md">the design</a>`,
			want: `see <a href="design.html">the design</a>`,
		},
		{
			name: "intra-doc slug with fragment is rewritten",
			in:   `<a href="paper.md#section-14">§14</a>`,
			want: `<a href="paper.html#section-14">§14</a>`,
		},
		{
			name: "external GitHub .md URL is preserved (the bug)",
			in:   `<a href="https://github.com/scttfrdmn/go-notebook/blob/main/docs/notebook-as-service.md">seam</a>`,
			want: `<a href="https://github.com/scttfrdmn/go-notebook/blob/main/docs/notebook-as-service.md">seam</a>`,
		},
		{
			name: "external .md URL with fragment is preserved",
			in:   `<a href="https://example.com/docs/x.md#anchor">x</a>`,
			want: `<a href="https://example.com/docs/x.md#anchor">x</a>`,
		},
		{
			name: "a relative path with a separator is not treated as a sibling slug",
			in:   `<a href="../other/x.md">x</a>`,
			want: `<a href="../other/x.md">x</a>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteLinks(tc.in); got != tc.want {
				t.Errorf("rewriteLinks(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
