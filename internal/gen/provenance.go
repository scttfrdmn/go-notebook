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
	hash, err := hashSources(info.GoFiles, info.EmbedFiles)
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

// hashSources content-hashes the notebook package's source: its non-generated
// .go files AND its go:embed'd assets. The hash is over each file's base name and
// bytes, in sorted order within each kind, so it is stable and identifies the
// CONTENTS — change one character in any .go file, or one byte of an embedded
// dataset, and the hash changes; move or rename the directory and it does not.
//
// SCOPE (deliberately named, not "everything"): this covers all of the package's
// non-generated .go files (not just the marked notebook file, so a helper in a
// sibling .go changes the hash) and every file baked in with go:embed (so a
// dataset compiled into the binary is part of its identity — a change to it can
// change results, and now changes the hash). It does NOT cover go.mod/go.sum,
// imported module versions, or build tags. So SourceHash is the content identity
// of the PACKAGE SOURCE AND ITS EMBEDDED DATA, not a full build-input identity —
// see docs/reference-provenance.md and issue #224 for the module-graph and
// artifact-bytes layers still open. Naming the scope honestly is the point.
//
// go and embed files are hashed in separate framed groups so a contrived rename
// (a .go and an embed asset sharing a base name) can never collide across the two
// sets; within each group order is sorted for stability.
func hashSources(goFiles, embedFiles []string) (string, error) {
	h := sha256.New()
	for _, group := range [][]string{goFiles, embedFiles} {
		files := append([]string(nil), group...)
		sort.Strings(files)
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
		// A group separator so [{a},{b}] and [{a,b},{}] can't hash alike.
		_, _ = fmt.Fprint(h, "|")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// gitState returns the short commit and whether the working tree is dirty. The
// `git -C dir` only tells git where to FIND the repo (the notebook's directory);
// `status --porcelain` then reports the WHOLE repository, not just that
// subdirectory — so `dirty` is whole-tree, and a change to a shared helper in a
// sibling package correctly marks the build dirty. This matches Go's own
// `vcs.modified` (visible via `go version -m` on a native binary) by
// construction: same git, same whole-tree scope. Absence of git (no repo, git
// not installed) is not an error — it returns "", false, and provenance omits the
// git fields. Computed at codegen time, so it is uniform across native and wasm
// (unlike debug.ReadBuildInfo's VCS fields, which the wasm build does not carry).
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
