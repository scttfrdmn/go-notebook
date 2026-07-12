package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	sup := httptest.NewServer(newSupervisor(st))
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
	sup := httptest.NewServer(newSupervisor(st))
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
	sup := httptest.NewServer(newSupervisor(st))
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
