package main

import "github.com/scttfrdmn/go-notebook/internal/webui"

// indexHTMLWASM is the browser host for a --target=wasm notebook. The page shell
// and the client are assembled by internal/webui and shared with the SSE server,
// so this supplies only the WASM transport glue.
//
// This glue is a CONSUMER of the port (globalThis.notebook), not a privileged
// part of it — the same standing a foreign host page has. It reads notebook.meta,
// sends edits with notebook.set, subscribes for values with notebook.subscribe,
// pulls initial control positions with notebook.values, and triggers the first
// wave with notebook.start. Our default UI (webui.NB) sits on top of exactly the
// surface a stranger would hold; that it can be rebuilt on any other renderer is
// the point of naming the port.
//
// __NB_NAME__ is replaced with the notebook name (by string replace, not
// Sprintf, because the assembled page contains literal %).
var indexHTMLWASM = webui.Page(webui.PageOpts{
	Title:     "__NB_NAME__ — go-notebook (wasm)",
	Subtitle:  "· running in your browser, no server",
	Status:    true,
	HeadExtra: `<script src="wasm_exec.js"></script>`,
	Glue: `
const status = document.getElementById('status');

// When embedded in an iframe (the landing page), report content height so the
// parent can size the frame. No-op when opened directly. Wired as NB's
// afterRender hook so it runs after every repaint.
function reportHeight() {
  if (window.parent === window) return;
  window.parent.postMessage({ type: 'notebook:height', height: Math.ceil(document.documentElement.scrollHeight) }, '*');
}
window.addEventListener('resize', reportHeight);

// Build the UI on the port. This runs once the port exists (after go.run).
function start(nb) {
  NB.init(nb.meta, {
    onEdit: (leaf, value) => nb.set(leaf, value),
    afterRender: reportHeight,
    provenance: nb.provenance,
  });
  // The value channel IS the subscription: every cell's value arrives here, and
  // a scalar control seeds its starting position from its OWN first event (the
  // read-path analogue of the write path — init and update are one path, not a
  // separate seed channel that can race or be empty). Subscribe BEFORE start so
  // no initial render is missed.
  nb.subscribe((ev) => NB.render(ev));
  status.textContent = 'ready — compiled Go, no server';
  reportHeight();
  // UI built and subscribed — now trigger the first wave so its renders land in
  // DOM elements that already exist (no drop race). Seeding needs no separate
  // step: the wave's events carry each control's default. notebook.values() is
  // the pull form of the same channel, for a host that wants a synchronous
  // snapshot instead of subscribing.
  nb.start();
}

const go = new Go();
// instantiateStreaming needs Content-Type: application/wasm; if a host serves
// the wrong MIME it hard-fails, so fall back to a plain fetch+arrayBuffer.
async function instantiate() {
  try {
    return await WebAssembly.instantiateStreaming(fetch('__NB_WASM__'), go.importObject);
  } catch (_) {
    const bytes = await (await fetch('__NB_WASM__')).arrayBuffer();
    return await WebAssembly.instantiate(bytes, go.importObject);
  }
}
instantiate().then((r) => {
  go.run(r.instance);
  // The port is published synchronously by RunNotebook once go.run starts the
  // program; poll briefly for it, then build once.
  const wait = setInterval(() => {
    if (globalThis.notebook) { clearInterval(wait); start(globalThis.notebook); }
  }, 5);
}).catch((e) => { status.textContent = 'wasm load failed: ' + e; });`,
})
