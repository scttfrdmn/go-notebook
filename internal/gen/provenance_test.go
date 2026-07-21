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
	h1, err := hashSources([]string{f}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Same content at a different path → same hash (path is not the identity).
	dir2 := t.TempDir()
	f2 := filepath.Join(dir2, "nb.go")
	_ = os.WriteFile(f2, []byte("package nb\n\nfunc x() (a int) { return 1 }\n"), 0o644)
	h2, _ := hashSources([]string{f2}, nil)
	if h1 != h2 {
		t.Errorf("same contents at a different path must hash the same: %s vs %s", h1, h2)
	}

	// One character changed → different hash.
	_ = os.WriteFile(f, []byte("package nb\n\nfunc x() (a int) { return 2 }\n"), 0o644)
	h3, _ := hashSources([]string{f}, nil)
	if h1 == h3 {
		t.Error("a one-character source change must change the hash")
	}
}

// TestSourceHashCoversEmbeds is the #224 gap this closes: a go:embed'd asset is
// part of what the built artifact computes over, so a change to it must change the
// source hash — otherwise a dataset baked into the binary could change results
// with the artifact's identity unmoved. Driven at the hash level: same .go, an
// embed file whose one byte changes → a different hash.
func TestSourceHashCoversEmbeds(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "nb.go")
	if err := os.WriteFile(goFile, []byte("package nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(data, []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withEmbed, err := hashSources([]string{goFile}, []string{data})
	if err != nil {
		t.Fatal(err)
	}
	// The same .go with NO embed hashes differently — the embed is part of identity.
	goOnly, _ := hashSources([]string{goFile}, nil)
	if withEmbed == goOnly {
		t.Error("hashing with an embed file must differ from hashing the .go alone")
	}

	// One byte of the embedded data changes → the hash changes.
	_ = os.WriteFile(data, []byte("a,b\n1,3\n"), 0o644)
	changed, _ := hashSources([]string{goFile}, []string{data})
	if withEmbed == changed {
		t.Error("a one-byte change to an embedded asset must change the source hash")
	}
}

// TestLoadPackageCapturesEmbeds confirms the codegen load actually surfaces the
// embedded files, driven on the real embedded-data example (which embeds
// sales.csv). Without this, hashSources would receive an empty embed set and the
// coverage above would be dead in practice.
func TestLoadPackageCapturesEmbeds(t *testing.T) {
	root := moduleRoot(t)
	res, err := analyze.LoadPackage(filepath.Join(root, "examples", "minimal", "embedded-data"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Package.EmbedFiles {
		if strings.HasSuffix(f, "sales.csv") {
			found = true
		}
	}
	if !found {
		t.Errorf("LoadPackage did not capture the go:embed sales.csv; EmbedFiles = %v", res.Package.EmbedFiles)
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
	wantHash, _ := hashSources(res.Package.GoFiles, res.Package.EmbedFiles)
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

// TestToolVersionInProvenance pins that the go-notebook tool version, when set,
// flows into the provenance record — codegen changes can change behavior for
// identical source, so the tool version is part of "what produced this" — and
// that a dev build (empty ToolVersion) omits it rather than stamping "".
func TestToolVersionInProvenance(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "nb.go")
	if err := os.WriteFile(f, []byte("package nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := analyze.PackageInfo{Dir: dir, GoFiles: []string{f}}

	// Empty (a dev build of the tool): the field is empty and the literal omits
	// nothing but a "" — the engine JSON tag (omitempty) drops it on the wire.
	ToolVersion = ""
	p, err := computeProvenance(info, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if p.ToolVersion != "" {
		t.Errorf("dev build should leave ToolVersion empty, got %q", p.ToolVersion)
	}

	// Set (a released tool): it flows into the record and the emitted literal.
	ToolVersion = "v9.9.9"
	defer func() { ToolVersion = "" }() // don't leak into other tests
	p, err = computeProvenance(info, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if p.ToolVersion != "v9.9.9" {
		t.Errorf("ToolVersion = %q, want v9.9.9", p.ToolVersion)
	}
	if lit := provenanceLiteral(p); !strings.Contains(lit, `ToolVersion: "v9.9.9"`) {
		t.Errorf("provenance literal missing the tool version, got: %s", lit)
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
