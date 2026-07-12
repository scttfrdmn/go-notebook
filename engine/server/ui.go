package server

import "github.com/scttfrdmn/go-notebook/internal/webui"

// metaPlaceholder is replaced with the cell metadata JSON in indexHTML. A
// string replace (not a format verb) is used because the template contains
// literal % (CSS) and { } (JS).
const metaPlaceholder = "/*__META__*/null"

// indexHTML is the SSE server's page. The client itself — CSS, control/cell
// builder, dependency graph, event renderer — is shared with the WASM host in
// internal/webui, so the two transports cannot drift into showing different
// notebooks. This file supplies only the SSE transport glue: inline the
// metadata, send edits via POST /set, and feed the /events stream to the shared
// renderer.
const indexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>notebook</title>
<style>` + webui.CSS + `</style>
</head>
<body>
<h1>notebook</h1>
<div class="graph" id="graph"></div>
<div class="controls" id="controls"></div>
<div id="cells"></div>
<script>` + webui.JS + `
const META = /*__META__*/null;
NB.init(META, {
  onEdit: (leaf, value) => fetch('/set', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({leaf, value}),
  }),
});
const es = new EventSource('/events');
es.onmessage = (e) => { try { NB.render(JSON.parse(e.data)); } catch (_) {} };
</script>
</body>
</html>
`
