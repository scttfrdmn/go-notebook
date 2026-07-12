// Package webui holds the browser client that is shared by both transports —
// the SSE server (engine/server) and the WASM host (cmd/notebook). It is the
// "one thing" behind "one file, two transports": the CSS, the control/cell
// builder, the dependency-graph view, and the event renderer live here once, so
// the two clients cannot drift into showing different notebooks.
//
// It is pure data (Go string constants) with no imports, so importing it
// creates no dependency cycle and does not pull net/http into anything. Each
// client supplies only its transport glue: how META arrives, how an edit is
// sent, and how events are delivered.
//
// The client is deliberately ignorant of Go types. It reads cell metadata
// (labels, directives, dependency edges, source) to build controls, a graph,
// and cell displays; it renders {cell, state, mime, data, epoch, err} events;
// it reports edits as {leaf, value}. Renderers run in Go, in-process — the
// client only paints their MIME-tagged output.
package webui

import "strings"

// PageOpts parameterize the shared HTML shell for a transport. Title is the
// <title> and the <h1>; Subtitle is optional muted text after the <h1> (the
// wasm host uses "· running in your browser, no server"). Status, when true,
// adds a <div id="status"> line (the wasm bootstrap writes into it). Glue is the
// transport-specific <script> body appended after the shared client JS — it
// calls NB.init(...) and wires events. Head is optional extra <head> markup
// (e.g. the wasm host's <script src="wasm_exec.js">, placed before the client).
type PageOpts struct {
	Title     string
	Subtitle  string
	Status    bool
	HeadExtra string
	BodyPre   string // extra markup injected before the shared body (e.g. wasm status line)
	Glue      string // transport <script> body, appended after the shared JS
}

// Page assembles the complete notebook HTML for a transport. It owns the shell —
// <head> with the shared CSS, the graph/controls/cells body, the shared client
// JS — so no transport hand-rolls the page. A transport supplies only its glue
// (how META arrives, how edits are sent, how events are delivered). This is the
// presentation that used to live as a const inside engine/server; moving it here
// lets that package go back to being a transport that serves what it's handed.
func Page(opts PageOpts) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<title>")
	b.WriteString(opts.Title)
	b.WriteString("</title>\n<style>")
	b.WriteString(CSS)
	b.WriteString("</style>\n")
	b.WriteString(opts.HeadExtra)
	b.WriteString("\n</head>\n<body>\n<h1>")
	b.WriteString(opts.Title)
	if opts.Subtitle != "" {
		b.WriteString(` <span style="font-weight:400;color:#888">`)
		b.WriteString(opts.Subtitle)
		b.WriteString("</span>")
	}
	b.WriteString("</h1>\n")
	if opts.Status {
		b.WriteString(`<div id="status">loading…</div>` + "\n")
	}
	b.WriteString(opts.BodyPre)
	b.WriteString(`<div class="graph" id="graph"></div>` + "\n")
	b.WriteString(`<div class="controls" id="controls"></div>` + "\n")
	b.WriteString(`<div id="cells"></div>` + "\n")
	b.WriteString("<script>")
	b.WriteString(JS)
	b.WriteString("\n")
	b.WriteString(opts.Glue)
	b.WriteString("\n</script>\n</body>\n</html>\n")
	return b.String()
}

// CSS is the shared stylesheet: palette, controls + custom slider, cell state
// rail, the read-only source disclosure, and the dependency graph. Both clients
// embed it verbatim. It contains literal % (none currently) — callers that
// Sprintf their page must escape as needed; the server uses string.Replace and
// the wasm host uses indexed verbs, so this string is inserted, not formatted.
const CSS = `
  :root { --navy:#1b3a6b; --go:#00add8; --ink:#1a1a2e; --muted:#5b6472; --line:#e7ebf0;
          --run:#f0a020; --err:#d0433b; --stale:#b8c0cc; --done:#3fa845; }
  body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; margin: 2rem; max-width: 820px; color: var(--ink); }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .75rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; color: var(--navy); }
  .cell { margin: 1rem 0; padding: .5rem 0 .5rem .6rem; border-top: 1px solid #eee;
          border-left: 3px solid transparent; transition: border-color .15s, opacity .15s; }
  .cell.blocked { opacity: .4; }
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
  .val { font-variant-numeric: tabular-nums; color: var(--navy); font-weight: 600; }
  input[type=text] { font: inherit; color: var(--ink); padding: .3rem .5rem;
    border: 1px solid var(--line); border-radius: 7px; background: #fff; }
  input[type=text]:focus { outline: none; border-color: var(--go); }
  input[type=range] { -webkit-appearance: none; appearance: none;
    width: 100%; height: 22px; background: transparent; cursor: pointer; }
  input[type=range]:focus { outline: none; }
  input[type=range]::-webkit-slider-runnable-track { height: 4px; border-radius: 2px; background: var(--line); }
  input[type=range]::-moz-range-track { height: 4px; border-radius: 2px; background: var(--line); }
  input[type=range]::-webkit-slider-thumb { -webkit-appearance: none; appearance: none; margin-top: -7px;
    width: 18px; height: 18px; border-radius: 50%; background: #fff; border: 2px solid var(--go);
    box-shadow: 0 1px 2px rgba(20,30,60,.18); transition: border-color .12s, box-shadow .12s; }
  input[type=range]::-moz-range-thumb { width: 18px; height: 18px; border-radius: 50%;
    background: #fff; border: 2px solid var(--go); box-shadow: 0 1px 2px rgba(20,30,60,.18);
    transition: border-color .12s, box-shadow .12s; }
  input[type=range]:hover::-webkit-slider-thumb { border-color: var(--navy); }
  input[type=range]:hover::-moz-range-thumb { border-color: var(--navy); }
  input[type=range]:active::-webkit-slider-thumb, input[type=range]:focus::-webkit-slider-thumb { box-shadow: 0 0 0 4px rgba(0,173,216,.18); }
  input[type=range]:active::-moz-range-thumb, input[type=range]:focus::-moz-range-thumb { box-shadow: 0 0 0 4px rgba(0,173,216,.18); }
  #status { font: 12px monospace; color: var(--muted); }
  .cell details { margin-top: .4rem; }
  .cell details summary { font: 11px monospace; color: var(--muted); cursor: pointer; list-style: none; }
  .cell details summary::-webkit-details-marker { display: none; }
  .cell details summary::before { content: '\25b8 source'; }
  .cell details[open] summary::before { content: '\25be source'; }
  .cell pre.src { margin: .4rem 0 0; padding: .7rem .9rem; background: #0f1524; color: #e6ebf5;
                  border-radius: 8px; font: 12px/1.5 monospace; overflow-x: auto; }
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
`

// JS is the shared render engine. It defines a global NB object with the whole
// client behavior; a transport then calls NB.init(META, {onEdit}) once and
// NB.render(ev) per event. Everything the two clients had in common — controls
// with the degradation ladder, the dependency graph, the five-state rail, the
// read-only source disclosure, the epoch-monotonic guard, error/blocked
// display — lives here exactly once.
//
// The page must contain #controls, #cells, and #graph elements before init.
const JS = `
const NB = (function () {
  const cellEls = {};       // cell id -> its .cell element
  const graphNodes = {};    // cell id -> its <g> in the graph
  const leafCtl = {};       // leaf symbol -> {input, out} for seeding values
  const lastEpoch = {};     // cell id -> newest epoch rendered (drop stale waves)
  const STATES = ['running', 'done', 'error', 'blocked', 'stale'];
  let META = [];
  let onEdit = function () {};

  function coerce(s) { const n = Number(s); return s.trim() !== '' && !Number.isNaN(n) ? n : s; }

  // setState applies exactly one wave-state class (plus the blocked dim), so the
  // left rail / dot / graph node reflect the latest transition.
  function setState(el, state) {
    el.classList.remove(...STATES);
    if (STATES.includes(state)) el.classList.add(state);
    el.classList.toggle('blocked', state === 'blocked');
  }

  // buildControlsAndCells builds a control for every leaf (leaf-ness is decided
  // by the analyzer from the type; the directive only refines the control) and a
  // display element for every cell, with an optional read-only source
  // disclosure. A leaf's control IS its view, so its cell body is suppressed.
  function buildControlsAndCells() {
    const controls = document.getElementById('controls');
    const cells = document.getElementById('cells');
    for (const m of META) {
      if (m.Leaf) {
        const d = m.Directives || {};
        const label = document.createElement('label');
        label.textContent = m.Label || m.ID;
        const out = document.createElement('span'); out.className = 'val';
        const input = document.createElement('input');
        const ranged = ('slider' in d) || ('min' in d) || ('max' in d);
        if (ranged) { input.type = 'range'; input.min = d.min ?? 0; input.max = d.max ?? 100; input.step = d.step ?? 1; }
        else input.type = 'text';
        input.oninput = () => {
          const v = input.type === 'range' ? Number(input.value) : coerce(input.value);
          out.textContent = input.value;
          onEdit(m.Leaf, v);
        };
        controls.append(label, input, out);
        leafCtl[m.Leaf] = { input, out };
      }
      const el = document.createElement('div');
      el.className = 'cell';
      el.hidden = true; // revealed by the first event that has something to show
      el.innerHTML = '<div class="id"><span class="dot"></span>' + m.ID +
                     '</div><div class="err" hidden></div><div class="body"></div>';
      if (m.Leaf) el.dataset.leaf = '1'; // control is its view; don't echo the body
      if (m.Source) {
        const det = document.createElement('details');
        const sum = document.createElement('summary');
        const pre = document.createElement('pre'); pre.className = 'src';
        pre.textContent = m.Source; // code, set as text — never injected as HTML
        det.append(sum, pre); el.append(det);
      }
      cells.append(el);
      cellEls[m.ID] = el;
    }
  }

  // buildGraph draws the dependency graph from META[].In: cells in columns by
  // dependency depth (longest path from a source), edges as curves. Each node is
  // a <g> recolored on state — the graph IS the notebook, so it's worth drawing.
  function buildGraph() {
    const host = document.getElementById('graph');
    if (!host) return;
    const byId = {}; META.forEach(m => byId[m.ID] = m);
    const depth = {}; const inFlight = {};
    function depthOf(id) {
      if (depth[id] !== undefined) return depth[id];
      if (inFlight[id]) return 0;
      inFlight[id] = true;
      let d = 0;
      for (const p of ((byId[id] && byId[id].In) || [])) d = Math.max(d, depthOf(p) + 1);
      inFlight[id] = false;
      return depth[id] = d;
    }
    META.forEach(m => depthOf(m.ID));
    const cols = {};
    META.forEach(m => { (cols[depth[m.ID]] ||= []).push(m.ID); });
    const colKeys = Object.keys(cols).map(Number).sort((a, b) => a - b);
    const NW = 128, NH = 30, GX = 60, GY = 16, PAD = 16;
    const pos = {}; let maxRows = 0;
    colKeys.forEach((c, ci) => {
      cols[c].forEach((id, ri) => { pos[id] = { x: PAD + ci*(NW+GX), y: PAD + ri*(NH+GY) }; });
      maxRows = Math.max(maxRows, cols[c].length);
    });
    const W = PAD*2 + colKeys.length*NW + (colKeys.length-1)*GX;
    const H = PAD*2 + maxRows*NH + (maxRows-1)*GY;
    const svgns = 'http://www.w3.org/2000/svg';
    const svg = document.createElementNS(svgns, 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('width', W); svg.setAttribute('height', H);
    META.forEach(m => {
      for (const p of (m.In || [])) {
        if (!pos[p] || !pos[m.ID]) continue;
        const x1 = pos[p].x + NW, y1 = pos[p].y + NH/2;
        const x2 = pos[m.ID].x, y2 = pos[m.ID].y + NH/2;
        const mx = (x1 + x2) / 2;
        const path = document.createElementNS(svgns, 'path');
        path.setAttribute('class', 'edge');
        path.setAttribute('d', 'M'+x1+' '+y1+' C'+mx+' '+y1+' '+mx+' '+y2+' '+x2+' '+y2);
        svg.appendChild(path);
      }
    });
    META.forEach(m => {
      const g = document.createElementNS(svgns, 'g');
      g.setAttribute('class', 'node' + (m.Leaf ? ' leaf' : ''));
      g.setAttribute('transform', 'translate('+pos[m.ID].x+','+pos[m.ID].y+')');
      const rect = document.createElementNS(svgns, 'rect');
      rect.setAttribute('width', NW); rect.setAttribute('height', NH);
      const text = document.createElementNS(svgns, 'text');
      text.setAttribute('x', 8); text.setAttribute('y', NH/2 + 4);
      text.textContent = m.ID.length > 16 ? m.ID.slice(0, 15) + '…' : m.ID;
      g.appendChild(rect); g.appendChild(text);
      svg.appendChild(g);
      graphNodes[m.ID] = g;
    });
    host.appendChild(svg);
  }

  // render applies one cell event: state on the cell and its graph node, then
  // the body / error per the display ladder. Stale waves (older epoch) drop.
  function render(ev) {
    const el = cellEls[ev.cell];
    if (!el) return;
    const seen = lastEpoch[ev.cell];
    if (seen !== undefined && ev.epoch < seen) return; // superseded wave
    lastEpoch[ev.cell] = ev.epoch;
    setState(el, ev.state);
    if (graphNodes[ev.cell]) setState(graphNodes[ev.cell], ev.state);
    const body = el.querySelector('.body');
    const errEl = el.querySelector('.err');
    // Reveal a cell only when it has something to show (a rendered value, an
    // error, or a blocked notice); a leaf (control is its view) and a scalar
    // with no output stay hidden, so the display is never a wall of empty
    // labels. Errors and blocked states are always shown.
    if (ev.state === 'error') { el.hidden = false; errEl.textContent = 'error: ' + (ev.err || '(no message)'); errEl.hidden = false; afterRender(); return; }
    if (ev.state === 'blocked') { el.hidden = false; errEl.textContent = 'blocked — an upstream cell failed'; errEl.hidden = false; afterRender(); return; }
    if (ev.state === 'running' || ev.state === 'stale') return; // no new output
    errEl.hidden = true; errEl.textContent = '';
    if (el.dataset.leaf) return; // a leaf's control IS its view; don't echo the body
    if (!ev.mime) return; // scalar with no view — leave hidden
    el.hidden = false;
    if (ev.mime === 'image/svg+xml' || ev.mime === 'text/html') body.innerHTML = ev.data;
    else body.textContent = ev.data; // text/plain readout, markdown source — never injected
    afterRender();
  }

  // afterRender is a hook a transport can override (the wasm iframe reports its
  // height here). Default: nothing.
  let afterRender = function () {};

  // seedLeaves sets each control to its leaf's initial value (from a {leaf:
  // value} map), so a slider starts at its real position and the readout is
  // populated before any drag.
  function seedLeaves(vals) {
    for (const leaf in vals) {
      const ctl = leafCtl[leaf];
      if (!ctl) continue;
      const s = String(vals[leaf]);
      ctl.input.value = s; ctl.out.textContent = s;
    }
  }

  // init builds the whole UI from META. opts.onEdit(leaf, value) is called on a
  // control edit; opts.afterRender runs after each render (optional).
  function init(meta, opts) {
    META = meta || [];
    onEdit = (opts && opts.onEdit) || function () {};
    if (opts && opts.afterRender) afterRender = opts.afterRender;
    buildGraph();
    buildControlsAndCells();
  }

  return { init, render, seedLeaves };
})();
`
