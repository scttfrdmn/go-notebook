package server

// metaPlaceholder is replaced with the cell metadata JSON in indexHTML. A
// string replace (not a format verb) is used because the template contains
// literal % (CSS) and { } (JS).
const metaPlaceholder = "/*__META__*/null"

// indexHTML is the single-page client. It is intentionally minimal — the point
// of this milestone is to prove the edit loop is pleasant, not to build a UI.
//
// The client is ignorant of Go: it reads cell metadata (labels, slider
// directives) to build controls, opens an SSE stream for {cell, mime, data}
// updates, and posts {leaf, value} edits. Markdown is rendered crudely; SVG and
// other blobs are inserted as-is.
const indexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>notebook</title>
<style>
  :root { --navy:#1b3a6b; --go:#00add8; --muted:#5b6472; --line:#e7ebf0;
          --run:#f0a020; --err:#d0433b; --stale:#b8c0cc; --done:#3fa845; }
  body { font: 14px/1.5 -apple-system, system-ui, sans-serif; margin: 2rem; max-width: 820px; color: #1a1a2e; }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .5rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; }
  .cell { margin: 1rem 0; padding: .5rem 0 .5rem .6rem; border-top: 1px solid #eee;
          border-left: 3px solid transparent; transition: border-color .15s, opacity .15s; }
  .cell.blocked { opacity: .4; }
  /* Cell state, shown as a left rail + a dot by the id. Every wave paints the
     dirty subgraph: running (amber) → done (green, then fades) → error (red);
     blocked upstream dims; a superseded wave marks stale (grey). */
  .cell.running { border-left-color: var(--run); }
  .cell.error   { border-left-color: var(--err); }
  .cell.done    { border-left-color: var(--done); }
  .cell.stale   { border-left-color: var(--stale); }
  .cell .id { font: 11px monospace; color: #888; }
  .cell .id .dot { display:inline-block; width:7px; height:7px; border-radius:50%;
                   margin-right:5px; background:var(--stale); vertical-align:middle; }
  .cell.running .id .dot { background: var(--run); }
  .cell.done    .id .dot { background: var(--done); }
  .cell.error   .id .dot { background: var(--err); }
  .cell.blocked .id .dot { background: var(--stale); }
  .cell.stale   .id .dot { background: var(--stale); }
  .cell .err { color: var(--err); font: 12px/1.4 monospace; white-space: pre-wrap; }
  .val { font-variant-numeric: tabular-nums; }
  input[type=range] { width: 100%; }
</style>
</head>
<body>
<h1>notebook</h1>
<div class="controls" id="controls"></div>
<div id="cells"></div>
<script>
const META = /*__META__*/null;
const cells = document.getElementById('cells');
const controls = document.getElementById('controls');
const cellEls = {};

// Build a control for EVERY leaf (m.Leaf non-empty). Leaf-ness is decided by
// the analyzer from the type, never a directive — so every input has a control
// even with no //notebook: line. The directive only refines what the control
// looks like: a slider when min/max are given, otherwise a plain field. Delete
// every directive and every control is still here, just plainer. That is the
// degradation ladder.
for (const m of META) {
  if (m.Leaf) {
    const d = m.Directives || {};
    const label = document.createElement('label');
    label.textContent = m.Label || m.ID;
    const out = document.createElement('span');
    out.className = 'val';
    const input = document.createElement('input');
    const ranged = ('slider' in d) || ('min' in d) || ('max' in d);
    if (ranged) {
      input.type = 'range';
      input.min = d.min ?? 0;
      input.max = d.max ?? 100;
      input.step = d.step ?? 1;
    } else {
      // Plainest rung: a text/number field. A bool leaf gets a checkbox.
      input.type = 'text';
    }
    input.oninput = () => {
      const v = input.type === 'range' ? Number(input.value) : coerce(input.value);
      out.textContent = input.value;
      setLeaf(m.Leaf, v);
    };
    controls.append(label, input, out);
  }
  // A display element for every cell. The dot + left rail show its wave state.
  const el = document.createElement('div');
  el.className = 'cell';
  el.innerHTML = '<div class="id"><span class="dot"></span>' + m.ID +
                 '</div><div class="err" hidden></div><div class="body"></div>';
  cells.append(el);
  cellEls[m.ID] = el;
}

// setState applies exactly one wave-state class, so the left rail + dot reflect
// the latest transition (running → done/error, blocked, stale).
const STATES = ['running', 'done', 'error', 'blocked', 'stale'];
function setState(el, state) {
  el.classList.remove(...STATES);
  if (STATES.includes(state)) el.classList.add(state);
  el.classList.toggle('blocked', state === 'blocked');
}

// coerce turns a text-field string into a number when it looks numeric, else
// leaves it a string; the server does the authoritative type coercion.
function coerce(s) {
  const n = Number(s);
  return s.trim() !== '' && !Number.isNaN(n) ? n : s;
}

function setLeaf(leaf, value) {
  fetch('/set', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({leaf, value}),
  });
}

function render(ev) {
  const el = cellEls[ev.cell];
  if (!el) return;
  setState(el, ev.state);
  const body = el.querySelector('.body');
  const errEl = el.querySelector('.err');

  // Runtime error (KC8c): the cell returned a non-nil error. Show what and,
  // via the cell it's attached to, where. Keep the last good body dimmed behind.
  if (ev.state === 'error') {
    errEl.textContent = 'error: ' + (ev.err || '(no message)');
    errEl.hidden = false;
    return;
  }
  // Blocked upstream (KC8d): a producer failed, so this cell did not run.
  if (ev.state === 'blocked') {
    errEl.textContent = 'blocked — an upstream cell failed';
    errEl.hidden = false;
    return;
  }
  // running/stale carry no new output; leave the body as-is, the rail shows it.
  if (ev.state === 'running' || ev.state === 'stale') return;

  // done: a healthy result clears any prior error and updates the body.
  errEl.hidden = true;
  errEl.textContent = '';
  if (!ev.mime) return;
  // Only trusted rich blobs go in as HTML; a text/plain scalar readout and
  // markdown source are set as text, never injected.
  if (ev.mime === 'image/svg+xml' || ev.mime === 'text/html') {
    body.innerHTML = ev.data;
  } else {
    body.textContent = ev.data;
  }
}

const es = new EventSource('/events');
es.onmessage = (e) => { try { render(JSON.parse(e.data)); } catch (_) {} };
</script>
</body>
</html>
`
