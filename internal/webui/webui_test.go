package webui

import (
	"os"
	"path/filepath"
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
