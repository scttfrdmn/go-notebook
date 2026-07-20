package analyze

import (
	"path/filepath"
	"testing"
)

// TestRefinePurityInColdPath is the end-to-end pin for the cache's derivation:
// the production cold-path analysis (LoadPackage → RefineGraphPurity) must mark a
// provably-pure cell as Pure, so the generated registry ships pure:true and the
// engine cache can actually fire.
//
// This is the regression guard for the "cache is inert" finding: RefinePurity
// existed and was unit-tested, but no non-test caller ran it, so every built
// notebook shipped pure:false and nothing was ever cached. A test that hand-sets
// pure:true on a synthetic node (engine/cache_test.go) cannot catch that — only
// a test that runs the real analysis path can.
func TestRefinePurityInColdPath(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "capacity")
	res, err := LoadPackage(dir)
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}
	// Before refinement, everything defaults to impure (safe).
	if res.Graph.Cells["offeredLoad"].Pure {
		t.Fatal("precondition: offeredLoad should default to impure before refinement")
	}

	if err := RefineGraphPurity(dir, res.Graph); err != nil {
		t.Fatalf("RefineGraphPurity: %v", err)
	}

	// offeredLoad is lambda/mu — pure arithmetic, stdlib only. It MUST be pure, or
	// the cache never fires for it.
	if !res.Graph.Cells["offeredLoad"].Pure {
		t.Error("offeredLoad is pure arithmetic but was not marked Pure after cold-path refinement — the cache is inert")
	}
	// At least one derived cell being pure is the whole point; assert the graph
	// isn't uniformly impure (the dead-cache symptom).
	anyPure := false
	for _, c := range res.Graph.Cells {
		if c.Pure {
			anyPure = true
			break
		}
	}
	if !anyPure {
		t.Error("no cell is Pure after refinement — purity derivation did not run")
	}
}

// TestWiredEdgeImportsCollected is the analyzer half of the check≠build fix: a
// cell that takes a wired input whose type comes from an imported package must
// record that package in Graph.Imports, so codegen can import it in the registry.
// wrap-existing-package flows a *regexp.Regexp edge (produced by compile) into
// three cells, so "regexp" must be recorded — and NOTHING else, since its view
// types are local and no result type is a foreign package (results are assigned,
// not asserted, so they contribute no import even when imported).
func TestWiredEdgeImportsCollected(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "minimal", "wrap-existing-package")
	res, err := LoadPackage(dir)
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}
	if len(res.Graph.Imports) != 1 {
		t.Fatalf("expected exactly one wired-edge import, got %+v", res.Graph.Imports)
	}
	if got := res.Graph.Imports[0]; got.Path != "regexp" || got.Name != "regexp" {
		t.Errorf("wired-edge import = %+v, want {Path:regexp Name:regexp}", got)
	}
}

// TestContextParamRecordsNoImport pins the injected-vs-wired distinction: a
// context.Context parameter is supplied by the runtime and renders as ctx in the
// generated call — it is never type-asserted, so it must NOT enter Graph.Imports
// (which would emit a spurious, though harmless, "context" edge import). The
// cancel example takes a context param and no other imported-package edge, so its
// Imports must be empty.
func TestContextParamRecordsNoImport(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "minimal", "cancel")
	res, err := LoadPackage(dir)
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}
	if len(res.Graph.Imports) != 0 {
		t.Errorf("cancel has only a context param (injected, not wired) — Imports should be empty, got %+v", res.Graph.Imports)
	}
}
