# go-notebook JS/TS client

The optional JavaScript/TypeScript client for a go-notebook WASM notebook. It is
the JS sibling of the Go [`nb`](../nb) package: **a page never has to import it.**
The whole host-facing surface is already one plain-data object,
`globalThis.notebook` (published by the WASM build — see
[`engine/wasm`](../engine/wasm)), and you can call it directly. Import this
client and you trade a few lines of hand-rolled glue for editor autocomplete, a
structural view of the notebook's shape, and typed value events.

There is **no build step and no toolchain**. `notebook.js` is pure ESM with JSDoc
types; it runs unmodified in a browser (`<script type=module>`) and in Node, and
`notebook.d.ts` gives TypeScript first-class types. This mirrors the project's
importless ethos — nothing here is compiled, bundled, or published to npm.

## Use

```html
<script type="module">
  import { connect } from "./notebook.js";

  const nb = connect(); // wraps globalThis.notebook

  // Structural: what can I drive, and how is it wired?
  nb.leaves();  // [{ symbol, cell, label, kind, columns }, ...]
  nb.graph();   // { cell -> [upstream producer cells] }

  // Typed values out — what a program computes on (numbers, not "40.24"):
  nb.subscribeValues((ev) => console.log(ev.cell, "=", ev.value));

  nb.start();          // run the first wave
  nb.set("c", 40);     // edit a leaf; downstream recomputes
</script>
```

## What "structural" means

The client reads the graph, leaf symbols, widget kinds, and table columns from
`notebook.meta` — enough to **enumerate and drive** every input and to receive
every output as a typed value. It does **not** know each leaf's Go scalar type: a
`set("c", 40)` is not compile-time checked against `c`'s `int`. That needs a
per-leaf type tag the port does not yet publish (a deliberately deferred
feature). You get the shape, not the schema.

## Verify / example

[`example.mjs`](./example.mjs) drives the client end-to-end against a real built
notebook, with zero dependencies:

```sh
notebook build --target=wasm -o /tmp/nb examples/capacity
cp client/notebook.js /tmp/nb/notebook-client.js
GOROOT=$(go env GOROOT)
PATH="$GOROOT/lib/wasm:$PATH" node client/example.mjs /tmp/nb
```

It enumerates the leaves, subscribes to typed values, runs the first wave, sets a
leaf, and prints the recomputed typed values.
