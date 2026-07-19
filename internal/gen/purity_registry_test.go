package gen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
)

// TestRegistryShipsRefinedPurity is the pipeline-level regression guard for the
// dead-cache finding: the generated registry must ship pure:true for a provably
// pure cell once the cold-path analysis has refined purity. Before the fix,
// RefinePurity had no non-test caller, so every built notebook shipped
// pure:false and the engine's cache (which keys on node.Pure()) never fired.
//
// This exercises the real production shape — LoadPackage → RefineGraphPurity →
// Registry — rather than a synthetic node with pure:true set by hand (the reason
// engine/cache_test.go could pass while the cache was inert end to end).
func TestRegistryShipsRefinedPurity(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "examples", "capacity")

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}
	if err := analyze.RefineGraphPurity(dir, res.Graph); err != nil {
		t.Fatalf("RefineGraphPurity: %v", err)
	}

	gf, err := Registry(res.Graph, res.Package)
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	src := string(gf.Content)

	// offeredLoad = lambda/mu, pure arithmetic. Its generated cell must be
	// pure:true, on its own record (id then pure on adjacent lines).
	if !registryCellIsPure(src, "offeredLoad") {
		t.Error("offeredLoad ships pure:false — the cache is inert for it (RefinePurity did not reach the registry)")
	}
	// Guard against a uniform-impure registry (the dead-cache symptom): at least
	// the pure arithmetic chain must be pure.
	if strings.Count(src, "pure: true") == 0 {
		t.Error("registry ships no pure:true cell — purity derivation did not reach codegen")
	}
}

// registryCellIsPure reports whether the generated cell literal with id `name`
// carries pure:true. The generated form is a multi-line struct literal:
//
//	{
//	  id:   "offeredLoad",
//	  ...
//	  pure: true,
//	  ...
//	}
//
// so we find the id line and scan the following lines up to the next id for its
// pure flag.
func registryCellIsPure(src, name string) bool {
	lines := strings.Split(src, "\n")
	inCell := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "id:") {
			inCell = strings.Contains(t, `"`+name+`"`)
			continue
		}
		if inCell && strings.HasPrefix(t, "pure:") {
			return strings.Contains(t, "true")
		}
	}
	return false
}
