package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSupervisorProxiesAndStreams is the cheapest falsification of the proxy:
// with a healthy child behind it, ordinary requests proxy through AND an SSE
// stream flows without buffering. SSE-through-a-proxy is the real risk (default
// buffering stalls the stream), so it is what this proves first.
func TestSupervisorProxiesAndStreams(t *testing.T) {
	// A fake child: a plain endpoint and an SSE endpoint that emits ticks.
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			f, ok := w.(http.Flusher)
			if !ok {
				t.Error("child ResponseWriter is not a Flusher")
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < 3; i++ {
				_, _ = fmt.Fprintf(w, "data: tick %d\n\n", i)
				f.Flush()
				time.Sleep(20 * time.Millisecond)
			}
			return
		}
		_, _ = fmt.Fprintf(w, "hello from child at %s", r.URL.Path)
	}))
	defer child.Close()

	st := &status{}
	st.setChild(child.URL)
	sup := httptest.NewServer(newSupervisor(st, ""))
	defer sup.Close()

	// Ordinary request proxies through.
	resp, err := http.Get(sup.URL + "/foo")
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "hello from child at /foo") {
		t.Errorf("proxy did not forward path; got %q", body)
	}

	// SSE streams through incrementally: read the first tick well before the
	// child has finished emitting all three (which takes ~60ms). If the proxy
	// buffered, we'd block until the stream closed.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", sup.URL+"/events", nil)
	sresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sresp.Body.Close() }()
	sc := bufio.NewScanner(sresp.Body)
	first := time.Now()
	for sc.Scan() {
		if strings.Contains(sc.Text(), "tick 0") {
			if elapsed := time.Since(first); elapsed > 500*time.Millisecond {
				t.Errorf("first SSE tick took %v — proxy is buffering, not streaming", elapsed)
			}
			return // streamed through incrementally
		}
	}
	t.Fatal("never received an SSE tick through the proxy")
}

// TestSupervisorAlwaysAnswers proves the supervisor answers in every phase
// instead of a dead port or blank page (§8: drive each state, observe the
// page).
func TestSupervisorAlwaysAnswers(t *testing.T) {
	st := &status{}
	sup := httptest.NewServer(newSupervisor(st, ""))
	defer sup.Close()

	// Building: no child yet → a building page, HTTP 200 (not a connection error).
	st.set(phaseBuilding, "")
	if b := get(t, sup.URL+"/"); !strings.Contains(b, "building") {
		t.Errorf("building phase should serve a building page; got %q", b)
	}

	// Crashed: no child, crash detail → the death page carrying the reason.
	st.set(phaseCrashed, "signal: killed")
	if b := get(t, sup.URL+"/"); !strings.Contains(b, "died") || !strings.Contains(b, "signal: killed") {
		t.Errorf("crashed phase should serve the death page with the reason; got %q", b)
	}

	// Status endpoint reports the phase for the page poller.
	if b := get(t, sup.URL+"/__status"); !strings.Contains(b, `"phase":"crashed"`) {
		t.Errorf("/__status should report the phase; got %q", b)
	}
}

// TestSupervisorInjectsStatusPollerIntoHTML confirms the build-status poller is
// appended to proxied HTML (so an open page learns of a failure) but NOT to
// non-HTML responses like the SSE stream.
func TestSupervisorInjectsStatusPollerIntoHTML(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: x\n\n")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<html><body>notebook</body></html>")
	}))
	defer child.Close()

	st := &status{}
	st.setChild(child.URL)
	sup := httptest.NewServer(newSupervisor(st, ""))
	defer sup.Close()

	// HTML gets the poller appended.
	if b := get(t, sup.URL+"/"); !strings.Contains(b, "/__status") || !strings.Contains(b, "notebook") {
		t.Errorf("HTML page should carry the status poller and the original body; got %q", b)
	}
	// The SSE stream does not (it must stay a clean event stream).
	if b := get(t, sup.URL+"/events"); strings.Contains(b, "/__status") {
		t.Errorf("SSE response must not be decorated; got %q", b)
	}
}

// TestStatusRecoversFromBuildFailed pins the recovery the driven test caught
// regressing: build-failed → ok clears the error and points at the new child.
// (The bug was the swap's intentional child-kill being misread as a crash,
// which stuck the phase at crashed; this asserts the status machine itself
// recovers cleanly.)
func TestStatusRecoversFromBuildFailed(t *testing.T) {
	st := &status{}
	st.setChild("http://127.0.0.1:1")
	if p, _, _ := st.snapshot(); p != phaseOK {
		t.Fatalf("setChild should be ok, got %v", p)
	}
	st.set(phaseBuildFailed, "capacity.go:66: boom")
	if p, d, to := st.snapshot(); p != phaseBuildFailed || d == "" || to == "" {
		t.Fatalf("build-failed must keep the child target and carry detail; got %v %q %q", p, d, to)
	}
	// A fresh child recovers to ok with the error cleared.
	st.setChild("http://127.0.0.1:2")
	if p, d, to := st.snapshot(); p != phaseOK || d != "" || to != "http://127.0.0.1:2" {
		t.Fatalf("recovery should be ok with cleared detail and new child; got %v %q %q", p, d, to)
	}
}

// TestSupervisorServesSource is A1's falsification: GET /__source returns the
// exact on-disk bytes and is served by the supervisor ITSELF — so it works even
// with no child (phaseBuilding), which is the point (a browser editor loads the
// source while the notebook is still compiling). And with no source path
// configured it 404s, so the client feature-detects the editor off.
func TestSupervisorServesSource(t *testing.T) {
	const src = "//go:notebook\npackage demo\n\nfunc x() (n int) { return 42 }\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	// No child set: phase is the zero value (phaseBuilding), childTo == "".
	st := &status{}
	sup := httptest.NewServer(newSupervisor(st, path))
	defer sup.Close()

	resp, err := http.Get(sup.URL + "/__source")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /__source status = %d, want 200 (must serve during phaseBuilding)", resp.StatusCode)
	}
	got := readAllRaw(t, resp)
	if got != src {
		t.Errorf("GET /__source returned\n%q\nwant\n%q", got, src)
	}

	// With no source path, the endpoint 404s (editor unavailable → client falls
	// back to read-only source).
	supNo := httptest.NewServer(newSupervisor(&status{}, ""))
	defer supNo.Close()
	r2, err := http.Get(supNo.URL + "/__source")
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusNotFound {
		t.Errorf("GET /__source with no srcPath = %d, want 404", r2.StatusCode)
	}
}

// TestSupervisorWritesSource is A2's unit falsification: POST /__source replaces
// the file atomically and rejects an over-cap body before touching disk. (The
// rebuild it triggers is exercised end-to-end in service_test.go against the
// real loop; here we prove the write itself is correct and bounded.)
func TestSupervisorWritesSource(t *testing.T) {
	const orig = "//go:notebook\npackage demo\n\nfunc x() (n int) { return 1 }\n"
	const edited = "//go:notebook\npackage demo\n\nfunc x() (n int) { return 999 }\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.go")
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	sup := httptest.NewServer(newSupervisor(&status{}, path))
	defer sup.Close()

	// A valid POST replaces the file and 204s.
	resp, err := http.Post(sup.URL+"/__source", "text/plain", strings.NewReader(edited))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /__source status = %d, want 204", resp.StatusCode)
	}
	on, _ := os.ReadFile(path)
	if string(on) != edited {
		t.Errorf("file after POST =\n%q\nwant\n%q", on, edited)
	}
	// No staging temp files are left behind in the directory.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".notebook-src-") {
			t.Errorf("leftover temp file after write: %s", e.Name())
		}
	}

	// An over-cap body is rejected and the file is left untouched.
	big := strings.Repeat("x", maxSourceBytes+1)
	r2, err := http.Post(sup.URL+"/__source", "text/plain", strings.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize POST status = %d, want 413", r2.StatusCode)
	}
	on2, _ := os.ReadFile(path)
	if string(on2) != edited {
		t.Errorf("oversize POST must not touch the file; got\n%q", on2)
	}
}

// readAllRaw reads the full body verbatim (bytes preserved, unlike readAll which
// scans line by line and drops newlines).
func readAllRaw(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		b.WriteString(sc.Text())
	}
	return b.String()
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return readAll(t, resp)
}
