# The JS client

A notebook built for the browser (`--target=wasm`) is not a page you can only
*look at* — it is a **component a host page can drive**. Everything a stranger's
page needs is already one plain-data object, `globalThis.notebook`, published by
the WASM build. The JS client is the optional, typed way to hold that object.

You never *have* to import it. `globalThis.notebook` is callable directly. Import
the client and you trade a few lines of hand-rolled glue for editor autocomplete,
a structural view of the notebook's shape, and — the reason it exists — **typed
value events**: a program computes on the numbers a cell produces, not on the
text they render to.

There is **no build step and no toolchain**. `client/notebook.js` is pure ESM
with JSDoc types; it runs unmodified in a browser (`<script type=module>`) and in
Node, and a hand-written `client/notebook.d.ts` gives TypeScript first-class
types. Nothing here is compiled, bundled, or published to npm — the same
importless ethos as the notebooks themselves.

## See it work

The page below is **not** the notebook's built-in UI. It loads the `capacity`
notebook's WASM, then draws *its own* two panels and speaks to the notebook only
through the client. The left control is one this host page rendered itself; the
right panel is a second computation running on the notebook's typed output
stream. Drag the slider — the fleet's hourly cost arrives as a **number**, and
the host annualizes it.

<div class="demoframe"><iframe src="../component/index.html" loading="lazy" title="a host page driving a notebook as a component"></iframe></div>

## Connect

```html
<script type="module">
  import { connect } from "./notebook.js";

  const nb = connect(); // wraps globalThis.notebook; pass a port to override

  nb.leaves();  // [{ symbol, cell, label, kind, columns }, ...] — the settable inputs
  nb.graph();   // { cell -> [upstream producer cells] } — the dependency edges
  nb.cells();   // every cell's metadata

  nb.start();          // run the first wave, so cells paint their defaults
  nb.set("c", 40);     // edit a leaf by its symbol; downstream recomputes
</script>
```

`connect()` throws if no port is found (the script ran before the WASM published
`globalThis.notebook`, or you passed something that is not a notebook port).

## Typed values out — the point

`subscribeValues` delivers each cell's value **as a JavaScript value**, not as the
text it renders to. A scalar that a human sees as `"40.24"` arrives at a program
as the number `40.24`:

```js
nb.subscribeValues((ev) => {
  // ev.cell is the cell id; ev.value is a real JS value (number, bool, object)
  if (ev.cell === "hourlyCost") {
    const annual = ev.value * 24 * 365; // arithmetic, not string concatenation
    console.log("annualized:", annual);
  }
});
```

This is the capability the pipe adds. Before it, a host could only `subscribe` to
**rendered events** — `{ mime, data }`, where `data` is the string a human reads.
Multiplying `"40.24"` by a number is a bug waiting to happen; multiplying `40.24`
is arithmetic. Use `subscribe` when you want exactly what the UI shows (a chart's
SVG, a table's HTML); use `subscribeValues` when a program needs the value.

```js
// Rendered events — what a human reads (mime + data string):
nb.subscribe((ev) => { if (ev.mime === "image/svg+xml") draw(ev.data); });

// A synchronous snapshot of every leaf's current value, instead of subscribing:
nb.values();
```

`subscribeValues` throws on a notebook whose port predates it (an older `.wasm`).
Catch it and fall back to `subscribe` if you must support both.

## Structural, not schema

The client reads the graph, leaf symbols, widget kinds, and table columns from
`notebook.meta` — enough to **enumerate and drive** every input and to receive
every output as a typed value. It does **not** know each leaf's Go scalar type: a
`set("c", 40)` is not compile-time checked against `c`'s `int`. An unknown leaf
or an uncoercible value fails on the far side, not in the client. You get the
shape, not the schema — a per-leaf type tag is a deliberately deferred feature.

Note two shapes worth knowing: the metadata keys are PascalCase (`ID`, `Leaf`,
`Label`, `Widget.Kind`), and `values()` reports **leaf** cells only — a derived
cell like `hourlyCost` is absent there, so use `subscribeValues` to observe it.

## Verify it yourself

[`client/example.mjs`](https://github.com/scttfrdmn/go-notebook/blob/main/client/example.mjs)
drives the client end-to-end against a real built notebook, with zero
dependencies:

```sh
notebook build --target=wasm -o /tmp/nb examples/capacity
cp client/notebook.js /tmp/nb/notebook-client.js
GOROOT=$(go env GOROOT)
PATH="$GOROOT/lib/wasm:$PATH" node client/example.mjs /tmp/nb
```

It enumerates the leaves, subscribes to typed values, runs the first wave, sets a
leaf, and prints the recomputed typed values.
