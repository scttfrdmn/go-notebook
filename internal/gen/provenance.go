package gen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
)

// timeNow is indirected so a test can pin the build time if it ever needs a
// deterministic provenance; production uses the real clock.
var timeNow = time.Now

// ToolVersion is the go-notebook toolchain version stamped into provenance,
// set once by the CLI from its own build version (main.version). It is a
// package global rather than a Build parameter because it is a property of the
// running tool, not of any one notebook, and threading it through every codegen
// entry point (Build/BuildWASM/Registry) would add a parameter to each for a
// single process-wide value. Empty ("") for a plain `go test`/`go run` of codegen
// (no CLI to set it) or an un-versioned dev build, and then omitted from the record.
var ToolVersion string

// Provenance is what produced an artifact: the content identity of its source
// plus the git/build/tool context. It is computed at build time and emitted into
// the generated registry as engine.Provenance, so a frozen binary can say what it
// is. A path is not a handle; the source hash is the handle.
type Provenance struct {
	SourceHash string
	Commit     string
	Dirty      bool
	BuiltAt    string
	GoVersion  string
	// ToolVersion is the go-notebook toolchain that generated this artifact.
	// Codegen changes can change behavior for identical source, so the tool
	// version is part of "what produced this." Empty for an un-versioned dev
	// build of the tool.
	ToolVersion string
}

// computeProvenance derives provenance from the notebook package. The source
// hash is the identity and is always present; git info is best-effort (a
// notebook outside a repo is a normal case, not an error) and left empty when
// absent. builtAt is passed in rather than read from the clock so the caller
// controls determinism (tests pass a fixed time; the CLI passes time.Now).
func computeProvenance(info analyze.PackageInfo, builtAt time.Time) (Provenance, error) {
	hash, err := hashSources(info.GoFiles)
	if err != nil {
		return Provenance{}, err
	}
	commit, dirty := gitState(info.Dir)
	return Provenance{
		SourceHash:  hash,
		Commit:      commit,
		Dirty:       dirty,
		BuiltAt:     builtAt.UTC().Format(time.RFC3339),
		GoVersion:   runtime.Version(),
		ToolVersion: ToolVersion,
	}, nil
}

// hashSources content-hashes the notebook package's source files. The hash is
// over each file's base name and bytes, in sorted order, so it is stable and
// identifies the CONTENTS — change one character in any package .go file and the
// hash changes; move or rename the directory and it does not.
//
// SCOPE (deliberately named, not "everything"): this covers exactly the package's
// non-generated .go files — all of them, not just the marked notebook file, so a
// helper in a sibling .go changes the hash. It does NOT cover go:embed'd assets,
// go.mod/go.sum, imported module versions, or build tags. So SourceHash is the
// content identity of the PACKAGE SOURCE, not a full build-input identity — see
// docs/reference-provenance.md and issue #224 for the layering that would add
// those. Naming the scope honestly is the point: a narrow true claim beats a
// broad one the hash can't back.
func hashSources(goFiles []string) (string, error) {
	files := append([]string(nil), goFiles...)
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("hashing %s: %w", f, err)
		}
		base := f
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		// Writes to a hash.Hash never error; the length prefixes frame each file
		// so contents can't collide across boundaries.
		_, _ = fmt.Fprintf(h, "%d:%s\n%d:", len(base), base, len(b))
		_, _ = h.Write(b)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// gitState returns the short commit and whether the working tree is dirty, for
// the notebook's directory. Absence of git (no repo, git not installed) is not
// an error — it returns "", false, and provenance simply omits the git fields.
func gitState(dir string) (commit string, dirty bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", false // not a repo, or git absent — a normal case
	}
	commit = strings.TrimSpace(string(out))
	stout, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err == nil && len(strings.TrimSpace(string(stout))) > 0 {
		dirty = true
	}
	return commit, dirty
}

// provenanceLiteral renders a Provenance as an engine.Provenance composite
// literal for the generated registry.
func provenanceLiteral(p Provenance) string {
	return fmt.Sprintf("engine.Provenance{SourceHash: %q, Commit: %q, Dirty: %t, BuiltAt: %q, GoVersion: %q, ToolVersion: %q}",
		p.SourceHash, p.Commit, p.Dirty, p.BuiltAt, p.GoVersion, p.ToolVersion)
}
