package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBatchHeadlessSetJSON is the end-to-end batch story: build the capacity
// notebook, run it headless with a --set override, and confirm --json emits the
// overridden result. This is the "same file is a notebook, a job, and a
// callable model" claim, exercised as a real subprocess.
func TestBatchHeadlessSetJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")

	// Build via our own build command (exercises the real pipeline).
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("notebook build returned %d", code)
	}

	// Run headless with an override; capacity: hourlyCost = c * price, price≈1.006.
	head := filepath.Join(tmp, "h.json")
	out, err := exec.Command(bin, "--headless", "--set", "c=120", "--head", head, "--json").Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	vals := jsonValues(t, out)
	if c, _ := vals["c"].(float64); c != 120 {
		t.Errorf("c = %v, want 120 (the --set override should flow)", vals["c"])
	}
	// cost = 120 * 1.006 = 120.72
	if cost, _ := vals["cost"].(float64); cost < 120 || cost > 121 {
		t.Errorf("cost = %v, want ~120.72 (override must flow downstream)", vals["cost"])
	}
}

// TestBatchJSONIsSelfIdentifying is KC12: a headless --json run names what
// produced it — the source hash and commit — alongside the values, so an sbatch
// result is reproducible without external bookkeeping. Driven on the real
// built binary (§8).
func TestBatchJSONIsSelfIdentifying(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}
	out, err := exec.Command(bin, "--headless", "--json", "--head", filepath.Join(tmp, "h.json")).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var doc struct {
		Provenance struct {
			SourceHash string `json:"sourceHash"`
			Commit     string `json:"commit"`
			GoVersion  string `json:"goVersion"`
		} `json:"provenance"`
		Values map[string]any `json:"values"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not the expected envelope: %v\n%s", err, out)
	}
	if doc.Provenance.SourceHash == "" {
		t.Error("--json must carry the source hash (the notebook's content identity)")
	}
	if doc.Provenance.GoVersion == "" {
		t.Error("--json provenance should carry the Go version")
	}
	if len(doc.Values) == 0 {
		t.Error("--json must still carry the cell values under \"values\"")
	}
}

// TestBatchUnstableQueueJSON confirms --json survives a non-marshalable value:
// an unstable queue produces an infinite wait (math.Inf), which JSON can't
// represent; the output must still be valid JSON with the value stringified.
func TestBatchUnstableQueueJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}
	// lambda=2000, mu=20 → offered load 100 > servers 80 → unstable → wq = +Inf.
	head := filepath.Join(tmp, "h.json")
	out, err := exec.Command(bin, "--headless", "--set", "lambda=2000", "--head", head, "--json").Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON (Inf should be stringified):\n%s", out)
	}
	if !strings.Contains(string(out), "Inf") {
		t.Errorf("expected an Inf value stringified in output:\n%s", out)
	}
}

// TestLeafPropertyEndToEnd is the property that would have caught the inert-
// slider bug on the REAL notebook: for each control leaf, --set it away from
// its default and assert the batch JSON output differs from the unset run.
// Driving the built binary (not the engine directly) is the point — it
// exercises the full leaf-identity + coercion + override path a slider uses.
func TestLeafPropertyEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	run := func(sets ...string) map[string]any {
		args := []string{"--headless", "--json", "--head", filepath.Join(tmp, "h"+strings.Join(sets, "_")+".json")}
		for _, s := range sets {
			args = append(args, "--set", s)
		}
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			t.Fatalf("run %v: %v", sets, err)
		}
		return jsonValues(t, out)
	}

	base := run() // defaults

	// Each capacity leaf (by symbol) and a value clearly different from default.
	edits := map[string]string{
		"c":      "200",  // servers
		"lambda": "3000", // arrivalRate
		"mu":     "50",   // serviceRate
		"price":  "3.5",  // hourlyPrice
	}
	for leaf, val := range edits {
		got := run(leaf + "=" + val)
		if sameAllValues(base, got) {
			t.Errorf("leaf %q set to %s changed NO downstream value — inert control", leaf, val)
		}
	}
}

// TestBatchSetCompositeLeaf is the rank-3 unlock: --set now routes through the
// same CoerceWire path as the browser, so a COMPOSITE leaf (a Multi[Theme]'s
// selection, a JSON array) can be set from the CLI. The old string-only parser
// stored the raw string for such a leaf and was silently inert. Verified by
// consequence: restricting lego's themes to ["City"] cuts the downstream row set.
func TestBatchSetCompositeLeaf(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "legonb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "lego")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	rowsLen := func(sets ...string) int {
		args := []string{"--headless", "--json", "--head", filepath.Join(tmp, "h"+strings.Join(sets, "_")+".json")}
		for _, s := range sets {
			args = append(args, "--set", s)
		}
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			t.Fatalf("run %v: %v", sets, err)
		}
		rows, _ := jsonValues(t, out)["rows"].([]any)
		return len(rows)
	}

	base := rowsLen()
	city := rowsLen(`themes=["City"]`)
	if city == 0 {
		t.Fatal("setting the Multi[Theme] leaf produced no rows — the composite --set did not flow")
	}
	if city >= base {
		t.Errorf("themes=[\"City\"] rows=%d not fewer than unfiltered rows=%d — the composite leaf was inert", city, base)
	}
}

// TestBatchSetFailsLoud is the anti-pass: a --set that cannot be applied must
// exit non-zero, never store the wrong value and continue. This closes the two
// silent-failure modes the old path had — an unknown leaf (a no-op) and a scalar
// leaf handed the wrong kind (a dropped set). A loud failure is the whole point
// of routing --set through the one coercer.
func TestBatchSetFailsLoud(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	cases := []struct{ name, set, wantMsg string }{
		{"unknown leaf", "nonexistent=5", "unknown leaf"},
		{"kind mismatch", "c=not_a_number", "want a number"},
		{"uncoercible null", "c=null", "cannot coerce"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := filepath.Join(tmp, "h_"+tc.name+".json")
			out, err := exec.Command(bin, "--headless", "--json", "--head", head, "--set", tc.set).CombinedOutput()
			if err == nil {
				t.Fatalf("--set %q should fail loud, but exited 0:\n%s", tc.set, out)
			}
			if !strings.Contains(string(out), tc.wantMsg) {
				t.Errorf("--set %q: want error mentioning %q, got:\n%s", tc.set, tc.wantMsg, out)
			}
		})
	}
}

// TestBatchHandleLeafDeliversNewData is KC17 end-to-end: a content-addressed
// handle crosses into a BUILT notebook via --set, with no rebuild, and every
// downstream aggregate recomputes over the new dataset. taxi's trips leaf is a
// Rel[Trip] handle (settable because Rel carries Reconcile); pointing it at a
// different source changes the whole graph. Driving the built binary (not the
// engine) is the point — it exercises the real coerce + Reconcile + Open + Scan
// path a host's set(dataLeaf, handle) would.
func TestBatchHandleLeafDeliversNewData(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "taxinb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "taxi")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	run := func(sets ...string) map[string]any {
		args := []string{"--headless", "--json", "--head", filepath.Join(tmp, "h"+strings.Join(sets, "_")+".json")}
		for _, s := range sets {
			args = append(args, "--set", s)
		}
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			t.Fatalf("run %v: %v", sets, err)
		}
		return jsonValues(t, out)
	}

	base := run()                                    // default day dataset
	night := run(`all={"Source":"trips-night.csv"}`) // hand it a new handle

	// The SAME built binary, no rebuild: the handle's row count changed, so the
	// dataset genuinely changed — Scan read the source the new handle names.
	if fmt.Sprint(base["rows"]) == fmt.Sprint(night["rows"]) {
		t.Errorf("scale rows unchanged (%v) after setting a new handle — the dataset did not cross", base["rows"])
	}
	// And a downstream aggregate recomputed (not just the leaf): the worst hour
	// shifts from the day rush to the late-night shift.
	if fmt.Sprint(base["bad"]) == fmt.Sprint(night["bad"]) {
		t.Error("the saturated-hours readout did not recompute over the new dataset")
	}
}

// TestBatchIgnoresStrayHeadInCWD is the anti-pass for #208: a headless --json
// run must be a pure function of (source, --set). It must NOT silently merge a
// notebook-head.json that a prior interactive or --set run left in the working
// directory — same binary, same flags, different CWD state has to give the same
// output, or the reproducibility claim printJSON makes ("the exact source/commit
// that produced it") has no teeth. Driven on the real built binary in two
// controlled working directories: the generated main is a template string that
// go test never compiles (a build-tag-shaped blind spot, the #91 lesson), so
// only a built binary exercises the head-selection path this guards.
func TestBatchIgnoresStrayHeadInCWD(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "capnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	// Run headless --json in a given working directory with NO --head, so the
	// binary falls back to its default head path (notebook-head.json, resolved
	// relative to the CWD) — the exact path a stray file would occupy.
	runIn := func(dir string) map[string]any {
		cmd := exec.Command(bin, "--headless", "--json")
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("run in %s: %v", dir, err)
		}
		return jsonValues(t, out)
	}

	// A clean directory: the batch result is the source's own defaults (c=80).
	cleanDir := filepath.Join(tmp, "clean")
	if err := os.Mkdir(cleanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	clean := runIn(cleanDir)
	if got := fmt.Sprint(clean["c"]); got != "80" {
		t.Fatalf("clean batch c=%v, want 80 (the source default) — test premise broken", got)
	}

	// A directory a prior interactive/--set run polluted with a notebook-head.json
	// overriding a leaf (c=200, vs the source default 80). Writing the file
	// directly models the leftover artifact faithfully, independent of how the
	// running binary happens to persist.
	dirtyDir := filepath.Join(tmp, "dirty")
	if err := os.Mkdir(dirtyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirtyDir, "notebook-head.json"), []byte(`{"c":200}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty := runIn(dirtyDir)

	// The whole claim: a no---set batch run reports the SOURCE default, never the
	// stray head's override. Equal to the clean run, and concretely 80 (not 200).
	if got := fmt.Sprint(dirty["c"]); got != "80" {
		t.Errorf("batch --json merged a stray notebook-head.json: c=%v, want 80 "+
			"(the source default, not the leftover 200) — batch output is not a "+
			"pure function of (source, --set)", got)
	}

	// The explicit --head opt-in must still apply a named head in batch: that is
	// the deliberate, reproducible-by-path mode (a launcher that WANTS a snapshot
	// passes it explicitly). This pins the fix doesn't over-correct by ignoring
	// head entirely — only the implicit CWD default is dropped for batch.
	explicitHead := filepath.Join(tmp, "explicit-head.json")
	if err := os.WriteFile(explicitHead, []byte(`{"c":200}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "--headless", "--json", "--head", explicitHead).Output()
	if err != nil {
		t.Fatalf("explicit --head run: %v", err)
	}
	if got := fmt.Sprint(jsonValues(t, out)["c"]); got != "200" {
		t.Errorf("explicit --head should apply the named head in batch: c=%v, want 200", got)
	}
}

// jsonValues parses the batch --json envelope and returns its "values" submap
// (the cell results), failing the test if the shape is wrong.
func jsonValues(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var doc struct {
		Values map[string]any `json:"values"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not the expected {provenance, values} envelope: %v\n%s", err, out)
	}
	return doc.Values
}

// sameAllValues reports whether two result maps have identical values for all
// shared keys (a crude deep-equal over JSON scalars/strings).
func sameAllValues(a, b map[string]any) bool {
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			continue
		}
		if fmt.Sprint(av) != fmt.Sprint(bv) {
			return false
		}
	}
	return true
}

// repoRoot walks up to the go.mod.
func repoRoot(t *testing.T) string {
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
