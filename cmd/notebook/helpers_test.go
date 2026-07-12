package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/graph"
)

func TestResolveDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A file resolves to its parent directory.
	if got, err := resolveDir(file); err != nil || got != dir {
		t.Errorf("resolveDir(file) = %q, %v; want %q", got, err, dir)
	}
	// A directory resolves to itself.
	if got, err := resolveDir(dir); err != nil || got != dir {
		t.Errorf("resolveDir(dir) = %q, %v; want %q", got, err, dir)
	}
	// A missing path errors.
	if _, err := resolveDir(filepath.Join(dir, "nope")); err == nil {
		t.Error("resolveDir(missing) should error")
	}
}

func TestTargetDir(t *testing.T) {
	dir := t.TempDir()

	parse := func(args ...string) *flag.FlagSet {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		_ = fs.Parse(args)
		return fs
	}

	if got, code := targetDir(parse(dir), "t"); code != 0 || got != dir {
		t.Errorf("targetDir(dir) = %q, %d; want %q, 0", got, code, dir)
	}
	if _, code := targetDir(parse(), "t"); code != 2 {
		t.Errorf("targetDir(no args) code = %d; want 2", code)
	}
	if _, code := targetDir(parse(dir, "extra"), "t"); code != 2 {
		t.Errorf("targetDir(extra arg) code = %d; want 2", code)
	}
	if _, code := targetDir(parse(filepath.Join(dir, "missing")), "t"); code != 1 {
		t.Errorf("targetDir(bad path) code = %d; want 1", code)
	}
}

func TestReportDiagnostics(t *testing.T) {
	// No diagnostics → no errors.
	if reportDiagnostics(nil, "t") {
		t.Error("no diagnostics should not report errors")
	}
	// A notice alone does not block.
	notice := graph.Diagnostic{Severity: graph.Notice, Msg: "deferred"}
	if reportDiagnostics([]graph.Diagnostic{notice}, "t") {
		t.Error("a notice alone must not block")
	}
	// An error blocks.
	err := graph.Diagnostic{Severity: graph.Error, Msg: "boom"}
	if !reportDiagnostics([]graph.Diagnostic{notice, err}, "t") {
		t.Error("an error must block")
	}
}

func TestFindModuleRoot(t *testing.T) {
	root, err := findModuleRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("findModuleRoot returned %q with no go.mod: %v", root, err)
	}
	// A path with no go.mod above it errors.
	if _, err := findModuleRoot("/"); err == nil {
		t.Error("findModuleRoot(/) should error — no go.mod above root")
	}
}

func TestRunDispatch(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Errorf("run(no args) = %d; want 2 (usage)", code)
	}
	if code := run([]string{"bogus"}); code != 2 {
		t.Errorf("run(unknown) = %d; want 2", code)
	}
	if code := run([]string{"--help"}); code != 0 {
		t.Errorf("run(--help) = %d; want 0", code)
	}
}
