# M11 — A notebook is a component, not a page

*Framing doc. No code has been written for this milestone. Everything below is a **claim**; the milestone exists to convert claims into evidence.*

---

## Claim vs. evidence (read this first)

**Evidence, today, in the tree:**

- The WASM transport installs a data-only JS surface as **one named object, `globalThis.notebook`** (#148): `notebook.meta` (the graph + labels + leaf symbols), `notebook.provenance` (build identity), `notebook.set(leaf, value)` (edits in), `notebook.subscribe(fn)` → unsubscribe (values out), `notebook.values()` (current snapshot), `notebook.start()` (run the first wave). See `engine/wasm/wasm.go`. These are plain data both directions; JS never sees a Go type. This *is* the component API — the surface a stranger's page holds.
- The default UI *consumes* that port as one consumer among equals: `cmd/notebook/wasm_ui.go` reads `notebook.meta`, wires `notebook.set`/`notebook.subscribe`/`notebook.start`, and feeds the shared client (`NB.init`/`NB.render` in `internal/webui/webui.go`). It holds no privileged surface a foreign page lacks.

**Claim, not yet evidence:**

- That this surface is a *host-facing component API* rather than an internal convention. Right now `NB.init` assumes it **owns** the page: it requires `#controls`, `#cells`, and `#graph` to exist and it builds all three. A host page that wants its own layout cannot today drive the notebook *without* mounting our UI. The gap between "the seam exists" and "a stranger's page can drive it" is the whole milestone, and it is unmeasured.
- That bulk data-in (a dataset, not a scalar) survives the same wire. Untested. `set(leaf, 0.7)` is exercised; `set(leaf, <a parquet buffer>)` is not, and may not even typecheck against a compile-time leaf type.
- That spore.host needs zero go-notebook changes. This is a *hypothesis stated in order to be falsified*, not a plan. If it needs changes, the change is the finding.

This doc is unexercised prose. After sixteen rounds of catching *the system did a thing and the thing didn't reach anyone*, a framing doc is the highest-risk place in the project to sound confident. Treat KC16–KC18 as the only things that will ever be allowed to say "this is true."

---

## Evidence, now (the transport seam, mapped)

*Added after a three-transport code-mapping pass. This converts the structural claims above into evidence with `file:line` pointers. It does not tick a KC — no behavior was observed reaching a new consumer; it establishes what the code already is, and names precisely what is left.*

**The engine spine is ONE contract, and it is constraint-guarded.** All I/O reduces to three engine calls; the engine imports no transport.

| Direction | The one call | Where |
|---|---|---|
| **in** | `Head.Set(leaf, any)` — the single documented mutation chokepoint | `engine/head.go:64` |
| | `Runtime.Set(ctx, leaf, v)` wraps it (bump epoch, cancel stale waves, run wave) | `engine/schedule.go:155` |
| **out** | `Runtime.Subscribe() <-chan Event`; `Event{Epoch,Cell,State,Out *Rendered,Err}` | `engine/schedule.go:130`, `engine/event.go:45` |
| **read** | `Runtime.Finals() map[Symbol]any` (last committed value of every cell) | `engine/schedule.go:277` |
| **coerce** | `CoerceWire(raw)` strips wire encoding; generated `setLeafValue` applies per-leaf static types | `engine/widget.go:108`, `internal/gen/main.go:182` |

The doc comment at `engine/event.go:44` already states the intent verbatim: *"the engine never imports a transport — this channel is the whole seam that keeps headless, WASM, and batch modes free."* The mapping confirms all three transports are adapters over exactly these calls.

### The one-port organizing sentence

> **A notebook's entire interface to the outside world is one bidirectional seam: you `Set` a leaf (data in) and you `Subscribe` to a cell (data out) — the same two calls whether the counterparty is a human at a slider, a program over a socket, a foreign host page, or a batch job, and whether the transport is HTTP, the JS bridge, or a CLI. Rendering (for eyes) and the typed value (for machines) are two projections of the same subscription, exactly as bounds/options/reconcile are two probes of the same leaf.**

This is the compressed answer to the doc's own question ("What is a notebook's interface to the outside world?"). It adds **no second mechanism** and **no component model** (the anti-goal): the "component API" is just *naming and packaging the write/subscribe port that already exists*.

### The honest asterisk: the seam is *stated* three times, not *shared* once

The spine is clean, but three edges re-express it, and two of the three are impoverished:

1. **The wire event shape is defined twice** — `server.wireEvent` struct + `toWire` (`engine/server/server.go:73-94`) vs. an inline `map[string]any` hand-built in `wasm.pump` (`engine/wasm/wasm.go:106-117`). Same `{epoch,cell,state,mime,data,err}` shape, two sources of truth, synced by eye. A third transport would copy it a third time.
2. **Headless input is a weaker parallel path.** `--set` uses a *separate* generated string-parser `setLeaf` (`internal/gen/main.go:147`) that writes `head.Set` **directly** — bypassing `Runtime.Set`, epoch/wave, and `CoerceWire` — and its `kindOther` rung writes a raw **string**, so a composite/widget/bulk leaf gets a value its `Reconcile` rejects. The browser's `setLeafValue` path (which *does* call `CoerceWire`) has no such limit. Headless is the one place a leaf write scatters off the chokepoint.
3. **Output is bimodal and asymmetric.** Browsers get *rendered* output (`Rendered{MIME,Data}` from `doneEvent`, `engine/schedule.go:482`) — for eyes. Headless gets *raw values* (`{provenance,values}` of `Finals()`, `internal/gen/main.go:338`) — for machines, one-shot, not a stream. **There is no typed value a program can subscribe to** as it changes.

**Root cause (why this is coherent to fix, not ad hoc):** the project discovers *inputs* by capability probing — `Bounded`, `Optioned`, `Reconciler` (§4.3), "adding a kind means adding a probe, never editing a switch." *Outputs* have exactly one capability, `Renderable{ Render() Rendered }`, which only serves eyes. The value-out gap is not a transport bug — it is that **outputs were never given the capability symmetry inputs have.** `recordFinals` (`engine/schedule.go:267`) already captures every cell's typed value in the same wave that emits the `Event`; the value and its rendering are computed together, then delivered on two different roads.

### The finishing work — all additive to the engine, ranked (SQ1 first)

1. **One wire serializer** (pure refactor → SQ1/KC16 precondition). Collapse the duplicated shape into a single `engine`-owned `WireEvent`/`ToWire` both `server` and `wasm` import. Anti-pass: the SSE frame and the JS event object must be **byte-identical** before/after, or it changed behavior. (Deliberate, name-it-don't-hide-it decision: this puts a wire-format concern in `engine` — defensible because the shape is transport-*agnostic*, and `engine` already imports `encoding/json`; it does **not** import `net/http`, so the foreclosure-table constraint holds.)
2. **`Value any` on `Event`** (additive field → the one genuinely-new surface, value-out for machines). Mirrors how `Grips` was added to `Rendered`. **Opt-in only:** `Subscribe()` stays rendered-only and wire-safe (the default the browser transports use); a new `SubscribeValues()` populates `Value` for **in-process** consumers, so no transport is forced to marshal an arbitrary Go value. `Finals()` becomes a fold over that stream, not a separate road. Anti-pass: for a scalar cell, `Event.Value` must equal its `Finals()` entry and its rendered `Data` must agree — if they diverge, the two roads have forked. A value only crosses a wire by explicit projection (the `WidgetView`/`CoerceWire` discipline, in the out direction).
3. **Unify headless input** (refactor → SQ2/KC17, KC18). Route `--set` through the same `CoerceWire`/`Runtime.Set`/`setLeafValue` the browser uses (CLI values arrive as strings → parse to a JSON value first, then reuse the existing coercer). Deletes the weak `setLeaf`; removes the one scattered write; lets `--set leaf=<json>` carry composite/table/handle shapes. Anti-pass: an uncoercible shape must **fail loud** (CoerceWire's existing contract), never silently set a raw string.

### What CANNOT stay additive (findings, named not hidden)

- **F1 — KC16 needs a structural cut in `internal/webui`.** The wire-serializer collapse gives a host *a shape to read*, but `NB.init` still **requires and builds** `#controls`/`#cells`/`#graph` (`internal/webui/webui.go`) and owns the epoch guard and seeding. For a foreign page to drive the notebook with `NB` *absent from the bundle* (KC16's pass; its anti-pass rejects "NB still mounted"), the `{set, subscribe, graph}` surface must be extractable **below** `NB` — a new small layer, not a field addition. Additive to the *engine*; **structural to `internal/webui`.** Its difficulty *is* the SQ1 measurement. Sequence it last, own design pass.
- **F2 — bulk-in is free as a HANDLE, not as a PAYLOAD.** `CoerceWire`'s vocabulary is a *closed* set that fails on shapes it doesn't cover. A `Rel[T]` handle decodes as `map[string]any`, which `coerceMap` already accepts — so `set(dataLeaf, handleJSON)` **composes for free** (the strong SQ2 result: "we already built this"). A raw `[]byte` parquet *payload* is **not** in the vocabulary; accepting it needs a deliberate vocabulary addition or a new bulk transport (which KC17's anti-pass calls a finding, not a win). And the handle's *bytes* still live behind the wasm sandbox with no filesystem — identity travels, contents may not. **The SQ2 answer: bulk-in is additive only via a content-addressed handle, which constrains the author to declare a handle leaf.**
- **F3 — push-out is excluded (anti-goal).** A notebook publishing to a webhook/queue would put the impure edge *inside* a cell. Keep the rule: the transport owns the impure boundary; a cell is subscribed/pulled, never pushes.

**Sequence (no big bang):** rank 1 → F1 spike (drive a bare host page with `NB` removed; whatever breaks *is* the SQ1 measurement) → rank 3 → SQ2 experiment (`set(dataLeaf, RelHandleJSON)`, no rebuild, assert downstream recompute) → rank 2 (build the typed-value surface only when a program-consumer KC actually needs live values, not speculatively). Each step converts one cluster of claims to evidence and is independently revertible. **No KC ticks until observed end-to-end against a real consumer.**

### Progress (claims converted to evidence)

*Appended as each step lands. This is the ledger; the reasoning above is left intact as the record of how the shape was found.*

- **Rank 1 — DONE (#146).** One `engine.WireEvent`/`ToWire`; server and wasm share it. Byte-parity tests freeze the shape.
- **Rank 3 — DONE (#147).** `--set` routes through `CoerceWire`/`setLeafValue`; the weak `setLeaf` is deleted; composite leaves set from the CLI; uncoercible/unknown/wrong-kind fail loud. The "headless input is a weaker parallel path" asterisk is closed.
- **F1 — REFUTED, then NAMED (#99 spike, #148).** The spike drove `capacity` from a bare host page with `NB` absent and *no* `#controls`/`#cells`/`#graph`, and it worked: the port touches no DOM, so no structural cut in `internal/webui` was needed. `NB.init` owns the DOM only for *our* consumer; the port sits below it already. **KC16 is observed** — a foreign page holds `globalThis.notebook` (`{meta, provenance, set, subscribe, values, start}`) and drives compute; `webui.NB` is one consumer of that port (`cmd/notebook/wasm_ui.go`), not privileged. The old six ad-hoc globals collapsed into the one named object; the seed-ordering ritual dissolved — the value channel *is* the subscription, and `notebook.values()` is its pull form. Two spike findings on the OUT side remain open: values arrive as rendered strings (rank 2), and the seed race is gone but the OUT typed-value gap is not.
- **Rank 2 — not started.** The typed value-out surface; open until a program-consumer KC needs live values.
- **F2 / SQ2 — not started.** The handle-over-the-wire experiment.

---

## The one question, three hats

"Designerly," data-in/out, and spore.host are not three questions. They are one:

> **What is a notebook's interface to the outside world?**

Today a WASM notebook is a **page**: it owns an `index.html`, mounts our UI, renders a footer. That is right for the landing page and wrong for everything else. The shape to interrogate:

> **A notebook exposes a small surface — set a leaf, subscribe to a cell, query the graph — and the *host* decides what to draw. Our default UI becomes one consumer of that surface, not a privileged one.**

If that holds, three things collapse into one:

- **"Designerly"** stops meaning "our CSS is nicer" and starts meaning *the host owns layout and typography; the notebook is the compute engine behind it.*
- **Data in/out** is the same surface. `set(leaf, value)` **is** data-in. Subscribing to a cell **is** data-out. There is no second mechanism to design.
- **spore.host** is the compute tier under the same contract — a notebook driven remotely by the same set/subscribe protocol, over a wire instead of in-process.

This is framable **now and not a month ago** because the widget vocabulary (M10) exists: you could not design "what it means to set a leaf" before knowing what a leaf *is* for a `Multi`, a grip, or a `Table`. Now you do. That dependency is real and it is why this is M11, not M6.

---

## The three sub-questions (one issue each)

### SQ1 — Is the JS surface a component API, and is our UI just a consumer of it?

The seam exists as transport glue; it is not *packaged* as an API a host can hold. Concretely: `NB.init` builds `#controls`/`#cells`/`#graph` itself. A host that wants only the compute — its own chart, its own layout — has no way in that doesn't drag our UI along.

The question is a location question as much as a design one: **is this a change to `internal/webui`, or a new layer beneath it?** The honest hypothesis: `webui.NB` becomes *a* consumer sitting on top of a smaller, documented `set` / `subscribe(cell, cb)` / `graph()` object, and that object — not `NB` — is what a host imports. Do not assume the split is clean; the current entanglement (`NB` owning the DOM and the epoch-guard and the seeding) is evidence it might not be.

### SQ2 — Does bulk data-in cross the wire without a rebuild?

`set(leaf, 0.7)` is easy. Handing a running WASM notebook a whole dataset — a CSV, a parquet buffer — without a rebuild is not obviously the same mechanism. Two things to settle:

1. **Is a dataset just a leaf whose value is bulk data?** A leaf has a compile-time Go type. A `[]byte` leaf or a `Rel[T]` leaf is a legal shape; a `float64` leaf is not going to accept a parquet buffer. So "bulk data-in" may require the notebook to *declare* a bulk-input leaf, which is a design constraint on the author, not a free property.
2. **`Rel[T]` is already a content-addressed handle.** Does *handle-over-the-wire* fall out for free — you `set(dataLeaf, handle)` and the notebook reads the contents itself — or does it break because the handle's contents live on the far side of the WASM sandbox with no filesystem? This is the sharp edge: `Rel` was designed so identity travels with the value, which is exactly the property a wire needs. Either it composes and the finding is "we already built this," or it doesn't and the finding names precisely what the sandbox can't reach.

### SQ3 — Where is the spore.host seam?

spore.host is Scott's ephemeral-compute project (Go, importable, Python surface, TypeScript considered). The honest hypothesis to **test, not assume**:

> **go-notebook may need zero changes.** spore.host builds the notebook for linux, spawns the binary, tunnels the port, reaps it.

If that is true, the finding is *"the seam already exists and it is `notebook build`"* — a better result than a feature, because it means the compute-tier story was already paid for by the transport-independence constraint. If it is false, the deliverable is **the exact list of what is missing** — a specific missing flag, a missing lifecycle hook, a port-ownership mismatch (cf. the run-loop port-ownership finding: a child that owns its own port is a structural hole). Any non-empty diff to `engine/` or `cmd/` is a non-zero answer and *is* the finding.

---

## KC16–KC18 — the scoreboard (sharpened, not accepted)

These are the only claims in this doc that get to become true. They are behavioral, not numeric — deliberately, because inventing a millisecond target here would be the KC4 error (a guessed number reasoned from as evidence). Each has a **pass** and an explicit **anti-pass** (the way a demo could masquerade as a result).

**KC16 — a foreign host page drives a notebook it did not render.**
- *Pass:* a host page with its **own** layout and typography calls `set` / `subscribe` / `graph`, and our default UI (`webui.NB`) is **not in the bundle at all** — not hidden with CSS, absent.
- *Anti-pass:* it "works" only because `NB` is still mounted and we styled it to look custom. If the notebook can't be driven with our UI *removed*, KC16 has not passed — it has proved the entanglement, which is a finding, not a win.

**KC17 — a dataset crosses into a running WASM notebook without a rebuild, and downstream cells recompute.**
- *Pass:* a dataset enters a **pre-existing** leaf of the appropriate type via the same `set` surface, with **no rebuild** and **no new engine public API**, and the transitive downstream recomputes.
- *Anti-pass:* it required a new bulk transport, a new engine method, or a rebuild to pick up the data. That is not failure — it is the finding, and it must be *named* (which API, which limit), never quietly folded into "done."

**KC18 — spore.host spawns a notebook, drives it, and reaps it.**
- *Pass:* spore.host builds, spawns, drives (set/subscribe over the wire), and reaps a notebook binary.
- *Report, either way:* **did go-notebook need any change?** A non-empty diff to `engine/` or `cmd/` is a non-zero result. If the answer is "zero," that is the strongest possible outcome and must be stated as such. If non-zero, the diff is the deliverable.

**All three are unexercised.** None has been run. Do not tick a box until the behavior is observed end-to-end against a real host / real dataset / real spore.host — not a mock, not a test that asserts the shape we hoped for. Verify the instrument against a known-good before trusting a green.

---

## Sequencing

SQ1 is the spine — SQ2 and SQ3 both assume a set/subscribe surface a stranger can hold. Frame all three; build SQ1 first. Do **not** start M11 from this doc: the milestone is framed, not opened. `Prev[Viewport]` (#28) is unrelated and stays where it is.
