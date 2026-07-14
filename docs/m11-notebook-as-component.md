# M11 — A notebook is a component, not a page

*Framing doc. No code has been written for this milestone. Everything below is a **claim**; the milestone exists to convert claims into evidence.*

---

## Claim vs. evidence (read this first)

**Evidence, today, in the tree:**

- The WASM transport already installs a data-only JS surface: `notebookSet(leaf, value)` in (edits in), `__notebook_event(ev)` (values out), `__notebook_meta` (the graph + labels), `__notebook_leaves` (initial values). See `engine/wasm/wasm.go`. These are plain-data both directions; JS never sees a Go type.
- The default UI already *consumes* that surface through one entry point: `NB.init(meta, {onEdit})` + `NB.render(ev)` in `internal/webui/webui.go`. The transport glue is thin; the client is shared across SSE and WASM.

**Claim, not yet evidence:**

- That this surface is a *host-facing component API* rather than an internal convention. Right now `NB.init` assumes it **owns** the page: it requires `#controls`, `#cells`, and `#graph` to exist and it builds all three. A host page that wants its own layout cannot today drive the notebook *without* mounting our UI. The gap between "the seam exists" and "a stranger's page can drive it" is the whole milestone, and it is unmeasured.
- That bulk data-in (a dataset, not a scalar) survives the same wire. Untested. `set(leaf, 0.7)` is exercised; `set(leaf, <a parquet buffer>)` is not, and may not even typecheck against a compile-time leaf type.
- That spore.host needs zero go-notebook changes. This is a *hypothesis stated in order to be falsified*, not a plan. If it needs changes, the change is the finding.

This doc is unexercised prose. After sixteen rounds of catching *the system did a thing and the thing didn't reach anyone*, a framing doc is the highest-risk place in the project to sound confident. Treat KC16–KC18 as the only things that will ever be allowed to say "this is true."

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
