package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
)

// TestSourceHashIsContentIdentity is KC10's core: the hash identifies the
// CONTENTS, not the path. Change one character → the hash changes; move/rename
// the file → it does not. Driven with real files (§8 — assert the actual
// behavior, not that a hash exists).
func TestSourceHashIsContentIdentity(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nb.go")
	if err := os.WriteFile(f, []byte("package nb\n\nfunc x() (a int) { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := hashSources([]string{f})
	if err != nil {
		t.Fatal(err)
	}

	// Same content at a different path → same hash (path is not the identity).
	dir2 := t.TempDir()
	f2 := filepath.Join(dir2, "nb.go")
	_ = os.WriteFile(f2, []byte("package nb\n\nfunc x() (a int) { return 1 }\n"), 0o644)
	h2, _ := hashSources([]string{f2})
	if h1 != h2 {
		t.Errorf("same contents at a different path must hash the same: %s vs %s", h1, h2)
	}

	// One character changed → different hash.
	_ = os.WriteFile(f, []byte("package nb\n\nfunc x() (a int) { return 2 }\n"), 0o644)
	h3, _ := hashSources([]string{f})
	if h1 == h3 {
		t.Error("a one-character source change must change the hash")
	}
}

// TestRegistryEmitsProvenance confirms the registry carries a real provenance
// record: a non-empty source hash and the Go version, emitted as
// engine.Provenance for the transports to display.
func TestRegistryEmitsProvenance(t *testing.T) {
	root := moduleRoot(t)
	res, err := analyze.LoadPackage(filepath.Join(root, "examples", "capacity"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	src := string(reg.Content)
	if !strings.Contains(src, "var NotebookProvenance = engine.Provenance{SourceHash: \"") {
		t.Errorf("registry must emit NotebookProvenance with a source hash; got:\n%s", firstProvLine(src))
	}
	if !strings.Contains(src, "GoVersion: \"go") {
		t.Errorf("provenance should carry the Go version")
	}
	// The emitted hash must be the real one for these files, not a placeholder.
	wantHash, _ := hashSources(res.Package.GoFiles)
	if !strings.Contains(src, "SourceHash: \""+wantHash+"\"") {
		t.Errorf("emitted source hash does not match the files' content hash")
	}
}

// TestGitStateGracefulWithoutRepo confirms a notebook outside a git repo is a
// normal case: gitState returns empty, no error, no panic.
func TestGitStateGracefulWithoutRepo(t *testing.T) {
	dir := t.TempDir() // a fresh temp dir is not a git repo
	commit, dirty := gitState(dir)
	if commit != "" || dirty {
		t.Errorf("a non-repo dir should yield empty git state, got commit=%q dirty=%v", commit, dirty)
	}
	// And computeProvenance still succeeds, with the hash present and git omitted.
	f := filepath.Join(dir, "nb.go")
	_ = os.WriteFile(f, []byte("package nb\n"), 0o644)
	p, err := computeProvenance(analyze.PackageInfo{Dir: dir, GoFiles: []string{f}}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("computeProvenance outside a repo should not error: %v", err)
	}
	if p.SourceHash == "" {
		t.Error("source hash must be present even without a repo")
	}
	if p.Commit != "" {
		t.Error("commit should be empty outside a repo")
	}
}

func firstProvLine(src string) string {
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "NotebookProvenance") {
			return line
		}
	}
	return "(no NotebookProvenance line)"
}
