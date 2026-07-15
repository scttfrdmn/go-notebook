package webui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSharedClientContract pins the "one thing, two transports" structure: the
// shared JS exposes the NB.{init,render,seedLeaves} surface both clients depend
// on, and the CSS carries the graph + state-rail rules. If someone renames or
// drops one, the transports would silently diverge — this fails first.
func TestSharedClientContract(t *testing.T) {
	for _, want := range []string{
		"return { init, render, seedLeaves, showProvenance }", // the surface the transports call
		"function buildGraph", // the dependency-graph view
		"function render",     // the event renderer
		"const STATES = ['running', 'done', 'error', 'blocked', 'stale']",
	} {
		if !strings.Contains(JS, want) {
			t.Errorf("shared JS missing %q — a transport depends on it", want)
		}
	}
	for _, want := range []string{".graph .node", ".cell.running", ".cell pre.src"} {
		if !strings.Contains(CSS, want) {
			t.Errorf("shared CSS missing %q", want)
		}
	}
}

// TestBothTransportsUseSharedClient is the anti-drift guard: both the SSE server
// page and the WASM host page must build on the shared client (NB.init +
// NB.render), not re-implement rendering. If a client stops importing webui,
// this test — which reads the two source files — catches it.
func TestBothTransportsUseSharedClient(t *testing.T) {
	root := moduleRoot(t)
	cases := []struct {
		name, path string
	}{
		{"SSE server", filepath.Join(root, "engine", "server", "ui.go")},
		{"WASM host", filepath.Join(root, "cmd", "notebook", "wasm_ui.go")},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		s := string(src)
		if !strings.Contains(s, "webui.Page") {
			t.Errorf("%s (%s) must assemble its page via webui.Page, not hand-roll one", c.name, c.path)
		}
		if !strings.Contains(s, "NB.init") || !strings.Contains(s, "NB.render") {
			t.Errorf("%s must drive the shared client via NB.init/NB.render", c.name)
		}
	}
}

// TestEveryWidgetKindHasAControl is the test that would have caught the
// [object Object] bug (#113): a widget Kind the analyzer emits but buildControl
// has no branch for falls through to the scalar text rung and renders a struct
// value as the literal string "[object Object]". That bug shipped on curvefit
// undetected for the project's whole life, because no Go test could see the client
// JS — until this one, which reads both sides as text and asserts they agree.
//
// The client JS is not executable from Go without a DOM, so this checks the
// CONTRACT, not the behavior: every Kind the analyzer produces must have a
// `kind === '<kind>'` branch in buildControl. Cheap, no dependency, and it fails
// the moment a new widget kind is added on one side but not the other.
func TestEveryWidgetKindHasAControl(t *testing.T) {
	root := moduleRoot(t)

	// Kinds the analyzer emits, from internal/analyze/leaf.go: `Kind: "<name>"`.
	leaf, err := os.ReadFile(filepath.Join(root, "internal", "analyze", "leaf.go"))
	if err != nil {
		t.Fatal(err)
	}
	emitted := setOf(regexp.MustCompile(`Kind:\s*"([a-z]+)"`).FindAllStringSubmatch(string(leaf), -1))
	if len(emitted) == 0 {
		t.Fatal("found no Kind values in leaf.go — regex or source changed")
	}

	// Kinds the client dispatches on, from webui.JS: `kind === '<name>'`.
	handled := setOf(regexp.MustCompile(`kind === '([a-z]+)'`).FindAllStringSubmatch(JS, -1))

	// A leaf Kind is either matched by name in buildControl, or it is the scalar
	// default (an empty Kind, "" — a slider/text box). Every non-empty emitted Kind
	// MUST have an explicit branch, or it silently renders as the scalar default.
	for k := range emitted {
		if k == "" {
			continue // the scalar default rung, intentionally unbranched
		}
		if !handled[k] {
			t.Errorf("widget Kind %q is emitted by the analyzer but buildControl has no `kind === '%s'` branch — "+
				"it will fall through to the scalar rung and render as [object Object]", k, k)
		}
	}
}

// TestRowDirectiveIsHonored is the test that would have caught the row= gap (#121):
// the //notebook:row= directive laid cells side by side in prose (bayes/seam/lotka)
// but the client ignored it and stacked them, for the project's whole life. The fix
// (#122) groups same-row cells into a .cellrow flex container. This pins that the
// client actually reads the row directive and builds the grouping — if someone
// removes it, the directive goes silently inert again.
func TestRowDirectiveIsHonored(t *testing.T) {
	for _, want := range []string{
		"Directives.row", // the client reads the row directive off CellMeta
		"cellrow",        // and groups matching cells into a flex row container
	} {
		if !strings.Contains(JS, want) {
			t.Errorf("shared JS missing %q — the //notebook:row= directive would be inert (regression of #121)", want)
		}
	}
	if !strings.Contains(CSS, ".cellrow") {
		t.Error("shared CSS missing .cellrow — row=grouped cells would not lay out side by side")
	}
}

// setOf collects submatch group 1 from a regexp FindAllStringSubmatch result.
func setOf(matches [][]string) map[string]bool {
	out := map[string]bool{}
	for _, m := range matches {
		if len(m) > 1 {
			out[m[1]] = true
		}
	}
	return out
}

// moduleRoot walks up to the go.mod so the test can read sibling packages'
// source without hardcoding a relative depth.
func moduleRoot(t *testing.T) string {
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
			t.Fatal("no go.mod found")
		}
		dir = parent
	}
}
