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

// Provenance is what produced an artifact: the content identity of its source
// plus the git/build context. It is computed at build time and emitted into the
// generated registry as engine.Provenance, so a frozen binary can say what it
// is. A path is not a handle; the source hash is the handle.
type Provenance struct {
	SourceHash string
	Commit     string
	Dirty      bool
	BuiltAt    string
	GoVersion  string
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
		SourceHash: hash,
		Commit:     commit,
		Dirty:      dirty,
		BuiltAt:    builtAt.UTC().Format(time.RFC3339),
		GoVersion:  runtime.Version(),
	}, nil
}

// hashSources content-hashes the notebook's source files. The hash is over each
// file's base name and bytes, in sorted order, so it is stable and identifies
// the CONTENTS — change one character in any file and the hash changes; move or
// rename the directory and it does not.
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
	return fmt.Sprintf("engine.Provenance{SourceHash: %q, Commit: %q, Dirty: %t, BuiltAt: %q, GoVersion: %q}",
		p.SourceHash, p.Commit, p.Dirty, p.BuiltAt, p.GoVersion)
}
