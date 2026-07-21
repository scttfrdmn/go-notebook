// example.mjs — drive a built go-notebook WASM notebook with the JS client.
//
// This is the client's anti-pass and its usage example in one file: it runs the
// structural client end-to-end against a REAL notebook, with zero build step and
// zero dependencies (Node's built-in WASM + this repo's client).
//
// Run it against any notebook you have built to wasm (the build copies Go's
// wasm_exec.js beside the .wasm, and this script imports it from there, so no
// GOROOT/PATH setup is needed — plain node):
//
//	notebook build --target=wasm -o /tmp/nb examples/capacity
//	cp client/notebook.js /tmp/nb/notebook-client.js
//	node client/example.mjs /tmp/nb
//
// It enumerates the leaves from meta, reports the port's capabilities, subscribes
// to typed values, runs the first wave, sets a leaf, and ASSERTS the recompute
// produced a typed effect — the whole port surface, through the client, in a real
// JS runtime. Exit code is 0 only if every anti-pass held; non-zero on any
// failure, so CI can gate on it (.github/workflows/ci.yml runs exactly this).

import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

const dir = process.argv[2];
if (!dir) {
  console.error("usage: node example.mjs <dir-with-built-wasm>");
  process.exit(2);
}

// Node's wasm_exec.js ships with Go; the build copies it beside the .wasm.
await import(pathToFileURL(path.join(dir, "wasm_exec.js")).href);
const go = new globalThis.Go();
const wasmFile = fs.readdirSync(dir).find((f) => f.endsWith(".wasm"));
const { instance } = await WebAssembly.instantiate(
  fs.readFileSync(path.join(dir, wasmFile)),
  go.importObject,
);
go.run(instance); // blocks forever inside; publishes globalThis.notebook during init
await new Promise((r) => setTimeout(r, 100));

const { connect } = await import(pathToFileURL(path.join(dir, "notebook-client.js")).href);
const nb = connect();

// Feature-detect what the port supports, derived from the port itself (never a
// hand-maintained list). A current build supports all four; an older .wasm would
// report fewer, and can() lets a host branch without a try/catch.
console.log("capabilities:", nb.capabilities().join(", "));
if (!nb.can("typed-events")) {
  console.error("ANTI-PASS FAIL: a freshly-built notebook does not report the typed-events capability");
  process.exit(1);
}

console.log("leaves:", nb.leaves().map((l) => `${l.symbol}(${l.kind ?? "scalar"})`).join(", "));

// Each leaf now carries its Go result type — the declared name and the basic
// kind it resolves to — so a client can validate a value's SHAPE before set(),
// without knowing Go. Print it, then use it: a tiny shape-checker that would
// reject a wrong-typed value up front (the coercer would reject it anyway, but
// the point of B4b is the client can now see the schema, not just the shape).
console.log("leaf types:", nb.leaves().map((l) => `${l.symbol}:${l.type?.Name ?? "?"}/${l.type?.Underlying ?? "?"}`).join(", "));

const okForLeaf = (leaf, v) => {
  const u = leaf.type?.Underlying;
  if (u === "int") return Number.isInteger(v);
  if (u === "float64" || u === "float32") return typeof v === "number";
  if (u === "bool") return typeof v === "boolean";
  if (u === "string") return typeof v === "string";
  return true; // composite/interface or an older port with no type — defer to the coercer
};

const numericLeaf = nb.leaves().find((l) => l.type?.Underlying === "int" || l.type?.Underlying === "float64");
if (numericLeaf) {
  console.log(
    `shape-check on ${numericLeaf.symbol} (${numericLeaf.type.Underlying}):`,
    `okForLeaf(1)=${okForLeaf(numericLeaf, 1)}`,
    `okForLeaf("nope")=${okForLeaf(numericLeaf, "nope")}`,
  );
  if (okForLeaf(numericLeaf, "nope")) {
    console.error(`ANTI-PASS FAIL: a numeric leaf accepted the string "nope" — Type did not surface a usable schema`);
    process.exit(1);
  }
}

const typed = {};
nb.subscribeValues((ev) => {
  if (!ev.settled && ev.cell !== undefined) typed[ev.cell] = ev.value;
});
// subscribeEpoch delivers a COHERENT snapshot of each wave's values, all at once
// when the wave settles — so a host combining several derived values never mixes
// two waves. We keep the latest snapshot to show it lines up with the stream.
let lastSnapshot = null;
nb.subscribeEpoch((snap) => (lastSnapshot = snap));
nb.start();
await new Promise((r) => setTimeout(r, 300));

console.log("initial typed values:", JSON.stringify(typed));
if (lastSnapshot) console.log(`epoch ${lastSnapshot.epoch} settled with ${Object.keys(lastSnapshot.values).length} coherent values`);

// Set the first scalar leaf to a new number and watch derived values recompute.
// This is the load-bearing anti-pass: driving the port must produce a typed
// effect. We snapshot the values before the edit, set the leaf, and assert some
// value actually changed — a port that silently stopped recomputing (the failure
// this whole file guards) would leave `typed` untouched and must fail CI, not
// print a stale map and exit 0.
const before = JSON.stringify(typed);
const first = nb.leaves()[0];
nb.set(first.symbol, 1);
await new Promise((r) => setTimeout(r, 300));
console.log(`after set(${first.symbol}, 1):`, JSON.stringify(typed));
if (lastSnapshot) console.log(`epoch ${lastSnapshot.epoch} settled with ${Object.keys(lastSnapshot.values).length} coherent values`);

if (JSON.stringify(typed) === before) {
  console.error(`ANTI-PASS FAIL: set(${first.symbol}, 1) changed no typed value — the port did not recompute (or the value stream is dead)`);
  process.exit(1);
}
// The coherent-snapshot path must also have delivered the post-edit wave: its
// epoch advances past the initial wave's, and it carries the whole value set.
if (!lastSnapshot || lastSnapshot.epoch < 1 || Object.keys(lastSnapshot.values).length === 0) {
  console.error("ANTI-PASS FAIL: subscribeEpoch did not deliver a coherent post-edit snapshot");
  process.exit(1);
}
console.log("ok: client drove the port end-to-end (enumerate → subscribe → set → typed recompute → coherent snapshot)");
