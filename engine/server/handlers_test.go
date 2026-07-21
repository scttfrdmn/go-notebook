package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scttfrdmn/go-notebook/engine"
)

// TestHandleIndexServesPage confirms the index handler returns the client page
// with the cell metadata injected, and 404s an unknown path.
func TestHandleIndexServesPage(t *testing.T) {
	rt, meta := testRuntime(t)
	h := New(rt, meta, nil).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>notebook</title>") {
		t.Error("index page missing title")
	}
	if strings.Contains(body, metaPlaceholder) {
		t.Error("meta placeholder was not substituted")
	}
	if !strings.Contains(body, `"Leaf":"n"`) {
		t.Error("cell metadata (leaf n) not injected into the page")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", rec.Code)
	}
}

// TestHandleSetRejectsBadRequests covers the /set error paths: wrong method and
// malformed JSON.
func TestHandleSetRejectsBadRequests(t *testing.T) {
	rt, meta := testRuntime(t)
	h := New(rt, meta, nil).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/set", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /set = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/set", strings.NewReader("{not json"))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /set with bad JSON = %d, want 400", rec.Code)
	}
}

// TestNewNilSetFallback confirms the default SetFunc (nil) writes the raw value
// through the runtime — the path used by tests whose values are already typed.
func TestNewNilSetFallback(t *testing.T) {
	rt, meta := testRuntime(t)
	h := New(rt, meta, nil).Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/set", strings.NewReader(`{"leaf":"n","value":7}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("POST /set = %d, want 204", rec.Code)
	}
}

// TestLoopbackHost pins the Host classifier: loopback literals (with or without a
// port, v4 or v6, and the "localhost" name) pass; anything else fails. This is the
// predicate the DNS-rebinding guard turns on.
func TestLoopbackHost(t *testing.T) {
	pass := []string{"127.0.0.1", "127.0.0.1:8080", "localhost", "localhost:80", "[::1]", "[::1]:8080", "127.0.0.5:9"}
	fail := []string{"", "evil.com", "evil.com:80", "192.168.1.9", "192.168.1.9:8080", "notebook.local", "0.0.0.0:80"}
	for _, h := range pass {
		if !loopbackHost(h) {
			t.Errorf("loopbackHost(%q) = false, want true", h)
		}
	}
	for _, h := range fail {
		if loopbackHost(h) {
			t.Errorf("loopbackHost(%q) = true, want false", h)
		}
	}
}

// TestHostGuardRejectsForgedHost is the DNS-rebinding defense (#226): a
// loopback-bound server accepts a loopback Host and rejects a forged one (403),
// while a server without the guard (the default New(), as tests use) accepts any
// Host — so the guard is opt-in to a loopback bind, never a regression to callers
// that didn't ask for it.
func TestHostGuardRejectsForgedHost(t *testing.T) {
	rt, meta := testRuntime(t)

	// Default handler (no loopback binding) accepts any Host — unchanged behavior.
	open := New(rt, meta, nil).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/leaves", nil)
	req.Host = "evil.com"
	open.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Error("the default (non-loopback-guarded) handler must not reject a Host — that would regress the New() path")
	}

	// A loopback-bound server: forged Host → 403, loopback Host → allowed.
	guarded := New(rt, meta, nil)
	guarded.requireLoopbackHost = true
	h := guarded.Handler()

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/leaves", nil)
	req.Host = "evil.com"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("forged Host on a loopback-bound server = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/leaves", nil)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Errorf("loopback Host on a loopback-bound server = 403, want it allowed")
	}
}

// TestHandleSetRejectsOversizeBody confirms POST /set bounds its body: a payload
// past the cap is 413, not decoded unboundedly. A leaf edit is tiny, so a
// multi-megabyte body is a mistake or abuse — the localhost boundary should
// refuse it rather than allocate for it.
func TestHandleSetRejectsOversizeBody(t *testing.T) {
	rt, meta := testRuntime(t)
	h := New(rt, meta, nil).Handler()

	// A JSON body comfortably over the 1 MiB cap.
	big := `{"leaf":"n","value":"` + strings.Repeat("x", (1<<20)+1024) + `"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/set", strings.NewReader(big))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize POST /set = %d, want 413", rec.Code)
	}
}

// validatingSet mirrors the generated SetFunc's contract for the test: it coerces
// synchronously, wrapping engine.ErrUnknownLeaf for a name it doesn't know and
// engine.ErrBadValue for a value of the wrong kind, and only "applies" (a no-op
// here) on success. This is what a real built notebook's /set closure does, so the
// handler's status mapping is exercised against the true contract, not a mock that
// always succeeds.
func validatingSet(_ context.Context, leaf string, raw any) error {
	if leaf != "n" {
		return fmt.Errorf("%w: %s", engine.ErrUnknownLeaf, leaf)
	}
	// A real leaf coercer runs engine.CoerceWire first (json.Number → float64),
	// then asserts the leaf's kind. Do the same here so the test drives the true
	// path: the handler's UseNumber decoder hands us a json.Number for a number and
	// a string for a string.
	norm, ok := engine.CoerceWire(raw)
	if !ok {
		return fmt.Errorf("%w: leaf %q: cannot coerce %v (%T)", engine.ErrBadValue, leaf, raw, raw)
	}
	if _, ok := norm.(float64); !ok { // "n" is numeric
		return fmt.Errorf("%w: leaf %q: want a number, got %v (%T)", engine.ErrBadValue, leaf, norm, norm)
	}
	return nil
}

// TestHandleSetStatusReflectsValidation is the #3 behavior: an invalid /set no
// longer looks successful. A typo'd leaf gets 404, a wrong-typed value gets 422,
// and only an accepted edit gets 204 — so a programmatic driver can tell a
// silently-dropped edit from an applied one. Before this, all three returned 204.
func TestHandleSetStatusReflectsValidation(t *testing.T) {
	rt, meta := testRuntime(t)
	h := New(rt, meta, validatingSet).Handler()

	cases := []struct {
		name, body string
		want       int
	}{
		{"accepted edit", `{"leaf":"n","value":7}`, http.StatusNoContent},
		{"unknown leaf", `{"leaf":"nope","value":7}`, http.StatusNotFound},
		{"uncoercible value", `{"leaf":"n","value":"not a number"}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/set", strings.NewReader(tc.body))
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("POST /set %s = %d, want %d (body: %s)", tc.name, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestHandleSetBatch is the atomic multi-leaf edit (#225): a {"values": {...}}
// body validates every value first, commits under one epoch, and reports it in
// X-Notebook-Epoch — and a bad leaf in the batch fails the whole edit (404),
// writing nothing. The test's SetManyFunc mirrors the generated coerce-all-first
// contract; the testRuntime leaf is "n".
func TestHandleSetBatch(t *testing.T) {
	rt, meta := testRuntime(t)
	// A coerce-all-first batch setter: any unknown leaf fails the whole map.
	setMany := func(_ context.Context, vals map[string]any) (uint64, error) {
		for leaf := range vals {
			if leaf != "n" {
				return 0, fmt.Errorf("%w: %s", engine.ErrUnknownLeaf, leaf)
			}
		}
		return uint64(rt.SetMany(context.Background(), toLeaves(vals))), nil
	}
	h := New(rt, meta, validatingSet).SetManyWith(setMany).Handler()

	t.Run("accepted batch reports its epoch", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/set", strings.NewReader(`{"values":{"n":7}}`))
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("batch /set = %d, want 204 (%s)", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Notebook-Epoch") == "" {
			t.Error("accepted batch must report X-Notebook-Epoch")
		}
	})

	t.Run("bad leaf fails the whole batch", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/set", strings.NewReader(`{"values":{"n":7,"nope":1}}`))
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("batch with an unknown leaf = %d, want 404", rec.Code)
		}
	})
}

// toLeaves converts a string-keyed value map to LeafID keys for rt.SetMany.
func toLeaves(vals map[string]any) map[engine.LeafID]any {
	out := make(map[engine.LeafID]any, len(vals))
	for k, v := range vals {
		out[engine.LeafID(k)] = v
	}
	return out
}
