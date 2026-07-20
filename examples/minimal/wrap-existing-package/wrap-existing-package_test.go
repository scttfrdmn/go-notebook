package wrapexistingpackage

import (
	"strings"
	"testing"
)

// These tests are the recipe's anti-pass. The whole claim of the example is that
// each cell just CALLS the wrapped package (stdlib regexp) and returns its
// result — the notebook adds no logic. So each test drives a cell end to end and
// pins the value regexp produced. If the wrapping ever silently broke (a cell
// stopped calling the real function, or called the wrong one), the composed
// output would change and the matching test would go red. They pass only because
// the cells really do reach into regexp.

// TestCompileWrapsRegexpCompile confirms the compile cell hands the pattern to
// regexp.Compile: a valid pattern yields a usable *regexp.Regexp (the edge every
// downstream cell consumes), and an invalid one returns regexp's own error rather
// than a nil edge with no error — which is what makes the graph block downstream.
func TestCompileWrapsRegexpCompile(t *testing.T) {
	re, err := compile(`(\d{4})-(\d{2})-(\d{2})`)
	if err != nil || re == nil {
		t.Fatalf("compile(valid) = (%v, %v), want a non-nil regexp and no error", re, err)
	}
	if got := re.NumSubexp(); got != 3 {
		t.Errorf("compiled regexp has %d subexpressions, want 3 (is it the pattern we passed?)", got)
	}
	if _, err := compile(`(\d+`); err == nil {
		t.Error("compile(invalid) returned nil error — a bad pattern must surface regexp's error so downstream cells block")
	}
}

// TestMatchesWrapsFindAll pins the matches cell against re.FindAllStringIndex:
// the default pattern finds exactly the three ISO dates in the subject, at the
// byte offsets regexp reports. If matches stopped calling FindAllStringIndex (or
// the wire from compile broke), the span count would change and this fails.
func TestMatchesWrapsFindAll(t *testing.T) {
	re, _ := compile(pattern())
	marked := matches(re, subject())
	if len(marked.Spans) != 3 {
		t.Fatalf("matches found %d spans, want 3 dates (does the cell call re.FindAllStringIndex?)", len(marked.Spans))
	}
	// The first span must delimit the first date in the subject.
	first := marked.Spans[0]
	if got := subject()[first[0]:first[1]]; got != "2024-01-15" {
		t.Errorf("first match = %q, want %q", got, "2024-01-15")
	}
}

// TestGroupsWrapsFindStringSubmatch pins the groups cell against
// re.FindStringSubmatch: the first ISO date decomposes into the whole match plus
// its three capture groups (year, month, day). A broken wrapping — wrong call,
// wrong regexp — would not produce these exact four values.
func TestGroupsWrapsFindStringSubmatch(t *testing.T) {
	re, _ := compile(pattern())
	caps := groups(re, subject())
	want := []string{"2024-01-15", "2024", "01", "15"}
	if len(caps.Values) != len(want) {
		t.Fatalf("groups returned %d values, want %d (whole match + 3 groups from re.FindStringSubmatch)", len(caps.Values), len(want))
	}
	for i, w := range want {
		if caps.Values[i] != w {
			t.Errorf("group %d = %q, want %q", i, caps.Values[i], w)
		}
	}
}

// TestSummaryWrapsCounts guards the summary readout, which composes two wrapped
// facts: the match count (len of matches' spans) and re.NumSubexp. The exact
// "3 · 3" pins both — three dates found, three capture groups declared. Break
// either wrapped call and the string changes.
func TestSummaryWrapsCounts(t *testing.T) {
	re, _ := compile(pattern())
	marked := matches(re, subject())
	if got := summary(re, marked).Value; got != "3 · 3" {
		t.Fatalf("summary = %q, want %q (matches count · re.NumSubexp)", got, "3 · 3")
	}
}

// TestHighlightRenderMarksMatches confirms the OUT side: the Highlight view wraps
// each matched run in <mark>, and escapes the text (so a subject can't inject
// markup). It is the rendered consequence of the wrapped FindAllStringIndex.
func TestHighlightRenderMarksMatches(t *testing.T) {
	re, _ := compile(pattern())
	html := matches(re, subject()).Render().Data
	if n := strings.Count(html, "<mark"); n != 3 {
		t.Errorf("rendered highlight has %d <mark> spans, want 3 (one per match)", n)
	}
	if !strings.Contains(html, "2024-01-15") {
		t.Error("rendered highlight dropped the first matched date")
	}
}
