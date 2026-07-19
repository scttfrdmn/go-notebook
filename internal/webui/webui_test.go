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

// TestAreaDirectiveIsHonored is the test that would have caught the original
// grouping gap (#121): a grouping directive laid cells side by side in prose
// (bayes/seam/lotka) but the client ignored it and stacked them. The directive
// is now //notebook:area= (it replaced //notebook:row= when composition landed);
// with no layout block, consecutive same-area cells group into a .cellrow flex
// container. This pins that the client actually reads the area directive and
// builds the grouping — if someone removes it, the directive goes silently inert.
func TestAreaDirectiveIsHonored(t *testing.T) {
	for _, want := range []string{
		"Directives.area", // the client reads the area directive off CellMeta
		"cellrow",         // and groups matching cells into a flex row container
	} {
		if !strings.Contains(JS, want) {
			t.Errorf("shared JS missing %q — the //notebook:area= directive would be inert (regression of #121)", want)
		}
	}
	if !strings.Contains(CSS, ".cellrow") {
		t.Error("shared CSS missing .cellrow — area-grouped cells would not lay out side by side")
	}
}

// TestGraphPlacement pins the showcase/working split: a showcase page leads with
// the graph open and BEFORE the controls (the demos' pitch is watching the wave);
// a working page collapses it and puts it AFTER the cells (results first, the
// graph a tool you open). Both carry the legend, since the graph's colors and
// directional edges mean nothing without a key. If the placement logic regresses,
// every notebook page silently reverts to graph-first-always.
func TestGraphPlacement(t *testing.T) {
	showcase := Page(PageOpts{Title: "t", GraphShowcase: true})
	working := Page(PageOpts{Title: "t", GraphShowcase: false})

	// Showcase: the graph disclosure is open and sits before the controls.
	if !strings.Contains(showcase, `class="graphwrap" open>`) {
		t.Error("showcase page: graph disclosure should be open")
	}
	// Anchor on the element markup (class="graphwrap"), not the bare word, which
	// also appears in the .graphwrap CSS in the <head>.
	if idx(showcase, `class="graphwrap"`) > idx(showcase, `id="controls"`) {
		t.Error("showcase page: graph should come BEFORE the controls")
	}
	// Working: the graph disclosure is collapsed and sits after the cells.
	if strings.Contains(working, `class="graphwrap" open>`) {
		t.Error("working page: graph disclosure should be collapsed, not open")
	}
	if idx(working, `class="graphwrap"`) < idx(working, `id="cells"`) {
		t.Error("working page: graph should come AFTER the cells")
	}
	// Both: the legend explains the state colors + edge direction where the graph
	// is shown (the only place they are explained).
	for _, page := range []string{showcase, working} {
		for _, want := range []string{"legend", "recomputing", "feeds into"} {
			if !strings.Contains(page, want) {
				t.Errorf("page missing graph legend token %q", want)
			}
		}
	}
	// The arrowhead marker makes edges directional, not bare curves.
	if !strings.Contains(JS, "marker-end") || !strings.Contains(JS, "id', 'arrow'") {
		t.Error("shared JS missing the edge arrowhead marker — edges would be non-directional")
	}
}

// idx returns the index of sub in s, or a large sentinel if absent, so ordering
// comparisons in TestGraphPlacement read naturally.
func idx(s, sub string) int {
	if i := strings.Index(s, sub); i >= 0 {
		return i
	}
	return 1 << 30
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
