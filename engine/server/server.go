// Package server exposes a [engine.Runtime] over HTTP: a page to view the
// notebook, a Server-Sent Events stream of cell updates, and an endpoint to
// post leaf edits.
//
// This package is the transport, and it is the ONLY place net/http appears. The
// engine emits Events on a channel and knows nothing about HTTP; the server
// subscribes and pushes. That separation is what keeps headless, WASM, and
// batch modes free — none of them link this package.
//
// The client is deliberately ignorant of Go types: it receives {cell, mime,
// data} blobs and renders them, and posts {leaf, value} edits back. Renderers
// run in Go, in-process.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/scttfrdmn/go-notebook/engine"
)

// Server serves a runtime over HTTP.
type Server struct {
	rt   *engine.Runtime
	meta []engine.CellMeta
	prov engine.Provenance
	set  SetFunc
	mux  *http.ServeMux
}

// SetFunc applies a leaf edit: it coerces the raw JSON value (numbers arrive as
// float64) into the leaf's static Go type and writes it through the head, then
// runs the wave. The generated main supplies this because only codegen knows
// each leaf's real type — the server itself is type-agnostic. A nil SetFunc
// falls back to writing the raw value (used in tests with already-typed values).
type SetFunc func(ctx context.Context, leaf string, raw any)

// New builds a server for a runtime, its cell metadata, and a type-aware leaf
// setter. If set is nil, edits write the raw JSON value directly (fine for
// tests whose leaves are already the right type). It carries no provenance;
// use [NewNotebook] to display build identity.
func New(rt *engine.Runtime, meta []engine.CellMeta, set SetFunc) *Server {
	return NewNotebook(rt, meta, engine.Provenance{}, set)
}

// NewNotebook is [New] plus build provenance, shown on the page so a served
// binary can say what it is. New delegates here with an empty Provenance, so the
// older signature keeps working unchanged.
func NewNotebook(rt *engine.Runtime, meta []engine.CellMeta, prov engine.Provenance, set SetFunc) *Server {
	if set == nil {
		set = func(ctx context.Context, leaf string, raw any) {
			rt.Set(ctx, engine.LeafID(leaf), raw)
		}
	}
	s := &Server{rt: rt, meta: meta, prov: prov, set: set, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/events", s.handleEvents)
	s.mux.HandleFunc("/set", s.handleSet)
	s.mux.HandleFunc("/leaves", s.handleLeaves)
	return s
}

// Handler returns the HTTP handler, so callers can wrap or test it without a
// live listener.
func (s *Server) Handler() http.Handler { return s.mux }

// handleEvents streams cell updates as Server-Sent Events. SSE (rather than a
// WebSocket) keeps this package stdlib-only: it is a one-directional
// server→client stream, which is exactly the event shape; edits flow back via
// the separate /set endpoint.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := s.rt.Subscribe()
	// Replay current state for the newly-connected client: the initial wave's
	// events were emitted before this subscription existed, so without this a
	// freshly-opened page would be blank until the first edit. Running a full
	// wave now re-emits every cell's current output to all subscribers.
	go s.rt.RunAll(r.Context())

	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub:
			// A write error means the client disconnected; the encoder error
			// path below returns, so the bare Fprints only need their errors
			// discarded.
			_, _ = fmt.Fprint(w, "data: ")
			if err := enc.Encode(engine.ToWire(ev)); err != nil {
				return
			}
			_, _ = fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}

// handleSet applies a leaf edit posted as {"leaf": "...", "value": <json>}. The
// write goes through the runtime, which funnels it into the head's single Set
// chokepoint and runs the resulting wave.
func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Leaf  string `json:"leaf"`
		Value any    `json:"value"`
	}
	// UseNumber so a numeric selection stays a json.Number, not a float64 — the
	// leaf coercer preserves int vs float (a Range[int] or an int64 id would be
	// silently floated otherwise).
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Run the wave in the background so the POST returns immediately; the client
	// sees results stream in over /events. Coalescing in the scheduler handles
	// rapid drags. The set func coerces the raw JSON value to the leaf's type.
	go s.set(context.Background(), req.Leaf, req.Value)
	w.WriteHeader(http.StatusNoContent)
}

// handleLeaves returns each leaf's current value keyed by leaf symbol, so the
// client can seed every control's initial position and readout — otherwise a
// slider sits at a browser default and the readout is blank until first drag
// (the wasm host publishes the same via __notebook_leaves; this is the SSE
// parallel). Values come from Finals (public), so this adds no engine surface.
// A wave is run first if none has yet, so Finals is populated even before any
// /events client connects.
func (s *Server) handleLeaves(w http.ResponseWriter, r *http.Request) {
	finals := s.rt.Finals()
	if len(finals) == 0 {
		// No wave has run yet (no /events client connected); run one so leaf
		// defaults exist. RunAll reads the head snapshot and commits finals.
		s.rt.RunAll(r.Context())
		finals = s.rt.Finals()
	}
	vals := map[string]any{}
	for _, m := range s.meta {
		if m.Leaf == "" {
			continue
		}
		if v, ok := finals[m.Leaf]; ok {
			vals[string(m.Leaf)] = v
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(vals)
}

// handleIndex serves the single-page client.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	metaJSON, _ := json.Marshal(s.meta)
	provJSON, _ := json.Marshal(s.prov)
	// The template contains literal % (CSS) and { } (JS), so substitute the
	// placeholders by string replace rather than a format verb.
	page := strings.Replace(indexHTML, metaPlaceholder, string(metaJSON), 1)
	page = strings.Replace(page, provPlaceholder, string(provJSON), 1)
	_, _ = fmt.Fprint(w, page)
}

// Serve starts an HTTP server on addr and blocks. Each client that connects to
// the event stream triggers a full wave, so a freshly-opened page always shows
// current state without a separate startup wave here. It shows no provenance;
// use [ServeNotebook] to display build identity.
func Serve(ctx context.Context, addr string, rt *engine.Runtime, meta []engine.CellMeta, set SetFunc) error {
	return ServeNotebook(ctx, addr, rt, meta, engine.Provenance{}, set)
}

// ServeNotebook is [Serve] plus build provenance, shown on the page so a served
// binary — e.g. one scp'd to a login node months ago — can say what it is. Serve
// delegates here with an empty Provenance, so the older signature is unchanged.
func ServeNotebook(ctx context.Context, addr string, rt *engine.Runtime, meta []engine.CellMeta, prov engine.Provenance, set SetFunc) error {
	s := NewNotebook(rt, meta, prov, set)
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving on %s: %w", addr, err)
	}
	return nil
}
