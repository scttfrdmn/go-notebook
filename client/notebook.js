// notebook.js — the optional JS/TS client for a go-notebook WASM notebook.
//
// A page never has to import this. The whole host-facing surface is already one
// plain-data object, globalThis.notebook (see engine/wasm), and you can call it
// directly. This module is the other track, the JS sibling of the Go `nb`
// package: import it and you trade a few lines of hand-rolled glue for editor
// autocomplete, a structural view of the notebook's shape, and typed value
// events — without a build step. It is pure ESM with JSDoc types; it runs
// unmodified in a browser (<script type=module>) and in Node, and a hand-written
// notebook.d.ts gives TypeScript first-class types. Delete it and nothing breaks.
//
// It is STRUCTURAL: it reads the graph, leaf symbols, widget kinds, and table
// columns from notebook.meta — enough to enumerate and drive every input and to
// receive every output as a typed value. It does NOT know each leaf's Go scalar
// type (a `set('c', 40)` is not compile-time checked against `c`'s int) — that
// needs a per-leaf type tag the port does not yet publish, and is deliberately
// out of scope. What it gives you is the shape, not the schema.

/**
 * @typedef {Object} WidgetColumn
 * @property {string} Name
 * @property {string} Type
 */

/**
 * @typedef {Object} WidgetMeta
 * @property {"range"|"select"|"multi"|"bool"|"draggable"|"table"|string} Kind
 * @property {WidgetColumn[]} [Columns]
 */

/**
 * @typedef {Object} LeafType   a leaf's Go result type, in two coordinates.
 * @property {string} Name        the type as declared ("PerHour", "int")
 * @property {string} [Underlying] the basic kind it resolves to ("float64", "int", "bool", "string"); absent for a composite/interface leaf
 */

/**
 * @typedef {Object} CellMeta
 * @property {string} ID          the cell id
 * @property {string} [Leaf]      the leaf symbol this cell produces, if it is an input
 * @property {string} [Label]     display label
 * @property {Object<string,string>} [Directives]
 * @property {string[]} [In]      upstream producer cell ids (the graph edges)
 * @property {string} [Source]    verbatim cell source
 * @property {WidgetMeta} [Widget] static control descriptor, present only for leaves
 * @property {LeafType} [Type]    the leaf's Go result type, present only for leaves
 */

/**
 * @typedef {Object} WireEvent
 * @property {number} epoch
 * @property {string} cell
 * @property {string} state   "running" | "done" | "error" | "blocked" | "stale"
 * @property {string} [mime]
 * @property {string} [data]
 * @property {string} [err]
 */

/**
 * @typedef {Object} ValueEvent
 * @property {number} [epoch]   the wave this value belongs to; lets a consumer group a wave's values coherently. Absent on a port that predates it.
 * @property {string} [cell]    the cell whose value this is (absent on a settled marker)
 * @property {*} [value]        the cell's typed value, flattened to a plain JS value (absent on a settled marker)
 * @property {boolean} [settled] true on the terminal wave-settled marker: every value for `epoch` has now been delivered
 */

/**
 * @typedef {Object} NotebookPort  the raw globalThis.notebook object.
 * @property {CellMeta[]} meta
 * @property {*} provenance
 * @property {(string[][]|null)} layout
 * @property {(leaf: string, value: *) => void} set
 * @property {(fn: (ev: WireEvent) => void) => (() => void)} subscribe
 * @property {(fn: (ev: ValueEvent) => void) => (() => void)} [subscribeValues]
 * @property {() => Object<string,*>} values
 * @property {() => void} start
 */

/**
 * A leaf (input) of the notebook, as read from meta.
 * @typedef {Object} Leaf
 * @property {string} symbol   the leaf symbol you pass to set()
 * @property {string} cell     the cell id that produces it
 * @property {string} label
 * @property {(string|null)} kind   the widget kind ("range", "multi", ...) or null for a bare scalar
 * @property {WidgetColumn[]} columns  a table leaf's columns, else []
 * @property {(LeafType|null)} type   the leaf's Go result type ({Name, Underlying}), or null if the port predates it
 */

/**
 * connect wraps a raw notebook port in the structural client. Pass the port
 * explicitly, or omit it to use globalThis.notebook.
 * @param {NotebookPort} [port]
 * @returns {Notebook}
 */
export function connect(port) {
  const p = port ?? /** @type {NotebookPort} */ (globalThis.notebook);
  if (!p || !Array.isArray(p.meta)) {
    throw new Error("go-notebook: no notebook port found (expected globalThis.notebook, or pass one to connect)");
  }
  return new Notebook(p);
}

/** The structural client. Thin, typed, delete-and-nothing-breaks. */
export class Notebook {
  /** @param {NotebookPort} port */
  constructor(port) {
    /** @type {NotebookPort} */
    this.port = port;
  }

  /**
   * The capabilities this notebook's port supports — for feature detection
   * instead of calling a method and catching an exception. Each entry is DERIVED
   * from the port itself, never a hand-maintained list that could claim more than
   * the port delivers:
   *
   *   - `"typed-events"`  — subscribeValues is present (the port hands typed Go
   *                         values, not just rendered strings).
   *   - `"leaf-types"`    — at least one leaf carries its Go result type (so a
   *                         host can validate a value's shape before set()).
   *   - `"wave-settled"`  — the value stream emits a {settled} marker per wave.
   *   - `"epoch-events"`  — value events carry their wave's epoch.
   *
   * The last two are behavioral (a static object can't be probed for them), so
   * they are gated on the one thing that proves the build generation that added
   * them: subscribeValues. They shipped in the same release, so a port that has
   * subscribeValues has all three, and one that doesn't claims none of them —
   * the list stays honest without a version number.
   * @returns {string[]}
   */
  capabilities() {
    const caps = [];
    if (typeof this.port.subscribeValues === "function") {
      caps.push("typed-events", "wave-settled", "epoch-events");
    }
    if (this.port.meta.some((m) => m.Type)) {
      caps.push("leaf-types");
    }
    return caps;
  }

  /**
   * Whether the port supports a named capability — the ergonomic form of
   * [capabilities]. `if (nb.can("typed-events")) …` reads better than a method
   * probe, and unlike a try/catch it does not run the call to find out.
   * @param {string} cap @returns {boolean}
   */
  can(cap) {
    return this.capabilities().includes(cap);
  }

  /** Every cell's metadata. @returns {CellMeta[]} */
  cells() {
    return this.port.meta;
  }

  /**
   * The input leaves — the settable surface. Each carries its widget kind and,
   * for a table, its columns, plus its Go result type ({Name, Underlying}) so a
   * caller can validate a value's shape before set() — without hard-coding leaf
   * names or knowing Go. type is null on a port that predates the field (an
   * older .wasm).
   * @returns {Leaf[]}
   */
  leaves() {
    return this.port.meta
      .filter((m) => m.Leaf)
      .map((m) => ({
        symbol: /** @type {string} */ (m.Leaf),
        cell: m.ID,
        label: m.Label ?? "",
        kind: m.Widget?.Kind ?? null,
        columns: m.Widget?.Columns ?? [],
        type: m.Type ?? null,
      }));
  }

  /**
   * The dependency graph as {cell -> upstream producer cells}, from each cell's
   * In edges. Presentation-only, exactly as the built-in view reads it.
   * @returns {Object<string,string[]>}
   */
  graph() {
    /** @type {Object<string,string[]>} */
    const g = {};
    for (const m of this.port.meta) g[m.ID] = m.In ?? [];
    return g;
  }

  /**
   * Set a leaf. The value crosses the port's coercer exactly as a UI edit does;
   * an unknown leaf or an uncoercible value fails on the far side (this client
   * does not type-check the value against the leaf's Go type — see the module
   * doc).
   * @param {string} leaf @param {*} value
   */
  set(leaf, value) {
    this.port.set(leaf, value);
  }

  /** Pull a snapshot of every leaf's current value. @returns {Object<string,*>} */
  values() {
    return this.port.values();
  }

  /**
   * Subscribe to rendered events (mime/data — what a human reads).
   * @param {(ev: WireEvent) => void} fn @returns {() => void} unsubscribe
   */
  subscribe(fn) {
    return this.port.subscribe(fn);
  }

  /**
   * Subscribe to TYPED value events ({cell, value}) — what a program computes
   * on. Throws if the port predates subscribeValues (an older .wasm); catch it
   * and fall back to subscribe + a readout parse if you must support both.
   * @param {(ev: ValueEvent) => void} fn @returns {() => void} unsubscribe
   */
  subscribeValues(fn) {
    if (typeof this.port.subscribeValues !== "function") {
      throw new Error("go-notebook: this notebook's port has no subscribeValues (rebuild with a newer go-notebook)");
    }
    return this.port.subscribeValues(fn);
  }

  /**
   * Subscribe to COHERENT per-wave value snapshots. This is the convenience over
   * subscribeValues for a host that combines several derived values from one edit:
   * it buffers each wave's typed values and calls fn once, when the wave settles,
   * with { epoch, values } — every cell that updated in that wave, together, never
   * a mix of two waves. Built entirely on the one value stream (the engine emits
   * per-value events plus a terminal {settled} marker per wave); delete it and
   * subscribeValues still works.
   *
   * Values from a superseded wave are dropped: the engine only emits a settled
   * marker for the wave that actually completed, so a buffer for an abandoned
   * epoch is discarded when a newer epoch's values arrive.
   * @param {(snapshot: {epoch: number, values: Object<string,*>}) => void} fn
   * @returns {() => void} unsubscribe
   */
  subscribeEpoch(fn) {
    let epoch = null;
    /** @type {Object<string,*>} */
    let buf = {};
    return this.subscribeValues((ev) => {
      if (ev.settled) {
        if (ev.epoch === epoch) fn({ epoch, values: buf });
        // else: a settled marker for a wave we have no buffer for — ignore.
        epoch = null;
        buf = {};
        return;
      }
      // A value for a newer wave supersedes any half-built older buffer.
      if (ev.epoch !== epoch) {
        epoch = ev.epoch;
        buf = {};
      }
      if (ev.cell !== undefined) buf[ev.cell] = ev.value;
    });
  }

  /** Run the first wave, so cells paint their defaults. */
  start() {
    this.port.start();
  }
}
