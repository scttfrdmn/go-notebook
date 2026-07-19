// app.js — the host page's own logic. It is deliberately NOT the notebook's UI:
// it never calls NB.init, never assumes #controls/#cells/#graph exist, and draws
// entirely into its own DOM (see index.html). Its only contact with the notebook
// is through the JS client, connect(), which wraps globalThis.notebook.
//
// The point of the page: a second, independent computation that runs on the
// notebook's TYPED value stream. hourlyCost used to reach a host only as rendered
// text ("40.24"); via subscribeValues it arrives as a JS number, so this page can
// do real arithmetic on it (annualize it) instead of concatenating a string.
import { connect } from "./notebook.js";

const status = document.getElementById("status");
const HOURS_PER_YEAR = 24 * 365; // 8760

const els = {
  slider: document.getElementById("servers"),
  serversNum: document.getElementById("serversNum"),
  hourly: document.getElementById("hourly"),
  typechip: document.getElementById("typechip"),
  annual: document.getElementById("annual"),
  why: document.getElementById("why"),
};

const money = (n) =>
  "$" + n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// paint runs whenever a typed hourlyCost value arrives. The type check is the
// anti-pass made visible: we assert we received a number, not a string, and show
// what typeof actually said. Then we do arithmetic that would be nonsense on a
// string (× 8760) — the proof that the typed pipe delivered a real value.
function paint(hourlyCost) {
  const kind = typeof hourlyCost; // "number" if the typed pipe worked
  els.typechip.textContent = kind;
  els.typechip.className = "chip " + (kind === "number" ? "ok" : "bad");

  if (kind !== "number") {
    els.hourly.textContent = String(hourlyCost);
    els.annual.textContent = "—";
    els.why.textContent = "not a number — arithmetic would be a string concat, not a sum.";
    return;
  }
  els.hourly.innerHTML = money(hourlyCost) + " <small>/hr</small>";
  const annual = hourlyCost * HOURS_PER_YEAR;
  els.annual.textContent = money(annual);
  els.why.textContent =
    `${money(hourlyCost)} × ${HOURS_PER_YEAR} = ${money(annual)} — a real sum, because the value is a number.`;
}

function start(port) {
  const nb = connect(port);

  // subscribeValues is the whole reason this page exists. If the wasm predates it
  // (an older build), say so plainly rather than silently degrading.
  let unsub;
  try {
    unsub = nb.subscribeValues((ev) => {
      if (ev.cell === "hourlyCost") paint(ev.value);
    });
  } catch (e) {
    status.textContent = "this notebook's port has no subscribeValues (rebuild with a newer go-notebook)";
    return;
  }

  // The host draws its own control and drives the leaf. servers() returns the
  // named result `c`, so the leaf symbol is "c" (not "servers").
  els.slider.disabled = false;
  els.slider.addEventListener("input", () => {
    const c = Number(els.slider.value);
    els.serversNum.textContent = c;
    nb.set("c", c); // downstream hourlyCost recomputes; the value arrives via subscribeValues
  });

  nb.start(); // run the first wave so hourlyCost paints its default
  status.textContent =
    "live — this page is a host, not the notebook's UI. Drag the slider; the right panel recomputes on the typed stream.";

  window.addEventListener("pagehide", () => unsub && unsub());
}

// Load the wasm ourselves — a host page controls its own page lifecycle.
const go = new Go();
async function instantiate() {
  try {
    return await WebAssembly.instantiateStreaming(fetch("notebook.wasm"), go.importObject);
  } catch (_) {
    const bytes = await (await fetch("notebook.wasm")).arrayBuffer();
    return await WebAssembly.instantiate(bytes, go.importObject);
  }
}
instantiate()
  .then((r) => {
    go.run(r.instance);
    // RunNotebook publishes globalThis.notebook synchronously once go.run starts;
    // poll briefly for it, then wire up once.
    const wait = setInterval(() => {
      if (globalThis.notebook) {
        clearInterval(wait);
        start(globalThis.notebook);
      }
    }, 5);
  })
  .catch((e) => {
    status.textContent = "wasm load failed: " + e;
  });
