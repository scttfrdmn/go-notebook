package main

// indexHTMLWASM is the browser host for a --target=wasm notebook. It loads the
// Go wasm runtime shim, instantiates notebook.wasm, then drives the SAME
// JS contract the wasm transport exposes: read __notebook_meta for controls,
// receive cell updates via __notebook_event, and edit leaves via notebookSet.
// There is no server — the compiled engine runs in the page. The %s is the
// notebook name.
//
// Deliberately dependency-free and minimal: the point is to prove the topology,
// not to build a UI. `%%` escapes a literal percent for the single Sprintf.
const indexHTMLWASM = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>%[1]s — go-notebook (wasm)</title>
<style>
  body { font: 14px/1.5 -apple-system, system-ui, sans-serif; margin: 2rem; max-width: 820px; color: #1a1a2e; }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .5rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; }
  .cell { margin: 1rem 0; padding: .5rem 0; border-top: 1px solid #eee; }
  .cell.blocked { opacity: .4; }
  .cell .id { font: 11px monospace; color: #888; }
  .val { font-variant-numeric: tabular-nums; }
  input[type=range] { width: 100%%; }
  #status { font: 12px monospace; color: #888; }
</style>
</head>
<body>
<h1>%[1]s <span style="font-weight:400;color:#888">· running in your browser, no server</span></h1>
<div id="status">loading wasm…</div>
<div class="controls" id="controls"></div>
<div id="cells"></div>
<script src="wasm_exec.js"></script>
<script>
const cells = document.getElementById('cells');
const controls = document.getElementById('controls');
const status = document.getElementById('status');
const cellEls = {};

// The wasm transport calls this per cell update.
globalThis.__notebook_event = (ev) => {
  const el = cellEls[ev.cell];
  if (!el) return;
  el.classList.toggle('blocked', ev.state === 'blocked' || ev.state === 'error');
  const body = el.querySelector('.body');
  if (ev.state === 'error') { body.textContent = 'error: ' + (ev.err||''); return; }
  if (ev.state === 'blocked') { body.textContent = 'blocked upstream'; return; }
  if (!ev.mime) return;
  if (ev.mime === 'text/markdown') body.textContent = ev.data;
  else body.innerHTML = ev.data;
};

function buildUI() {
  const META = JSON.parse(globalThis.__notebook_meta || '[]');
  for (const m of META) {
    if (m.Leaf) {
      const d = m.Directives || {};
      const label = document.createElement('label');
      label.textContent = m.Label || m.ID;
      const out = document.createElement('span'); out.className = 'val';
      const input = document.createElement('input');
      const ranged = ('slider' in d) || ('min' in d) || ('max' in d);
      if (ranged) { input.type='range'; input.min=d.min??0; input.max=d.max??100; input.step=d.step??1; }
      else input.type='text';
      input.oninput = () => {
        const v = input.type==='range' ? Number(input.value) : coerce(input.value);
        out.textContent = input.value;
        globalThis.notebookSet(m.Leaf, v);
      };
      controls.append(label, input, out);
    }
    const el = document.createElement('div');
    el.className = 'cell';
    el.innerHTML = '<div class="id">'+m.ID+'</div><div class="body"></div>';
    cells.append(el);
    cellEls[m.ID] = el;
  }
  status.textContent = 'ready — compiled Go, no server';
  // UI is built and cell elements exist — now trigger the initial wave, so its
  // first render lands in a DOM element that's already there (no drop race).
  globalThis.notebookStart();
}
function coerce(s){ const n=Number(s); return s.trim()!=='' && !Number.isNaN(n) ? n : s; }

const go = new Go();
WebAssembly.instantiateStreaming(fetch('notebook.wasm'), go.importObject).then((r) => {
  go.run(r.instance);
  // Wait for the wasm main to publish metadata + set functions, then build UI.
  const wait = setInterval(() => {
    if (globalThis.__notebook_ready) { clearInterval(wait); buildUI(); }
  }, 5);
}).catch((e) => { status.textContent = 'wasm load failed: ' + e; });
</script>
</body>
</html>
`
