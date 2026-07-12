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
  /* Read-only source per cell — "a cell is a function," made visible. Not an
     editor: your editor is your editor. */
  .cell details { margin-top: .4rem; }
  .cell details summary { font: 11px monospace; color: var(--muted); cursor: pointer; list-style: none; }
  .cell details summary::-webkit-details-marker { display: none; }
  .cell details summary::before { content: '▸ source'; }
  .cell details[open] summary::before { content: '▾ source'; }
  .cell pre.src { margin: .4rem 0 0; padding: .7rem .9rem; background: #0f1524; color: #e6ebf5;
                  border-radius: 8px; font: 12px/1.5 monospace; overflow-x: auto; }
  /* The dependency graph — the artifact. Which cell feeds which, colored by the
     same live wave state as the cells below. */
  .graph { border: 1px solid var(--line); border-radius: 10px; margin: 0 0 1.5rem; overflow-x: auto; }
  .graph svg { display: block; }
  .graph .node rect { fill: #fff; stroke: var(--stale); stroke-width: 1.5; rx: 6; }
  .graph .node text { font: 11px monospace; fill: #1a1a2e; }
  .graph .node.running rect { stroke: var(--run); stroke-width: 2.5; }
  .graph .node.done    rect { stroke: var(--done); }
  .graph .node.error   rect { stroke: var(--err); stroke-width: 2.5; }
  .graph .node.blocked rect { stroke: var(--stale); stroke-dasharray: 3 3; }
  .graph .node.leaf    rect { fill: #f3f8fc; }
  .graph .edge { stroke: var(--line); stroke-width: 1.5; fill: none; }
</style>
</head>
<body>
<h1>notebook</h1>
<div class="graph" id="graph"></div>
<div class="controls" id="controls"></div>
<div id="cells"></div>
<script>
const META = /*__META__*/null;
const cells = document.getElementById('cells');
const controls = document.getElementById('controls');
const cellEls = {};
const graphNodes = {}; // cell id -> its <g class=node> in the graph view

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
  // Read-only source disclosure (text node, never HTML — it's code, not markup).
  if (m.Source) {
    const det = document.createElement('details');
    const sum = document.createElement('summary');
    const pre = document.createElement('pre');
    pre.className = 'src';
    pre.textContent = m.Source;
    det.append(sum, pre);
    el.append(det);
  }
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

// buildGraph draws the dependency graph from META[].In: cells laid out in
// columns by dependency depth (longest path from a source), edges drawn as
// curves from producer to consumer. Each node is a <g> we recolor on state.
// Pure layout, no library — the graph IS the notebook, so it's worth drawing.
function buildGraph() {
  const byId = {}; META.forEach(m => byId[m.ID] = m);
  // Depth = longest chain of producers (memoized; the graph is acyclic).
  const depth = {}; const inFlight = {};
  function depthOf(id) {
    if (depth[id] !== undefined) return depth[id];
    if (inFlight[id]) return 0; // guard (graph is acyclic, but be safe)
    inFlight[id] = true;
    const ins = (byId[id] && byId[id].In) || [];
    let d = 0;
    for (const p of ins) d = Math.max(d, depthOf(p) + 1);
    inFlight[id] = false;
    return depth[id] = d;
  }
  META.forEach(m => depthOf(m.ID));

  // Group cells by column (depth); position within a column by order.
  const cols = {};
  META.forEach(m => { (cols[depth[m.ID]] ||= []).push(m.ID); });
  const colKeys = Object.keys(cols).map(Number).sort((a,b)=>a-b);
  const NW = 128, NH = 30, GX = 60, GY = 16, PAD = 16;
  const pos = {};
  let maxRows = 0;
  colKeys.forEach((c, ci) => {
    cols[c].forEach((id, ri) => {
      pos[id] = { x: PAD + ci*(NW+GX), y: PAD + ri*(NH+GY) };
    });
    maxRows = Math.max(maxRows, cols[c].length);
  });
  const W = PAD*2 + colKeys.length*NW + (colKeys.length-1)*GX;
  const H = PAD*2 + maxRows*NH + (maxRows-1)*GY;

  const svgns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(svgns, 'svg');
  svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
  svg.setAttribute('width', W); svg.setAttribute('height', H);

  // Edges first (under nodes).
  META.forEach(m => {
    for (const p of (m.In || [])) {
      if (!pos[p] || !pos[m.ID]) continue;
      const x1 = pos[p].x + NW, y1 = pos[p].y + NH/2;
      const x2 = pos[m.ID].x,  y2 = pos[m.ID].y + NH/2;
      const mx = (x1 + x2) / 2;
      const path = document.createElementNS(svgns, 'path');
      path.setAttribute('class', 'edge');
      path.setAttribute('d', 'M'+x1+' '+y1+' C'+mx+' '+y1+' '+mx+' '+y2+' '+x2+' '+y2);
      svg.appendChild(path);
    }
  });
  // Nodes.
  META.forEach(m => {
    const g = document.createElementNS(svgns, 'g');
    g.setAttribute('class', 'node' + (m.Leaf ? ' leaf' : ''));
    g.setAttribute('transform', 'translate('+pos[m.ID].x+','+pos[m.ID].y+')');
    const rect = document.createElementNS(svgns, 'rect');
    rect.setAttribute('width', NW); rect.setAttribute('height', NH);
    const text = document.createElementNS(svgns, 'text');
    text.setAttribute('x', 8); text.setAttribute('y', NH/2 + 4);
    text.textContent = m.ID.length > 16 ? m.ID.slice(0,15) + '…' : m.ID;
    g.appendChild(rect); g.appendChild(text);
    svg.appendChild(g);
    graphNodes[m.ID] = g;
  });
  document.getElementById('graph').appendChild(svg);
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
  if (graphNodes[ev.cell]) setState(graphNodes[ev.cell], ev.state); // same wave state on the graph
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

buildGraph();

const es = new EventSource('/events');
es.onmessage = (e) => { try { render(JSON.parse(e.data)); } catch (_) {} };
</script>
</body>
</html>
`
