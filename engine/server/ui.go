package server

import "github.com/scttfrdmn/go-notebook/internal/webui"

// metaPlaceholder is replaced with the cell metadata JSON when the page is
// served. A string replace (not a format verb) is used because the assembled
// page contains literal % (CSS) and { } (JS).
const metaPlaceholder = "/*__META__*/null"

// indexHTML is the SSE server's page. The page SHELL and the client (CSS,
// control/cell builder, dependency graph, event renderer) are assembled by
// internal/webui and shared with the WASM host, so this package no longer owns
// any presentation — it supplies only the SSE transport glue: inline the
// metadata, send edits via POST /set, and feed the /events stream to the shared
// renderer. engine/server is a transport again; it serves what webui hands it.
var indexHTML = webui.Page(webui.PageOpts{
	Title: "notebook",
	Glue: `const META = ` + metaPlaceholder + `;
NB.init(META, {
  onEdit: (leaf, value) => fetch('/set', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({leaf, value}),
  }),
});
const es = new EventSource('/events');
es.onmessage = (e) => { try { NB.render(JSON.parse(e.data)); } catch (_) {} };`,
})
