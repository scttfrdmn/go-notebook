package main

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// The supervisor owns the user-facing port and reverse-proxies to the child
// notebook binary on an internal port. This inverts the earlier model — where
// the child owned the port and the supervisor only watched files — because that
// made the only HTTP-speaking process the one process definitionally out of
// date about a failed rebuild, and left a dead port when a child crashed.
//
// The supervisor ALWAYS answers:
//   - building / no child yet → a "building…" page
//   - build failed            → the last-good notebook, with the error overlaid
//   - child crashed           → a "the process died, here's why" page
//   - healthy                 → the child's notebook, proxied verbatim
//
// Guardrail: this file routes requests and injects build status. It knows
// nothing about cells, widgets, or the graph — if it ever needs to, that is the
// framework growing back in a new place, and it must stop.

// phase is the supervisor's view of the notebook's health.
type phase int

const (
	phaseBuilding    phase = iota // no healthy child is serving yet
	phaseOK                       // a child is serving the notebook
	phaseBuildFailed              // a rebuild failed; the previous child still serves
	phaseCrashed                  // the child exited unexpectedly; nothing serves
)

func (p phase) String() string {
	switch p {
	case phaseBuilding:
		return "building"
	case phaseOK:
		return "ok"
	case phaseBuildFailed:
		return "build-failed"
	case phaseCrashed:
		return "crashed"
	default:
		return "unknown"
	}
}

// status is the supervisor's current health, guarded for concurrent reads by
// the proxy handler and writes by the build/watch loop.
type status struct {
	mu      sync.RWMutex
	phase   phase
	detail  string // error text / diagnostic (file:line) when not OK
	childTo string // current child base URL (e.g. http://127.0.0.1:54321), "" if none
}

func (s *status) set(p phase, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = p
	s.detail = detail
}

func (s *status) setChild(baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.childTo = baseURL
	s.phase = phaseOK
	s.detail = ""
}

func (s *status) snapshot() (phase, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase, s.detail, s.childTo
}

// supervisor is the user-facing HTTP surface.
type supervisor struct {
	status *status
	proxy  *httputil.ReverseProxy
}

func newSupervisor(st *status) *supervisor {
	s := &supervisor{status: st}
	s.proxy = &httputil.ReverseProxy{
		// Director reads the CURRENT child target on every request, so a swap to
		// a new child needs no proxy rebuild.
		Director: func(r *http.Request) {
			_, _, to := st.snapshot()
			if to == "" {
				return // no child; ServeHTTP handles this before proxying
			}
			u, err := url.Parse(to)
			if err != nil {
				return
			}
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		},
		// FlushInterval -1 flushes every write immediately — required for the
		// SSE event stream, which the default buffering would stall.
		FlushInterval: -1,
		// Inject the status poller into proxied HTML so an already-open notebook
		// learns about a build failure (and clears it on recovery) without the
		// child knowing anything. This is the "inject build status" half of the
		// supervisor's job — it appends a script, it does not understand the page.
		ModifyResponse: injectStatusPoller,
		// If the child is unreachable (crashed mid-request), don't 502 blankly —
		// report it as a crash the page can render.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			st.set(phaseCrashed, "the notebook process is unreachable: "+err.Error())
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprint(w, crashedPage(err.Error()))
		},
	}
	return s
}

// injectStatusPoller appends a small script to proxied HTML responses. The
// script polls /__status and, on a build failure, overlays the error on the
// already-open notebook (last-good stays live beneath it); on recovery it
// removes the overlay and reloads. It is appended, never parsed — the
// supervisor stays ignorant of the page it decorates.
func injectStatusPoller(resp *http.Response) error {
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		return nil // only decorate the page itself, not SSE/JSON/SVG
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	injected := make([]byte, 0, len(body)+len(statusPollerScript))
	injected = append(injected, body...)
	injected = append(injected, statusPollerScript...)
	resp.Body = io.NopCloser(bytes.NewReader(injected))
	resp.ContentLength = int64(len(injected))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(injected)))
	return nil
}

// statusPollerScript is appended to the proxied notebook page. It renders a
// fixed banner from /__status: a build error overlaid on the still-live
// notebook, cleared (with a reload) when the build recovers.
const statusPollerScript = `<script>
(function () {
  var bar = document.createElement('div');
  bar.style.cssText = 'position:fixed;left:0;right:0;top:0;z-index:99999;padding:.6rem 1rem;'
    + 'font:13px/1.4 -apple-system,system-ui,sans-serif;background:#d0433b;color:#fff;'
    + 'box-shadow:0 1px 6px rgba(0,0,0,.2);white-space:pre-wrap;display:none';
  document.addEventListener('DOMContentLoaded', function () { document.body.appendChild(bar); });
  var wasFailed = false;
  setInterval(async function () {
    try {
      var s = await (await fetch('/__status', {cache:'no-store'})).json();
      if (s.phase === 'build-failed') {
        wasFailed = true;
        bar.style.display = 'block';
        bar.textContent = 'build failed — still showing the last good version:\n' + s.detail;
      } else if (s.phase === 'ok') {
        if (wasFailed) { location.reload(); return; } // recovered: pick up the new build
        bar.style.display = 'none';
      }
    } catch (_) {}
  }, 1000);
})();
</script>`

func (s *supervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The supervisor's own status endpoint — served directly, never proxied.
	if r.URL.Path == "/__status" {
		p, detail, _ := s.status.snapshot()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = fmt.Fprintf(w, `{"phase":%q,"detail":%q}`, p.String(), detail)
		return
	}

	p, detail, to := s.status.snapshot()
	// No healthy child to proxy to: answer ourselves rather than a dead port.
	if to == "" || p == phaseCrashed {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if p == phaseCrashed {
			_, _ = fmt.Fprint(w, crashedPage(detail))
		} else {
			_, _ = fmt.Fprint(w, buildingPage())
		}
		return
	}
	// Healthy or build-failed-with-previous-child: proxy to the child. The
	// injected status poller (see banner) surfaces a build-failed overlay on the
	// already-open page without touching the child.
	s.proxy.ServeHTTP(w, r)
}

// pickFreePort asks the OS for an unused localhost TCP port for the child. The
// listener is closed immediately; the child binds it a moment later. The tiny
// race window is acceptable for a localhost dev loop.
func pickFreePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String(), nil
}

// buildingPage is served before any child is up.
func buildingPage() string {
	return supervisorPage("building…", "#5b6472",
		"Compiling the notebook. This page refreshes when it's ready.", true)
}

// crashedPage is served when the child exited unexpectedly — never a dead port.
func crashedPage(detail string) string {
	return supervisorPage("the notebook process died", "#d0433b",
		html.EscapeString(detail), true)
}

// supervisorPage renders a minimal status page that polls /__status and reloads
// itself when the notebook becomes healthy again. It is the supervisor's own
// surface (not the child's) and carries no notebook knowledge.
func supervisorPage(title, color, body string, poll bool) string {
	pollJS := ""
	if poll {
		pollJS = `<script>
setInterval(async () => {
  try {
    const r = await fetch('/__status', {cache:'no-store'});
    const s = await r.json();
    if (s.phase === 'ok') location.reload();
  } catch (_) {}
}, 1000);
</script>`
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8">
<title>notebook — %s</title>
<style>
  body { font: 15px/1.6 -apple-system, system-ui, sans-serif; margin: 4rem auto; max-width: 640px; color: #1a1a2e; }
  h1 { color: %s; font-size: 1.3rem; }
  pre { background: #0f1524; color: #e6ebf5; padding: 1rem; border-radius: 8px; overflow-x: auto; white-space: pre-wrap; }
</style></head><body>
<h1>%s</h1>
<pre>%s</pre>
%s
</body></html>`, html.EscapeString(title), color, html.EscapeString(title), body, pollJS)
}
