package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServiceReadinessAndDrive is the notebook-as-service seam
// (docs/notebook-as-service.md), end to end on the built binary: launch with
// --addr 127.0.0.1:0 (an OS-assigned port), read the {event:"ready",addr}
// readiness line from stdout, then drive /set on the REPORTED port and confirm
// /leaves reflects it. This is the local, $0 half of KC18 — a launcher learns
// the address from stdout instead of polling-and-hoping, and the child never
// fixes a port a parent has to guess.
func TestServiceReadinessAndDrive(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a binary; skipped in -short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "svcnb")
	if code := cmdBuild([]string{"-o", bin, filepath.Join(root, "examples", "capacity")}); code != 0 {
		t.Fatalf("build returned %d", code)
	}

	cmd := exec.Command(bin, "--addr", "127.0.0.1:0", "--head", filepath.Join(tmp, "h.json"))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	// Read stdout until the readiness line appears (or the pipe closes / times
	// out). The line is machine-readable JSON with the RESOLVED address.
	addr := readReadyAddr(t, stdout)
	if addr == "" {
		t.Fatal("no readiness line on stdout")
	}
	if strings.HasSuffix(addr, ":0") {
		t.Fatalf("readiness reported an unresolved port %q — :0 must resolve to a real port", addr)
	}

	base := "http://" + addr
	// The reported port must actually serve: read the default and drive a new value.
	if got := leafC(t, base); got != 80 {
		t.Fatalf("default c = %v, want 80 (is the reported port serving?)", got)
	}
	body := strings.NewReader(`{"leaf":"c","value":40}`)
	resp, err := http.Post(base+"/set", "application/json", body)
	if err != nil {
		t.Fatalf("POST /set on the reported port: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("accepted /set = %d, want 204", resp.StatusCode)
	}

	// A bad edit must fail loud on the REAL generated coercer — not a silent 204.
	// This is #3 end-to-end: an unknown leaf is 404, a wrong-typed value is 422, so
	// a programmatic driver can tell a dropped edit from an applied one. Before, all
	// three returned 204 and the error was only logged on the far side.
	assertSetStatus(t, base, `{"leaf":"nope","value":1}`, http.StatusNotFound)
	assertSetStatus(t, base, `{"leaf":"c","value":"not a number"}`, http.StatusUnprocessableEntity)

	// By consequence: the accepted drive on the reported port took effect.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if leafC(t, base) == 40 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("c did not become 40 after driving /set on the reported port %s", addr)
}

// readReadyAddr scans stdout for the {event:"ready",addr,...} line and returns
// its addr, bounded so a wedged binary can't hang the test.
func readReadyAddr(t *testing.T, r io.Reader) string {
	t.Helper()
	type ready struct {
		Event string `json:"event"`
		Addr  string `json:"addr"`
	}
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return ""
			}
			var rd ready
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &rd) == nil && rd.Event == "ready" {
				return rd.Addr
			}
		case <-deadline:
			return ""
		}
	}
}

// leafC fetches /leaves and returns the current value of the c leaf.
func leafC(t *testing.T, base string) float64 {
	t.Helper()
	resp, err := http.Get(base + "/leaves")
	if err != nil {
		return -1
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, resp.Body)
	var vals map[string]any
	if json.Unmarshal(buf.Bytes(), &vals) != nil {
		return -1
	}
	c, _ := vals["c"].(float64)
	return c
}

// assertSetStatus POSTs a /set body to the built binary and checks the HTTP
// status — the observable signal a driver reads to know whether its edit was
// accepted, applied against the real generated coercer.
func assertSetStatus(t *testing.T, base, body string, want int) {
	t.Helper()
	resp, err := http.Post(base+"/set", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /set %s: %v", body, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Errorf("POST /set %s = %d, want %d", body, resp.StatusCode, want)
	}
}
