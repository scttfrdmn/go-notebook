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
