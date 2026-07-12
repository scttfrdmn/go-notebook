package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
