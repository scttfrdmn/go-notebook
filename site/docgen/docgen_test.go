package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test dir")
		}
		dir = parent
	}
}

// countCorpus returns the number of corpus notebooks — directories directly
// under examples/ (excluding the minimal/ teaching set) that carry the
// //go:notebook marker. This is the paper's "44-notebook corpus" derived from the
// tree, so the assertion below cannot drift from what actually exists.
func countCorpus(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "examples"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "minimal" {
			continue
		}
		if dirHasNotebook(filepath.Join(root, "examples", e.Name())) {
			n++
		}
	}
	return n
}

// dirHasNotebook reports whether any .go file directly in dir begins with the
// //go:notebook marker (the same predicate the toolchain uses to recognize a
// notebook package).
func dirHasNotebook(dir string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err == nil && strings.HasPrefix(string(b), "//go:notebook") {
			return true
		}
	}
	return false
}

// buildShLive matches the corpus demo loop in site/build.sh — the notebooks built
// to WASM and served live. The count of names on that line is the paper's "38
// live". Deriving it from build.sh (the single source of what actually ships)
// means the number in the paper is checked against the deploy, not maintained by
// hand.
var buildShLive = regexp.MustCompile(`^for nb in (.+); do`)

// countLive returns how many corpus notebooks site/build.sh builds to WASM.
func countLive(t *testing.T, root string) int {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "site", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := buildShLive.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			return len(strings.Fields(m[1]))
		}
	}
	t.Fatal("no `for nb in … ; do` demo loop found in site/build.sh")
	return 0
}

// TestPaperCountsMatchTree is the skew guard the docs re-review asked for: the
// paper's corpus and live counts must equal what the tree and build.sh actually
// contain. The deployed paper once claimed 39/34 while the source had moved on to
// 44/38 — a hand-maintained number drifting from reality. This derives both
// counts from the source of truth and asserts the paper's prose agrees, so the
// number cannot silently rot. (It reads docs/paper.md directly — the generated
// HTML is a faithful rendering of it, and this keeps the test independent of a
// full docgen run.)
func TestPaperCountsMatchTree(t *testing.T) {
	root := repoRoot(t)
	corpus := countCorpus(t, root)
	live := countLive(t, root)

	paper, err := os.ReadFile(filepath.Join(root, "docs", "paper.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(paper)

	// The paper states the corpus count two ways ("corpus of N notebooks", "N-notebook
	// corpus") and the live count two ways ("N running live", "N live"). Each derived
	// number must appear at least once, in a corpus/live phrasing — so a stale number
	// (the drift the reviewer caught) fails the build.
	corpusPhrases := []string{
		fmt.Sprintf("corpus of %d notebooks", corpus),
		fmt.Sprintf("%d-notebook corpus", corpus),
	}
	if !containsAny(text, corpusPhrases) {
		t.Errorf("paper.md states no corpus count matching the tree (%d corpus notebooks under examples/, excluding minimal/).\n"+
			"Expected one of: %q", corpus, corpusPhrases)
	}
	livePhrases := []string{
		fmt.Sprintf("%d running live", live),
		fmt.Sprintf("%d live", live),
	}
	if !containsAny(text, livePhrases) {
		t.Errorf("paper.md states no live count matching site/build.sh (%d demos built to WASM).\n"+
			"Expected one of: %q", live, livePhrases)
	}
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// TestCheckTranscriptIsReal regenerates the `check` transcript embedded in the
// build-&-run reference from the actual command and asserts the doc matches. This
// is the guard that would have caught the [impure]/[pure] drift (#211): the doc
// showed output the tool never produced. Building the toolchain and running check
// is slow, so it is skipped in -short mode.
func TestCheckTranscriptIsReal(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the real `notebook check`; skipped in -short mode")
	}
	root := repoRoot(t)

	// The reference embeds the check output for examples/minimal/hello. Run the
	// real command the same way a reader would.
	cmd := exec.Command("go", "run", "./cmd/notebook", "check", "./examples/minimal/hello")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook check failed: %v\n%s", err, out)
	}
	realBlock := extractGraphBlock(string(out))
	if realBlock == "" {
		t.Fatalf("could not find a `graph:` block in check output:\n%s", out)
	}

	docBytes, err := os.ReadFile(filepath.Join(root, "docs", "reference-build-run.md"))
	if err != nil {
		t.Fatal(err)
	}
	docBlock := extractGraphBlock(string(docBytes))
	if docBlock == "" {
		t.Fatal("no `graph:` transcript block found in docs/reference-build-run.md")
	}

	if docBlock != realBlock {
		t.Errorf("the `check` transcript in reference-build-run.md does not match the real command output.\n"+
			"This is the drift #211 fixed — regenerate the transcript from `notebook check ./examples/minimal/hello`.\n\n--- doc ---\n%s\n--- real ---\n%s", docBlock, realBlock)
	}
}

// extractGraphBlock returns the lines from a "graph: N cells" line through the
// last non-blank line of that block (the graph listing), normalized: leading/
// trailing blank lines trimmed, trailing whitespace per line stripped. It pulls
// the transcript out of both the raw command output and the fenced code block in
// the markdown so the two are compared on equal footing.
func extractGraphBlock(s string) string {
	lines := strings.Split(s, "\n")
	start := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "graph:") && strings.Contains(ln, "cells") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	var block []string
	for _, ln := range lines[start:] {
		t := strings.TrimSpace(ln)
		// Stop at a markdown fence or a prose line that isn't part of the listing.
		if t == "```" {
			break
		}
		block = append(block, strings.TrimRight(ln, " \t"))
	}
	// Trim trailing blank lines.
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1]
	}
	return strings.Join(block, "\n")
}

// TestStamplessPagesDetectsSkew proves the build-stamp guard actually fails when
// a page lacks the stamp — the "verify the instrument against a known-good"
// discipline: a guard that can't go red guards nothing. It builds a fake output
// dir with the stamp on every page except one, and asserts stamplessPages
// reports exactly that page.
func TestStamplessPagesDetectsSkew(t *testing.T) {
	dir := t.TempDir()
	const stamp = "built 2026-07-20 · abc1234"
	stamped := "<footer>" + stamp + "</footer>"

	var odd string
	for i, slug := range outputSlugs() {
		body := stamped
		if i == 3 { // one page from an "older generation" — no stamp
			body = "<footer>built 2000-01-01 · deadbee</footer>"
			odd = slug + ".html"
		}
		if err := os.WriteFile(filepath.Join(dir, slug+".html"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	missing := stamplessPages(dir, stamp)
	if len(missing) != 1 || missing[0] != odd {
		t.Errorf("stamplessPages = %v, want exactly [%s] — the guard did not flag the stampless page", missing, odd)
	}

	// And the happy path: with the stamp on every page, nothing is flagged.
	if err := os.WriteFile(filepath.Join(dir, odd), []byte(stamped), 0o644); err != nil {
		t.Fatal(err)
	}
	if missing := stamplessPages(dir, stamp); len(missing) != 0 {
		t.Errorf("stamplessPages = %v, want none when every page carries the stamp", missing)
	}
}
