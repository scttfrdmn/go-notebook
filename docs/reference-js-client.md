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

  nb.leaves();  // [{ symbol, cell, label, kind, columns, type }, ...] — the settable inputs
  nb.graph();   // { cell -> [upstream producer cells] } — the dependency edges
  nb.cells();   // every cell's metadata (the raw port objects, PascalCase — see "A note on casing")

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

One more shape worth knowing: `values()` reports **leaf** cells only — a derived
cell like `hourlyCost` is absent there, so use `subscribeValues` to observe it.

## A note on casing

There are two naming conventions in play, and it is worth being explicit about
which you are touching. The **raw port** exposes Go wire fields in PascalCase; the
client's `leaves()` **normalizes** the top level to camelCase for ergonomics — but
the nested `type` and `columns` objects are still the raw PascalCase shapes. The
event streams have their own fixed lowercase schemas.

| Surface | Naming | Example |
|---------|--------|---------|
| Raw port — `notebook.meta[i]`, `cells()[i]` | PascalCase Go fields | `meta[0].Widget.Kind`, `meta[0].Type.Name`, `meta[0].In` |
| Raw port — `notebook.values()` | keyed by leaf symbol | `values().c` |
| `leaves()[i]` (client-normalized) | camelCase top level | `leaves()[0].symbol`, `.kind`, `.label` |
| `leaves()[i].type` (nested, raw) | PascalCase | `leaves()[0].type.Name`, `.type.Underlying` |
| `leaves()[i].columns[j]` (nested, raw) | PascalCase | `columns[0].Name`, `columns[0].Type` |
| Rendered event — `subscribe` | fixed lowercase | `{ epoch, cell, state, mime, data, err }` |
| Typed event — `subscribeValues` | fixed lowercase | `{ cell, value }` |

Rule of thumb: if you read it off `notebook.` directly, it is PascalCase; if you
read it off a `leaves()` row's top level, it is camelCase; the nested `type` and
`columns` are the raw PascalCase objects either way. When in doubt, `cells()`
returns the port objects verbatim, so their keys are always the raw Go names.

## The complete host page

The demo above is a full, deployable example — not a snippet. It is four files:

```
component/
├── index.html       your page: its own layout, its own controls
├── app.js           your logic: connect() → subscribe → start
├── notebook.js      the client (copied from client/notebook.js)
├── wasm_exec.js     Go's WASM support shim (copied from the toolchain)
└── notebook.wasm    the built notebook (notebook build --target=wasm)
```

`index.html` and `app.js` are yours; the other three are produced by the build.
Get the runtime files this way — the `.wasm` name is content-addressed, so copy
the one the build emits to a stable name your page fetches:

```sh
notebook build --target=wasm -o ./out examples/capacity
cp ./out/notebook-*.wasm component/notebook.wasm
cp ./out/wasm_exec.js     component/wasm_exec.js
cp client/notebook.js     component/notebook.js
```

The load-and-drive sequence, in `app.js`, has a specific order that matters:

```js
import { connect } from "./notebook.js";

const go = new Go();
// 1. Load the WASM yourself — a host page owns its own page lifecycle.
const src = await WebAssembly.instantiateStreaming(fetch("notebook.wasm"), go.importObject)
  .catch(async () => WebAssembly.instantiate(  // fallback for hosts without the wasm MIME type
    await (await fetch("notebook.wasm")).arrayBuffer(), go.importObject));
go.run(src.instance); // publishes globalThis.notebook synchronously as it starts

// 2. Wait for the port to appear (go.run publishes it as it spins up).
const port = await new Promise((resolve) => {
  const t = setInterval(() => globalThis.notebook && (clearInterval(t), resolve(globalThis.notebook)), 5);
});

const nb = connect(port);

// 3. Subscribe BEFORE start(), so you don't miss the first wave's values.
let unsub;
try {
  unsub = nb.subscribeValues((ev) => { if (ev.cell === "hourlyCost") render(ev.value); });
} catch {
  status("this notebook predates subscribeValues — rebuild with a newer go-notebook");
}

// 4. Wire your own control to a leaf, then run the first wave.
myServersSlider.addEventListener("input", (e) => nb.set("c", Number(e.target.value)));
nb.start();

// 5. Clean up on navigation.
addEventListener("pagehide", () => unsub?.());
```

Deploy it like any static site — it is HTML + JS + a `.wasm`. Serve it over HTTP
(WASM will not load from `file://`), set the `application/wasm` MIME type, and
cache the content-addressed `.wasm` forever. See [publish & deploy](deployment.html)
for the MIME type, caching, and a GitHub Pages / S3 recipe.

The full source of the working demo is in the repo:
[`site/component/index.html`](https://github.com/scttfrdmn/go-notebook/blob/main/site/component/index.html)
and [`site/component/app.js`](https://github.com/scttfrdmn/go-notebook/blob/main/site/component/app.js)
— it draws its own two-panel layout, checks that the value arrived as a `number`
(not the string `"40.24"`), and never touches the notebook's built-in UI.

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
