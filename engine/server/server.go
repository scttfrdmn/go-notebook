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
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/engine"
)

// maxSetBytes bounds a POST /set body. An edit is a leaf name plus a value, or a
// small values map — 1 MiB is far above any real edit and well below a payload
// that could exhaust memory, so it stops an accidental or malicious oversize POST
// without constraining legitimate use. (The supervisor's /__source has its own,
// separate limit for whole-file source edits.)
const maxSetBytes = 1 << 20 // 1 MiB

// Server serves a runtime over HTTP.
type Server struct {
	rt      *engine.Runtime
	meta    []engine.CellMeta
	prov    engine.Provenance
	layout  [][]string
	set     SetFunc
	setMany SetManyFunc
	mux     *http.ServeMux
}

// SetManyWith installs the type-aware atomic multi-leaf setter and returns the
// server, for chaining after construction. It is a post-construction option
// rather than a New/Serve parameter so the many serve entry points keep their
// signatures — the batch path is additive. Without it, /set still accepts a batch
// body and applies it under one epoch via the default (raw) SetMany fallback.
func (s *Server) SetManyWith(setMany SetManyFunc) *Server {
	if setMany != nil {
		s.setMany = setMany
	}
	return s
}

// SetFunc validates and applies a leaf edit. It coerces the raw JSON value
// (numbers arrive as float64) into the leaf's static Go type SYNCHRONOUSLY,
// returning an error if the leaf is unknown or the value will not coerce (wrapping
// [engine.ErrUnknownLeaf] / [engine.ErrBadValue] so the handler can map the
// status); on success it applies the write, running the recompute wave in the
// background so the caller need not wait for it. The generated main supplies this
// because only codegen knows each leaf's real type — the server itself is
// type-agnostic. A nil SetFunc falls back to writing the raw value (used in tests
// with already-typed values), which cannot fail, so it returns nil.
type SetFunc func(ctx context.Context, leaf string, raw any) error

// SetManyFunc validates and applies SEVERAL leaf edits as one atomic edit. It
// coerces every value FIRST (returning the first error, wrapping
// [engine.ErrUnknownLeaf]/[engine.ErrBadValue], and writing NOTHING if any fails)
// then commits them all under a single epoch and runs one wave — so the caller
// never lands an intermediate combination. It returns the committed epoch so the
// caller can correlate the edit with its settled event. The generated main
// supplies it (only codegen knows each leaf's type); a nil SetManyFunc means the
// server writes each raw value under one epoch via [engine.Runtime.SetMany]
// directly (the test fallback, which cannot fail).
type SetManyFunc func(ctx context.Context, vals map[string]any) (epoch uint64, err error)

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
		set = func(ctx context.Context, leaf string, raw any) error {
			// Tests supply already-typed values and no coercer; there is nothing to
			// validate, so this never fails. Background the wave to match the real
			// SetFunc's contract (the POST returns before recompute finishes).
			go rt.Set(ctx, engine.LeafID(leaf), raw)
			return nil
		}
	}
	// The default atomic multi-set: write the raw values under one epoch via
	// Runtime.SetMany. No coercion (tests pass already-typed values); the generated
	// main installs a type-aware SetManyFunc via SetManyWith. Unlike single /set
	// (fire-and-forget, for rapid-drag coalescing), a batch edit is a deliberate
	// one-shot, so it runs the wave and returns the committed epoch — the caller
	// wants to know it landed and to correlate it with the settled event.
	setMany := func(ctx context.Context, vals map[string]any) (uint64, error) {
		leaves := make(map[engine.LeafID]any, len(vals))
		for k, v := range vals {
			leaves[engine.LeafID(k)] = v
		}
		return uint64(rt.SetMany(ctx, leaves)), nil
	}
	s := &Server{rt: rt, meta: meta, prov: prov, set: set, setMany: setMany, mux: http.NewServeMux()}
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

// handleSet applies a leaf edit. Two body shapes on the same endpoint:
//
//	{"leaf": "c", "value": 40}                 — one leaf (fire-and-forget)
//	{"values": {"c": 40, "price": 3.5}}        — several leaves, ONE atomic edit
//
// The single form backgrounds its wave (rapid drags coalesce). The batch form is
// a deliberate one-shot: it validates every value first, commits them under one
// epoch, runs one wave, and returns the committed epoch in X-Notebook-Epoch — so
// a form or a parameter sweep never lands an intermediate combination and can
// correlate its edit with the settled event. Both funnel through the head's
// single chokepoint.
func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// Cap the body: a leaf edit (or a batch of them) is small — a leaf name plus a
	// value or a modest values map. A multi-megabyte POST is a mistake or an abuse,
	// not a real edit, so bound it rather than decode unboundedly. MaxBytesReader
	// makes the decode below fail once the limit is crossed; we map that to 413.
	r.Body = http.MaxBytesReader(w, r.Body, maxSetBytes)
	var req struct {
		Leaf   string         `json:"leaf"`
		Value  any            `json:"value"`
		Values map[string]any `json:"values"`
	}
	// UseNumber so a numeric selection stays a json.Number, not a float64 — the
	// leaf coercer preserves int vs float (a Range[int] or an int64 id would be
	// silently floated otherwise).
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		// MaxBytesReader signals the overflow with a *http.MaxBytesError; report it
		// as 413 so an oversize body is distinct from malformed JSON (400).
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Batch form: {"values": {...}}. Validate all, commit atomically, report epoch.
	if req.Values != nil {
		epoch, err := s.setMany(context.Background(), req.Values)
		s.writeSetStatus(w, err, epoch)
		return
	}

	// Single form. Validate synchronously: the set func coerces the value and
	// returns an error for an unknown leaf or an uncoercible value WITHOUT waiting
	// for the wave (it backgrounds that itself). So a bad edit gets a real status
	// instead of a silent 204 that changed nothing.
	s.writeSetStatus(w, s.set(context.Background(), req.Leaf, req.Value), 0)
}

// writeSetStatus maps a set/setMany result to the HTTP status: 204 accepted (with
// X-Notebook-Epoch when a batch edit reported its committed epoch), 404 unknown
// leaf, 422 uncoercible value, 400 otherwise. One place so single and batch agree.
func (s *Server) writeSetStatus(w http.ResponseWriter, err error, epoch uint64) {
	switch {
	case err == nil:
		if epoch > 0 {
			w.Header().Set("X-Notebook-Epoch", strconv.FormatUint(epoch, 10))
		}
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, engine.ErrUnknownLeaf):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, engine.ErrBadValue):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// handleLeaves returns each leaf's current value keyed by leaf symbol, so the
// client can seed every control's initial position and readout — otherwise a
// slider sits at a browser default and the readout is blank until first drag
// (the wasm port exposes the same via notebook.values(); this is the SSE
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
	layoutJSON, _ := json.Marshal(s.layout) // null when no layout → source order
	// The template contains literal % (CSS) and { } (JS), so substitute the
	// placeholders by string replace rather than a format verb.
	page := strings.Replace(indexHTML, metaPlaceholder, string(metaJSON), 1)
	page = strings.Replace(page, provPlaceholder, string(provJSON), 1)
	page = strings.Replace(page, layoutPlaceholder, string(layoutJSON), 1)
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
	return ServeNotebookReady(ctx, addr, rt, meta, prov, nil, set, nil)
}

// ServeNotebookReady is [ServeNotebook] that binds the listener BEFORE serving
// and reports the resolved address through onReady, so a caller (or a launcher
// reading the process's stdout) learns the real port even when addr requested an
// OS-assigned one (host:0). This is the addressing half of the notebook-as-service
// seam (docs/notebook-as-service.md): the process picks its port — only it knows
// what is free on the box — but ANNOUNCES it rather than fixing it, so a parent
// never has to poll-and-hope. onReady is called once, with the bound host:port,
// just before the accept loop starts; a nil onReady preserves the old behavior.
func ServeNotebookReady(ctx context.Context, addr string, rt *engine.Runtime, meta []engine.CellMeta, prov engine.Provenance, layout [][]string, set SetFunc, onReady func(addr string), opts ...func(*Server)) error {
	s := NewNotebook(rt, meta, prov, set)
	s.layout = layout
	// Post-construction options (e.g. SetManyWith for a type-aware batch setter).
	// Variadic so existing callers are unchanged and the option is additive.
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	// Listen first, so a host:0 request resolves to a concrete port we can report.
	// http.Server.ListenAndServe hides the listener, which is exactly why :0 is
	// useless there — the chosen port is never surfaced.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if onReady != nil {
		onReady(ln.Addr().String())
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving on %s: %w", ln.Addr(), err)
	}
	return nil
}
