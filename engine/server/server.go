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
	mux  *http.ServeMux
}

// New builds a server for a runtime and its cell metadata.
func New(rt *engine.Runtime, meta []engine.CellMeta) *Server {
	s := &Server{rt: rt, meta: meta, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/events", s.handleEvents)
	s.mux.HandleFunc("/set", s.handleSet)
	return s
}

// Handler returns the HTTP handler, so callers can wrap or test it without a
// live listener.
func (s *Server) Handler() http.Handler { return s.mux }

// wireEvent is the JSON shape pushed to the client for each cell update. It
// carries no Go types — just the cell id, its state, and an optional rendered
// blob (mime + data).
type wireEvent struct {
	Epoch uint64 `json:"epoch"`
	Cell  string `json:"cell"`
	State string `json:"state"`
	MIME  string `json:"mime,omitempty"`
	Data  string `json:"data,omitempty"`
	Err   string `json:"err,omitempty"`
}

func toWire(ev engine.Event) wireEvent {
	w := wireEvent{
		Epoch: uint64(ev.Epoch),
		Cell:  string(ev.Cell),
		State: ev.State.String(),
		Err:   ev.Err,
	}
	if ev.Out != nil {
		w.MIME = ev.Out.MIME
		w.Data = ev.Out.Data
	}
	return w
}

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
			if err := enc.Encode(toWire(ev)); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Run the wave in the background so the POST returns immediately; the client
	// sees results stream in over /events. Coalescing in the scheduler handles
	// rapid drags.
	go s.rt.Set(context.Background(), engine.LeafID(req.Leaf), req.Value)
	w.WriteHeader(http.StatusNoContent)
}

// handleIndex serves the single-page client.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	metaJSON, _ := json.Marshal(s.meta)
	// The template contains literal % (CSS) and { } (JS), so substitute the one
	// placeholder by string replace rather than a format verb.
	page := strings.Replace(indexHTML, metaPlaceholder, string(metaJSON), 1)
	_, _ = fmt.Fprint(w, page)
}

// Serve starts an HTTP server on addr and blocks. Each client that connects to
// the event stream triggers a full wave, so a freshly-opened page always shows
// current state without a separate startup wave here.
func Serve(ctx context.Context, addr string, rt *engine.Runtime, meta []engine.CellMeta) error {
	s := New(rt, meta)
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
