package main

import "github.com/scttfrdmn/go-notebook/internal/webui"

// indexHTMLWASM is the browser host for a --target=wasm notebook. The client
// itself — CSS, control/cell builder, dependency graph, event renderer — is
// shared with the SSE server in internal/webui, so the two transports show the
// SAME notebook. This file supplies only the WASM transport glue: bootstrap the
// Go wasm runtime, take metadata from __notebook_meta, deliver events via
// __notebook_event, edit leaves via notebookSet, seed initial values from
// __notebook_leaves, and report iframe height for the landing page.
//
// __NB_NAME__ is replaced with the notebook name (by string replace, not
// Sprintf, because the shared CSS/JS contain literal %).
const indexHTMLWASM = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>__NB_NAME__ — go-notebook (wasm)</title>
<style>` + webui.CSS + `</style>
</head>
<body>
<h1>__NB_NAME__ <span style="font-weight:400;color:#888">· running in your browser, no server</span></h1>
<div id="status">loading wasm…</div>
<div class="graph" id="graph"></div>
<div class="controls" id="controls"></div>
<div id="cells"></div>
<script src="wasm_exec.js"></script>
<script>` + webui.JS + `

// --- WASM transport glue ---------------------------------------------------

const status = document.getElementById('status');

// When embedded in an iframe (the landing page), report content height so the
// parent can size the frame. No-op when opened directly. Wired as NB's
// afterRender hook so it runs after every repaint.
function reportHeight() {
  if (window.parent === window) return;
  window.parent.postMessage({ type: 'notebook:height', height: Math.ceil(document.documentElement.scrollHeight) }, '*');
}
window.addEventListener('resize', reportHeight);

// The engine calls this per cell update; hand it straight to the shared renderer.
globalThis.__notebook_event = (ev) => NB.render(ev);

// The engine publishes leaf defaults after the initial wave; seed the controls.
globalThis.__notebook_leaves_ready = () => {
  try { NB.seedLeaves(JSON.parse(globalThis.__notebook_leaves || '{}')); } catch (_) {}
};

function start() {
  const META = JSON.parse(globalThis.__notebook_meta || '[]');
  NB.init(META, {
    onEdit: (leaf, value) => globalThis.notebookSet(leaf, value),
    afterRender: reportHeight,
  });
  status.textContent = 'ready — compiled Go, no server';
  reportHeight();
  // UI is built and cell elements exist — now trigger the initial wave, so its
  // first render lands in a DOM element that's already there (no drop race).
  globalThis.notebookStart();
}

const go = new Go();
// instantiateStreaming needs Content-Type: application/wasm; if a host serves
// the wrong MIME it hard-fails, so fall back to a plain fetch+arrayBuffer.
async function instantiate() {
  try {
    return await WebAssembly.instantiateStreaming(fetch('notebook.wasm'), go.importObject);
  } catch (_) {
    const bytes = await (await fetch('notebook.wasm')).arrayBuffer();
    return await WebAssembly.instantiate(bytes, go.importObject);
  }
}
instantiate().then((r) => {
  go.run(r.instance);
  const wait = setInterval(() => {
    if (globalThis.__notebook_ready) { clearInterval(wait); start(); }
  }, 5);
}).catch((e) => { status.textContent = 'wasm load failed: ' + e; });
</script>
</body>
</html>
`
