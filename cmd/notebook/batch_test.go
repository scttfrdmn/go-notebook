package main

import (
	"encoding/json"
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
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if c, _ := got["c"].(float64); c != 120 {
		t.Errorf("c = %v, want 120 (the --set override should flow)", got["c"])
	}
	// cost = 120 * 1.006 = 120.72
	if cost, _ := got["cost"].(float64); cost < 120 || cost > 121 {
		t.Errorf("cost = %v, want ~120.72 (override must flow downstream)", got["cost"])
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
