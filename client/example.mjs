// example.mjs — drive a built go-notebook WASM notebook with the JS client.
//
// This is the client's anti-pass and its usage example in one file: it runs the
// structural client end-to-end against a REAL notebook, with zero build step and
// zero dependencies (Node's built-in WASM + this repo's client).
//
// Run it against any notebook you have built to wasm:
//
//	notebook build --target=wasm -o /tmp/nb examples/capacity
//	cp client/notebook.js /tmp/nb/notebook-client.js
//	GOROOT=$(go env GOROOT)
//	PATH="$GOROOT/lib/wasm:$PATH" node client/example.mjs /tmp/nb
//
// It enumerates the leaves from meta, subscribes to typed values, runs the first
// wave, sets a leaf, and prints the recomputed typed value — the whole port
// surface, through the client, in a real JS runtime. Exit code is 0 on success.

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

console.log("leaves:", nb.leaves().map((l) => `${l.symbol}(${l.kind ?? "scalar"})`).join(", "));

const typed = {};
nb.subscribeValues((ev) => (typed[ev.cell] = ev.value));
nb.start();
await new Promise((r) => setTimeout(r, 300));

console.log("initial typed values:", JSON.stringify(typed));

// Set the first scalar leaf to a new number and watch derived values recompute.
const first = nb.leaves()[0];
nb.set(first.symbol, 1);
await new Promise((r) => setTimeout(r, 300));
console.log(`after set(${first.symbol}, 1):`, JSON.stringify(typed));
