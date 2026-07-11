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
  body { font: 14px/1.5 -apple-system, system-ui, sans-serif; margin: 2rem; max-width: 820px; color: #1a1a2e; }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .5rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; }
  .cell { margin: 1rem 0; padding: .5rem 0; border-top: 1px solid #eee; }
  .cell.blocked { opacity: .4; }
  .cell .id { font: 11px monospace; color: #888; }
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

// Build a control for each cell whose directives declare a slider.
for (const m of META) {
  const d = m.Directives || {};
  if ('slider' in d) {
    const label = document.createElement('label');
    label.textContent = m.Label || m.ID;
    const input = document.createElement('input');
    input.type = 'range';
    input.min = d.min ?? 0;
    input.max = d.max ?? 100;
    input.step = d.step ?? 1;
    const out = document.createElement('span');
    out.className = 'val';
    input.oninput = () => {
      out.textContent = input.value;
      setLeaf(m.ID, Number(input.value));
    };
    controls.append(label, input, out);
  }
  // A display element for every cell.
  const el = document.createElement('div');
  el.className = 'cell';
  el.innerHTML = '<div class="id">' + m.ID + '</div><div class="body"></div>';
  cells.append(el);
  cellEls[m.ID] = el;
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
  el.classList.toggle('blocked', ev.state === 'blocked' || ev.state === 'error');
  const body = el.querySelector('.body');
  if (ev.state === 'error') { body.textContent = 'error: ' + ev.err; return; }
  if (ev.state === 'blocked') { body.textContent = 'blocked upstream'; return; }
  if (!ev.mime) return;
  if (ev.mime === 'text/markdown') {
    body.textContent = ev.data; // crude: show markdown source
  } else {
    body.innerHTML = ev.data;   // svg and html blobs
  }
}

const es = new EventSource('/events');
es.onmessage = (e) => { try { render(JSON.parse(e.data)); } catch (_) {} };
</script>
</body>
</html>
`
