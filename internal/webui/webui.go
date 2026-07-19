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
	b.WriteString(`<footer id="provenance"></footer>` + "\n")
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
  body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; margin: 2rem auto; max-width: 900px; padding: 0 24px; color: var(--ink); }
  .controls { display: grid; grid-template-columns: max-content 1fr max-content; gap: .75rem 1rem; align-items: center; margin-bottom: 1.5rem; }
  .controls label { font-weight: 600; color: var(--navy); }
  .cell { margin: 1rem 0; padding: .5rem 0 .5rem .6rem; border-top: 1px solid #eee;
          border-left: 3px solid transparent; transition: border-color .15s, opacity .15s; }
  /* Cells sharing a //notebook:area=<name> directive lay side by side; each flexes
     equally and clamps to a min width so charts don't crush, wrapping to stacked on
     a narrow viewport. min-width:0 lets a flex child shrink below its content. */
  .cellrow { display: flex; flex-wrap: wrap; gap: 0 1.5rem; align-items: flex-start; }
  .cellrow > .cell { flex: 1 1 320px; min-width: 0; }
  /* Arranged layout (//notebook:layout): a row holds equal-flex columns, each
     stacking its cells (an area's members). A column is a CARD — a subtle framed
     panel — so an arranged notebook reads as a composed dashboard rather than
     loose columns. align-items:stretch gives every card in a row the same height
     (Observable's grid-auto-rows:1fr rhythm). Cards appear ONLY in arranged mode
     (.cellcol is emitted only by buildArranged), so a plain notebook is untouched. */
  .cellrow { align-items: stretch; }
  .cellrow > .cellcol { flex: 1 1 320px; min-width: 0;
    background: #fcfcfd; border: 1px solid var(--line); border-radius: 12px;
    padding: 1rem 1.15rem; }
  /* A full-width row (one column) is a card too; the flex-basis lets it fill. */
  .cellrow > .cellcol:only-child { flex-basis: 100%; }
  /* Inside a card the per-cell top border is redundant with the card frame, and
     a green "done" rail on every stacked readout is visual noise — the card frame
     already carries the grouping. So drop the divider, trim margins, and mute the
     left rail to a hairline in the RESTING states (done/stale); the attention
     states (running amber, error red, blocked) keep their full rail because those
     still carry live meaning worth seeing. */
  .cellcol > .cell { border-top: none; margin: .4rem 0; padding-left: .5rem; }
  .cellcol > .cell:first-child { margin-top: 0; }
  .cellcol > .cell.done, .cellcol > .cell.stale { border-left-color: var(--line); }
  .cellcol > .cell.done .id .dot, .cellcol > .cell.stale .id .dot { background: var(--line); }
  /* The first cell's id line reads as the card's title. */
  .cellcol > .cell:first-child .id { font-size: 12px; }
  /* Controls inside an arranged area stack (label on its own line, then the
     control full-width with its value after it) rather than the top block's
     3-column label|input|value grid, which needs full page width and collapses
     in a ~50% column. The value sits inline after the control via order/flow. */
  .cellcol > .controls { display: block; margin-bottom: 1rem; }
  .cellcol > .controls label { display: block; margin-top: .6rem; }
  .cellcol > .controls input[type=range] { width: 100%; }
  .cellcol > .controls .val { margin-left: .5rem; color: var(--navy); font-weight: 600; }
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
  /* Widget controls — restrained, same palette. */
  select { font: inherit; color: var(--ink); padding: .3rem .5rem; border: 1px solid var(--line);
           border-radius: 7px; background: #fff; }
  select:focus { outline: none; border-color: var(--go); }
  .multi { display: flex; flex-wrap: wrap; gap: .3rem .8rem; }
  .multi .check { font: 13px/1 -apple-system, system-ui, sans-serif; color: var(--ink);
                  display: inline-flex; align-items: center; gap: .25rem; white-space: nowrap; }
  .range2 { display: flex; flex-direction: column; gap: .2rem; }
  table.grid { border-collapse: collapse; font: 12px/1.4 monospace; margin: .3rem 0; }
  table.grid th { text-align: left; color: var(--muted); font-weight: 600; padding: .2rem .5rem; border-bottom: 1px solid var(--line); }
  table.grid td { padding: .1rem .3rem; }
  table.grid td input { width: 8rem; font: inherit; padding: .2rem .35rem; border: 1px solid var(--line); border-radius: 5px; }
  table.grid td input:focus { outline: none; border-color: var(--go); }
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
  /* Provenance — what produced this artifact. Unobtrusive, no network call.
     A path is not a handle; this is the handle, shown. */
  #provenance { margin-top: 2rem; padding-top: .75rem; border-top: 1px solid var(--line);
                font: 11px/1.5 monospace; color: var(--muted); }
  #provenance .dirty { color: var(--err); font-weight: 600; }
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
  const leafCtl = {};       // cell ID -> control (events + render address by cell ID)
  const leafByCell = {};    // leaf symbol -> control (edits + seeding address by symbol)
  const lastEpoch = {};     // cell id -> newest epoch rendered (drop stale waves)
  const STATES = ['running', 'done', 'error', 'blocked', 'stale'];
  let META = [];
  let LAYOUT = null;        // presentation arrangement (rows of area/cell tokens), or null → source order
  let onEdit = function () {};

  function coerce(s) { const n = Number(s); return s.trim() !== '' && !Number.isNaN(n) ? n : s; }
  // coerceScalar turns a text/plain readout ("false", "1200", "hi") into the JS
  // value a control seeds from — so a checkbox reads true/false, not the truthy
  // string "false". bool first (else Number("false") is NaN → string).
  function coerceScalar(s) {
    if (s === 'true') return true;
    if (s === 'false') return false;
    return coerce(s);
  }

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
  // buildControl makes the control for a leaf, dispatched on its static Kind.
  // Returns { el, out, apply(view) } where apply updates the control from a live
  // WidgetView (options/bounds/selection) each wave. emit(selection) is called
  // on user edit with the SELECTION only. Kind is capability-derived at codegen;
  // a directive would refine it, but the kind is the type's own say.
  function buildControl(m, emit, container) {
    const kind = (m.Widget && m.Widget.Kind) || '';
    const out = document.createElement('span'); out.className = 'val';
    const controls = container || document.getElementById('controls');

    if (kind === 'select') {
      const sel = document.createElement('select');
      sel.onchange = () => emit(sel.value);
      controls.append(sel, out);
      return { el: sel, out, apply(v) {
        setOptions(sel, v.options || []);
        if (v.value != null) sel.value = String(v.value);
        out.textContent = sel.value;
      } };
    }
    if (kind === 'multi') {
      const box = document.createElement('div'); box.className = 'multi';
      controls.append(box, out);
      let max = 0;
      const emitSel = () => emit([...box.querySelectorAll('input:checked')].map(c => c.value));
      return { el: box, out, apply(v) {
        max = v.max || 0;
        const sel = new Set((v.value || []).map(String));
        renderChecks(box, v.options || [], sel, emitSel);
        out.textContent = sel.size + (max ? ' / ' + max : '') + ' selected';
      } };
    }
    if (kind === 'range') {
      // Two-handle range: two sliders sharing the data-derived bounds.
      const lo = document.createElement('input'); lo.type = 'range';
      const hi = document.createElement('input'); hi.type = 'range';
      const wrap = document.createElement('div'); wrap.className = 'range2';
      wrap.append(lo, hi);
      const emitSel = () => {
        let a = Number(lo.value), b = Number(hi.value);
        if (a > b) { const t = a; a = b; b = t; }
        out.textContent = a + ' – ' + b;
        emit([a, b]);
      };
      lo.oninput = emitSel; hi.oninput = emitSel;
      controls.append(wrap, out);
      return { el: wrap, out, apply(v) {
        if (v.lo != null) { lo.min = hi.min = v.lo; }
        if (v.hi != null) { lo.max = hi.max = v.hi; }
        const sel = v.value || [];
        if (sel.length === 2) { lo.value = sel[0]; hi.value = sel[1]; out.textContent = sel[0] + ' – ' + sel[1]; }
      } };
    }
    if (kind === 'bool') {
      const cb = document.createElement('input'); cb.type = 'checkbox';
      cb.onchange = () => { out.textContent = cb.checked; emit(cb.checked); };
      controls.append(cb, out);
      return { el: cb, out, apply(v) { cb.checked = !!v.value; out.textContent = cb.checked; },
               seed(val) { cb.checked = !!val; out.textContent = cb.checked; } };
    }
    if (kind === 'table') {
      // A table renders in the cell body (it needs the column schema + rows), not
      // as a compact control; a placeholder here, the grid is built on first view.
      const note = document.createElement('span'); note.className = 'val';
      note.textContent = '(edit rows below)';
      controls.append(note, out);
      return { el: note, out, emit, columns: (m.Widget && m.Widget.Columns) || [], apply() {} };
    }
    if (kind === 'draggable') {
      // A draggable leaf has no compact control: you edit it by dragging its grips
      // on the chart it's drawn in (a foreign cell draws the handles; the runtime
      // writes the leaf). So the strip shows a non-editable note, not a text input —
      // without this it fell through to the scalar rung and rendered a []point as
      // the literal "[object Object]". apply() reports the live point count.
      const note = document.createElement('span'); note.className = 'val';
      note.textContent = '(drag on the chart)';
      controls.append(note, out);
      return { el: note, out, apply(v) {
        const n = Array.isArray(v && v.value) ? v.value.length : 0;
        note.textContent = n ? '(drag on the chart · ' + n + ' points)' : '(drag on the chart)';
      } };
    }
    // Default rung: a scalar. Slider if directives give min/max, else a text box.
    const d = m.Directives || {};
    const input = document.createElement('input');
    const ranged = ('slider' in d) || ('min' in d) || ('max' in d);
    if (ranged) { input.type = 'range'; input.min = d.min ?? 0; input.max = d.max ?? 100; input.step = d.step ?? 1; }
    else input.type = 'text';
    input.oninput = () => { const val = input.type === 'range' ? Number(input.value) : coerce(input.value); out.textContent = input.value; emit(val); };
    controls.append(input, out);
    return { el: input, out, input, apply() {},
             seed(val) { input.value = String(val); out.textContent = String(val); } };
  }

  function setOptions(sel, opts) {
    if (sel.dataset.opts === JSON.stringify(opts)) return; // unchanged
    sel.dataset.opts = JSON.stringify(opts);
    const cur = sel.value;
    sel.innerHTML = '';
    for (const o of opts) { const e = document.createElement('option'); e.value = e.textContent = o; sel.append(e); }
    if (opts.includes(cur)) sel.value = cur;
  }
  function renderChecks(box, opts, selected, onchange) {
    if (box.dataset.opts === JSON.stringify(opts)) {
      // options unchanged: just sync checked state
      for (const cb of box.querySelectorAll('input')) cb.checked = selected.has(cb.value);
      return;
    }
    box.dataset.opts = JSON.stringify(opts);
    box.innerHTML = '';
    for (const o of opts) {
      const lbl = document.createElement('label'); lbl.className = 'check';
      const cb = document.createElement('input'); cb.type = 'checkbox'; cb.value = o; cb.checked = selected.has(o);
      cb.onchange = onchange;
      lbl.append(cb, document.createTextNode(' ' + o));
      box.append(lbl);
    }
  }

  // makeCellEl builds one cell's display element (state rail, error slot, body,
  // optional read-only source). Shared by both the source-order and the arranged
  // layout paths so the two cannot diverge in what a cell looks like.
  function makeCellEl(m) {
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
    cellEls[m.ID] = el;
    return el;
  }

  // registerLeaf wires a leaf cell's control (into the given container) and
  // records it under both addressing keys. Shared by both layout paths.
  function registerLeaf(m, container) {
    const label = document.createElement('label');
    label.textContent = m.Label || m.ID;
    container.append(label);
    const ctl = buildControl(m, (sel) => onEdit(m.Leaf, sel), container);
    ctl.leafSym = m.Leaf;
    leafCtl[m.ID] = ctl;
    leafByCell[m.Leaf] = ctl;
    return ctl;
  }

  function buildControlsAndCells() {
    if (LAYOUT && LAYOUT.length) { buildArranged(); return; }
    const controls = document.getElementById('controls');
    const cells = document.getElementById('cells');
    // With no //notebook:layout block, an //notebook:area=<name> directive lays
    // consecutive same-named cells side by side — the inline grouping shorthand
    // (a "row" is an area shown horizontally). We open a .cellrow flex container
    // on the first cell of a run and append siblings into it; a different (or
    // absent) area value closes it. Grouping only CONSECUTIVE cells keeps two
    // separate area=panels blocks from merging. (When a layout block IS present,
    // buildArranged handles areas globally instead — see above.)
    let rowEl = null, rowName = null;
    const rowOf = (m) => (m.Directives && m.Directives.area) || null;
    const container = (m) => {
      const r = rowOf(m);
      if (r && r === rowName) return rowEl;      // continue the open row
      if (r) {                                    // start a new row
        rowEl = document.createElement('div'); rowEl.className = 'cellrow';
        rowName = r; cells.append(rowEl); return rowEl;
      }
      rowEl = null; rowName = null; return cells; // no row → stack directly
    };
    for (const m of META) {
      if (m.Leaf) {
        const label = document.createElement('label');
        label.textContent = m.Label || m.ID;
        controls.append(label);
        // Dispatch on the leaf's static Kind (from CellMeta). The control edits
        // the SELECTION only; onEdit(leaf, selection) is what crosses /set — never
        // the options or bounds, which the cell recomputes each wave.
        const ctl = buildControl(m, (sel) => onEdit(m.Leaf, sel));
        // Keyed by CELL ID (what events + seeding address); leafSym lets edits
        // and seeding map back to the leaf symbol. A leaf's control is addressed
        // two ways — the cell it lives in (events) and the symbol it writes.
        ctl.leafSym = m.Leaf;
        leafCtl[m.ID] = ctl;
        leafByCell[m.Leaf] = ctl;
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
      container(m).append(el);
      cellEls[m.ID] = el;
    }
  }

  // buildArranged renders the notebook per an explicit LAYOUT (rows of
  // area-or-cell tokens). Every cell is placed into a named area by its
  // //notebook:area directive; a layout token names an AREA if one exists, else a
  // single CELL (so a lone cell needs no area= wrapper). Each layout row is a
  // flex row of equal-width columns (reusing the .cellrow CSS); controls render
  // inside their cell's area — a slider beside the chart it drives — not in a
  // separate top block. Cells named in no placed area append below in source
  // order, so nothing is ever dropped (degrade-to-linear even under a partial
  // layout). Rendering is address-by-ID (cellEls/leafCtl keyed by cell ID), so
  // this reordering never affects event routing.
  function buildArranged() {
    const cells = document.getElementById('cells');
    // The top #controls block is unused in arranged mode; hide it so no empty
    // grid gap shows. Controls live in their areas instead.
    const topControls = document.getElementById('controls');
    if (topControls) topControls.style.display = 'none';

    // Bucket cells by area (in source order within each area).
    const areaOf = (m) => (m.Directives && m.Directives.area) || null;
    const byArea = {};       // area name -> [meta...]
    const placedIDs = new Set();
    for (const m of META) {
      const a = areaOf(m);
      if (a) { (byArea[a] ||= []).push(m); }
    }

    // renderCellInto builds a cell (and its control, if a leaf) into a container.
    // A leaf's control needs the 3-column label|input|value grid (.controls); a
    // column reuses ONE such grid for all its leaves (opened lazily) so several
    // sliders in an area align, exactly as they do in the top block. The cell's
    // display element still goes directly in the column (a leaf's body stays
    // hidden — its control is its view).
    const gridOf = (col) => {
      let g = col.querySelector(':scope > .controls');
      if (!g) { g = document.createElement('div'); g.className = 'controls'; col.appendChild(g); }
      return g;
    };
    const renderCellInto = (m, col) => {
      if (m.Leaf) registerLeaf(m, gridOf(col));
      col.appendChild(makeCellEl(m));
      placedIDs.add(m.ID);
    };
    // renderToken resolves one layout token to a column of cells: an area's
    // members, or a single cell of that ID, or (unknown) a skipped empty column.
    const renderToken = (tok, col) => {
      if (byArea[tok]) { for (const m of byArea[tok]) renderCellInto(m, col); return; }
      const m = META.find(x => x.ID === tok);
      if (m) renderCellInto(m, col);
      // else: a token naming nothing — silently no column, not an error.
    };

    for (const row of LAYOUT) {
      const rowEl = document.createElement('div'); rowEl.className = 'cellrow';
      for (const tok of row) {
        const col = document.createElement('div'); col.className = 'cellcol';
        renderToken(tok, col);
        if (col.childNodes.length) rowEl.appendChild(col);
      }
      if (rowEl.childNodes.length) cells.appendChild(rowEl);
    }

    // Anything not placed by the layout appends below, in source order — the
    // override-not-manifest rule: place what matters, the rest still show. Route
    // through the same column helper so an unplaced leaf still gets the control
    // grid (the top #controls block is hidden in arranged mode).
    let tail = null;
    for (const m of META) {
      if (placedIDs.has(m.ID)) continue;
      if (!tail) { tail = document.createElement('div'); cells.appendChild(tail); }
      renderCellInto(m, tail);
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
    // A widget leaf's live state updates its control (fresh options/bounds after
    // the cell recomputed the schema each wave). The control is the widget's
    // view, so the cell body stays suppressed.
    if (ev.mime === WIDGET_MIME) {
      const ctl = leafCtl[ev.cell];
      if (ctl && ctl.apply) { try { ctl.apply(JSON.parse(ev.data)); } catch (_) {} }
      if (ctl && ctl.columns) renderTable(el, ctl, JSON.parse(ev.data)); // table body
      return;
    }
    // A scalar leaf (a plain bool/number/string — no WidgetView) initializes its
    // control from its OWN event, the same path that updates it. This is why a
    // checkbox shows its real default before any toggle: init and update are one
    // path, not a separate seed channel that can be empty or race (the read-path
    // analogue of the write-path silent default). Do this BEFORE the leaf-body
    // suppression, since the control is the leaf's view.
    if (el.dataset.leaf) {
      const ctl = leafCtl[ev.cell];
      if (ctl && ctl.seed && ev.mime === 'text/plain') ctl.seed(coerceScalar(ev.data));
      return; // the control is the view; never echo the value as a body
    }
    if (!ev.mime) return; // scalar with no view — leave hidden
    el.hidden = false;
    if (ev.mime === 'image/svg+xml' || ev.mime === 'text/html') {
      body.innerHTML = ev.data;
      bindGrips(body); // a Renderable may draw data-grip handles for a foreign leaf
    } else body.textContent = ev.data; // text/plain readout, markdown source — never injected
    afterRender();
  }

  const WIDGET_MIME = 'application/x-notebook-widget+json';

  // bindGrips wires pointer-drag on any data-grip="leaf:index" handle in a
  // rendered SVG. A grip is drawn by a cell that doesn't own its leaf (editor
  // draws handles for the ctrl leaf), so the leaf identity travels in the
  // attribute — the runtime stamped it, the notebook never wrote it. Dragging
  // emits the WHOLE point set for that leaf as the selection (continuous, one
  // Set per move; the scheduler already coalesces rapid waves), so the drag goes
  // through the same Head.Set chokepoint as a slider — the renderer reads, the
  // runtime writes, no cycle.
  function bindGrips(body) {
    const svg = body.querySelector('svg');
    if (!svg) return;
    const handles = [...svg.querySelectorAll('[data-grip]')];
    if (!handles.length) return;
    // Group handles by leaf; the drag emits that leaf's full [{x,y}] set.
    const byLeaf = {};
    for (const h of handles) {
      const [leaf, idx] = h.getAttribute('data-grip').split(':');
      (byLeaf[leaf] ||= []).push({ el: h, i: +idx });
    }
    for (const leaf in byLeaf) {
      const grips = byLeaf[leaf].sort((a, b) => a.i - b.i);
      // Emit a FLAT [x0,y0,x1,y1,...] — the coercer homogenizes to []float64 and
      // the notebook's Reconcile pairs them back into its own point type (the
      // label/value gap again: the wire carries primitives, the notebook owns
      // the domain shape).
      const pts = () => grips.flatMap(g => { const p = svgPoint(svg, g.el); return [p.X, p.Y]; });
      for (const g of grips) {
        g.el.style.cursor = 'grab';
        g.el.onpointerdown = (e) => {
          e.preventDefault(); g.el.setPointerCapture(e.pointerId); g.el.style.cursor = 'grabbing';
          const move = (ev2) => {
            const p = clientToSvgData(svg, ev2.clientX, ev2.clientY);
            g.el.setAttribute('cx', svgX(svg, p.x)); g.el.setAttribute('cy', svgY(svg, p.y));
            onEdit(leaf, pts()); // continuous: whole point set, every move
          };
          const up = () => { g.el.style.cursor = 'grab'; g.el.onpointermove = null; g.el.onpointerup = null; };
          g.el.onpointermove = move; g.el.onpointerup = up;
        };
      }
    }
  }
  // The chart draws in a 0..1 data space mapped to the SVG viewBox with a pad;
  // these invert that mapping. Kept simple: pad and range match the notebook's
  // Chart.Render (the demo uses pad=40 over a 720x420 viewBox, data 0..1).
  function svgBox(svg) { const vb = svg.viewBox.baseVal; return vb && vb.width ? vb : { width: 720, height: 420 }; }
  const PAD = 40;
  function svgX(svg, dx) { const b = svgBox(svg); return (PAD + dx * (b.width - 2 * PAD)).toFixed(1); }
  function svgY(svg, dy) { const b = svgBox(svg); return (b.height - PAD - dy * (b.height - 2 * PAD)).toFixed(1); }
  function svgPoint(svg, el) {
    const b = svgBox(svg);
    const cx = +el.getAttribute('cx'), cy = +el.getAttribute('cy');
    return { X: (cx - PAD) / (b.width - 2 * PAD), Y: (b.height - PAD - cy) / (b.height - 2 * PAD) };
  }
  function clientToSvgData(svg, clientX, clientY) {
    const r = svg.getBoundingClientRect(), b = svgBox(svg);
    const vx = (clientX - r.left) / r.width * b.width, vy = (clientY - r.top) / r.height * b.height;
    return { x: (vx - PAD) / (b.width - 2 * PAD), y: (b.height - PAD - vy) / (b.height - 2 * PAD) };
  }

  // renderTable draws a Table leaf's editable grid in its cell body (a table
  // needs its column schema, which a compact control can't hold). Columns come
  // from the static schema; rows from the live value. Editing a cell emits the
  // full row set as the selection.
  function renderTable(el, ctl, view) {
    el.hidden = false;
    const rows = Array.isArray(view.value) ? view.value : [];
    const body = el.querySelector('.body');
    const t = document.createElement('table'); t.className = 'grid';
    const hr = document.createElement('tr');
    for (const c of ctl.columns) { const th = document.createElement('th'); th.textContent = c.Name; hr.append(th); }
    t.append(hr);
    rows.forEach((row, ri) => {
      const tr = document.createElement('tr');
      for (const c of ctl.columns) {
        const td = document.createElement('td');
        const inp = document.createElement('input');
        inp.value = row[c.Name] != null ? row[c.Name] : '';
        inp.oninput = () => {
          rows[ri][c.Name] = c.Type === 'number' ? Number(inp.value) : inp.value;
          ctl.out.textContent = rows.length + ' rows';
          // emit is closed over in buildControl; a table re-emits all rows.
          if (ctl.emit) ctl.emit(rows);
        };
        td.append(inp); tr.append(td);
      }
      t.append(tr);
    });
    body.innerHTML = ''; body.append(t);
    afterRender();
  }

  // afterRender is a hook a transport can override (the wasm iframe reports its
  // height here). Default: nothing.
  let afterRender = function () {};

  // seedLeaves sets each SCALAR control to its leaf's initial value (from a
  // {leaf: value} map), so a slider/checkbox starts at its real position before
  // any edit. Widget leaves (multi/select/range/table) seed themselves from
  // their first WidgetView event instead — their state is richer than a scalar
  // and arrives on the stream. Only controls exposing seed() are scalar.
  function seedLeaves(vals) {
    for (const leaf in vals) {
      const ctl = leafByCell[leaf]; // seeding addresses by leaf symbol
      if (ctl && ctl.seed) ctl.seed(vals[leaf]);
    }
  }

  // showProvenance renders the build identity in the footer: what produced this
  // artifact, unobtrusively, with no network call. A path is not a handle — this
  // shows the handle. Absent fields (a notebook outside a git repo) are simply
  // omitted; the source hash is always present.
  function showProvenance(p) {
    if (!p || !p.sourceHash) return;
    const foot = document.getElementById('provenance');
    if (!foot) return;
    const short = (s) => (s || '').slice(0, 12);
    const parts = ['built from source ' + short(p.sourceHash)];
    if (p.commit) parts.push('commit ' + short(p.commit) + (p.dirty ? ' +dirty' : ''));
    if (p.builtAt) parts.push(p.builtAt);
    if (p.goVersion) parts.push(p.goVersion);
    foot.textContent = parts.join(' · ');
    if (p.dirty) {
      const tag = document.createElement('span');
      tag.className = 'dirty'; tag.textContent = '  (built from uncommitted changes)';
      foot.appendChild(tag);
    }
  }

  // init builds the whole UI from META. opts.onEdit(leaf, value) is called on a
  // control edit; opts.afterRender runs after each render (optional);
  // opts.provenance is the build identity to show in the footer (optional);
  // opts.layout is the presentation arrangement (optional; source order if absent).
  function init(meta, opts) {
    META = meta || [];
    onEdit = (opts && opts.onEdit) || function () {};
    if (opts && opts.afterRender) afterRender = opts.afterRender;
    LAYOUT = (opts && opts.layout) || null;
    buildGraph();
    buildControlsAndCells();
    if (opts && opts.provenance) showProvenance(opts.provenance);
  }

  return { init, render, seedLeaves, showProvenance };
})();
`
