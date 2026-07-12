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
  :root { --navy: #1b3a6b; --go: #00add8; --ink: #1a1a2e; --muted: #5b6472; --line: #e7ebf0; }
  body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; margin: 2rem; max-width: 820px; color: var(--ink); }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .75rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; color: var(--navy); }
  .cell { margin: 1rem 0; padding: .5rem 0; border-top: 1px solid var(--line); }
  .cell.blocked { opacity: .4; }
  .cell .id { font: 11px monospace; color: var(--muted); }
  .val { font-variant-numeric: tabular-nums; color: var(--navy); font-weight: 600; }
  input[type=text] {
    font: inherit; color: var(--ink); padding: .3rem .5rem;
    border: 1px solid var(--line); border-radius: 7px; background: #fff;
  }
  input[type=text]:focus { outline: none; border-color: var(--go); }

  /* Restrained custom range — the OS default accent is too loud. Thin neutral
     track, white thumb ringed in the Go accent; palette matches the site. */
  input[type=range] {
    -webkit-appearance: none; appearance: none;
    width: 100%%; height: 22px; background: transparent; cursor: pointer;
  }
  input[type=range]:focus { outline: none; }
  input[type=range]::-webkit-slider-runnable-track {
    height: 4px; border-radius: 2px; background: var(--line);
  }
  input[type=range]::-moz-range-track {
    height: 4px; border-radius: 2px; background: var(--line);
  }
  input[type=range]::-webkit-slider-thumb {
    -webkit-appearance: none; appearance: none; margin-top: -7px;
    width: 18px; height: 18px; border-radius: 50%%;
    background: #fff; border: 2px solid var(--go);
    box-shadow: 0 1px 2px rgba(20,30,60,.18); transition: border-color .12s, box-shadow .12s;
  }
  input[type=range]::-moz-range-thumb {
    width: 18px; height: 18px; border-radius: 50%%;
    background: #fff; border: 2px solid var(--go);
    box-shadow: 0 1px 2px rgba(20,30,60,.18); transition: border-color .12s, box-shadow .12s;
  }
  input[type=range]:hover::-webkit-slider-thumb { border-color: var(--navy); }
  input[type=range]:hover::-moz-range-thumb { border-color: var(--navy); }
  input[type=range]:active::-webkit-slider-thumb,
  input[type=range]:focus::-webkit-slider-thumb { box-shadow: 0 0 0 4px rgba(0,173,216,.18); }
  input[type=range]:active::-moz-range-thumb,
  input[type=range]:focus::-moz-range-thumb { box-shadow: 0 0 0 4px rgba(0,173,216,.18); }
  #status { font: 12px monospace; color: var(--muted); }
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

// Highest epoch rendered per cell. Each slider drag spawns a new wave; a slower
// older wave can emit its result AFTER a newer one, which would snap the chart
// back to a stale value. The engine already stamps every event with its epoch —
// so we render an event only if it is at least as new as the last one shown for
// that cell. Monotonic per cell; the newest wave always wins.
const lastEpoch = {};

// The wasm transport calls this per cell update.
globalThis.__notebook_event = (ev) => {
  const el = cellEls[ev.cell];
  if (!el) return;
  const seen = lastEpoch[ev.cell];
  if (seen !== undefined && ev.epoch < seen) return; // stale wave, drop it
  lastEpoch[ev.cell] = ev.epoch;
  el.classList.toggle('blocked', ev.state === 'blocked' || ev.state === 'error');
  const body = el.querySelector('.body');
  if (ev.state === 'error') { body.textContent = 'error: ' + (ev.err||''); return; }
  if (ev.state === 'blocked') { body.textContent = 'blocked upstream'; return; }
  if (!ev.mime) return;
  if (ev.mime === 'text/markdown') body.textContent = ev.data;
  else body.innerHTML = ev.data;
  reportHeight();
};

// When embedded in an iframe (the landing page), tell the parent how tall the
// content is so it can size the frame — a rendered chart can be taller than any
// fixed guess. No-op when opened directly.
function reportHeight() {
  if (window.parent === window) return;
  const h = Math.ceil(document.documentElement.scrollHeight);
  window.parent.postMessage({ type: 'notebook:height', height: h }, '*');
}

// Leaf control refs, keyed by leaf symbol, so we can seed initial values once
// the engine reports them (__notebook_leaves).
const leafCtl = {};

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
      leafCtl[m.Leaf] = { input, out };
    }
    const el = document.createElement('div');
    el.className = 'cell';
    el.innerHTML = '<div class="id">'+m.ID+'</div><div class="body"></div>';
    cells.append(el);
    cellEls[m.ID] = el;
  }
  status.textContent = 'ready — compiled Go, no server';
  reportHeight();
  // UI is built and cell elements exist — now trigger the initial wave, so its
  // first render lands in a DOM element that's already there (no drop race).
  globalThis.notebookStart();
}
window.addEventListener('resize', reportHeight);
function coerce(s){ const n=Number(s); return s.trim()!=='' && !Number.isNaN(n) ? n : s; }

// Seed each control with its leaf's initial value, so a slider starts at its
// real position and the readout is populated before any drag. The engine
// publishes these after the first wave; it may call us before or after that, so
// we both install the callback and try once immediately.
function seedLeaves() {
  const raw = globalThis.__notebook_leaves;
  if (!raw) return;
  const vals = JSON.parse(raw);
  for (const leaf in vals) {
    const ctl = leafCtl[leaf];
    if (!ctl) continue;
    const s = String(vals[leaf]);
    ctl.input.value = s;
    ctl.out.textContent = s;
  }
}
globalThis.__notebook_leaves_ready = seedLeaves;

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
  // Wait for the wasm main to publish metadata + set functions, then build UI.
  const wait = setInterval(() => {
    if (globalThis.__notebook_ready) { clearInterval(wait); buildUI(); }
  }, 5);
}).catch((e) => { status.textContent = 'wasm load failed: ' + e; });
</script>
</body>
</html>
`
