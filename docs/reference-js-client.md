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

## Each leaf carries its type

Every leaf reports its Go result type, so a program can **validate a value's
shape before it sets it** — without knowing Go. `leaves()[i].type` is
`{ Name, Underlying }`: the type as declared (`"PerHour"`, `"int"`) and the basic
kind it resolves to (`"float64"`, `"int"`, `"bool"`, `"string"`). `Underlying` is
absent for a composite or interface leaf (a table row, a multi-selection), where
no single scalar kind describes the value.

```js
for (const leaf of nb.leaves()) {
  console.log(leaf.symbol, "→", leaf.type?.Name, `(${leaf.type?.Underlying})`);
  // e.g.  c → int (int)   lambda → PerHour (float64)
}

// A shape check the host can run itself, before set():
function okForLeaf(leaf, v) {
  switch (leaf.type?.Underlying) {
    case "int": return Number.isInteger(v);
    case "float64": case "float32": return typeof v === "number";
    case "bool": return typeof v === "boolean";
    case "string": return typeof v === "string";
    default: return true; // composite, or an older port with no type — defer to the coercer
  }
}
```

The type is **readable**, not compile-time enforced: `set("c", "nope")` is still
not rejected by `tsc` — the port's coercer rejects it at runtime, on the far side.
You get the schema to check against, not a generated per-notebook type that fails
your build. (A generated typed `set()` is a separately-gated feature; the shape
you can read today covers the common case of validating input before sending it.)

Two more shapes worth knowing: the metadata keys are PascalCase (`ID`, `Leaf`,
`Label`, `Widget.Kind`, `Type.Name`), and `values()` reports **leaf** cells only —
a derived cell like `hourlyCost` is absent there, so use `subscribeValues` to
observe it.

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
