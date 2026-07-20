# Ports and wire formats

*The exact contract for driving a notebook from outside its own UI — over HTTP
(the served binary) or over the in-page JS object (the WASM build). Everything a
durable driver needs: request bodies, response schemas, the event shape, and what
is and isn't guaranteed.*

A notebook exposes **one port** in two directions: you **set** a leaf (data in)
and you **subscribe** to cells (data out). Every transport is a projection of that
single contract — the same two operations whether the counterparty is a human at
a slider, a feed driver, or another program. See [live feeds](live-feeds.html)
for the pattern this enables and the [JS client](reference-js-client.html) for a
typed wrapper over the browser side.

## HTTP (the served binary: `notebook run`, or a built binary served)

The server binds `127.0.0.1:8080` by default (`--addr` to change it) and serves
four endpoints. **It is an unauthenticated local endpoint** — see
[Security](#security) before exposing it.

### `POST /set` — edit a leaf

Request body:

```json
{"leaf": "c", "value": 40}
```

- `leaf` is the leaf's **result name** (the symbol it produces), not its function
  name — the same name `--set` and the dependency graph use.
- `value` is any JSON value; it is coerced to the leaf's Go type by the same
  coercer a browser edit crosses. A scalar, a bool, a string, an array (a
  multi-select or draggable selection), or an object (a structured/handle leaf)
  are all accepted; numeric values keep int-vs-float via `json.Number`.
- **The edit is validated synchronously; the recompute runs asynchronously.** The
  server coerces the value to the leaf's type before returning, then runs the wave
  in the background — results arrive over `/events`. The status tells a driver
  whether the edit was accepted:
  - **`204 No Content`** — accepted; the wave is running.
  - **`404 Not Found`** — no leaf by that name (a typo'd `leaf`, or the *result*
    symbol of a derived cell rather than an input leaf).
  - **`422 Unprocessable Entity`** — the leaf exists but the value will not coerce
    to its Go type (e.g. a string for a numeric leaf).
  - **`400 Bad Request`** — the body itself is malformed JSON.

  So a mistaken edit fails loud instead of looking successful: a `404`/`422` means
  nothing changed, and you know it at the POST rather than discovering it by the
  cell that never updated. Validation does not wait for the wave — only coercion,
  which is synchronous.

### `GET /events` — subscribe to cell updates (SSE)

A `text/event-stream`. On connect, the server runs a full wave so a freshly
opened client receives every cell's current output (not a blank page). Each event
is one line:

```
data: {"epoch":7,"cell":"hourlyCost","state":"done","mime":"text/plain","data":"40.24"}
```

The JSON payload is the frozen wire-event shape:

| Field | Type | Meaning |
|-------|------|---------|
| `epoch` | number | the wave this event belongs to; monotonic, bumped per edit |
| `cell` | string | the cell id |
| `state` | string | `running` \| `done` \| `error` \| `blocked` \| `stale` |
| `mime` | string | present on `done` with rendered output (`image/svg+xml`, `text/html`, `text/markdown`, `text/plain`) |
| `data` | string | the rendered payload for that MIME; omitted when empty |
| `err` | string | present only on `state: "error"` |

`mime`/`data`/`err` are omitted when empty (`omitempty`), so a bare transition
carries only `epoch`/`cell`/`state`.

### `GET /leaves` — current leaf values

Returns a JSON object of **leaf** cells only (inputs), keyed by result name, for
seeding controls:

```json
{"c": 80, "price": 1.006, "target": 0.2}
```

Derived cells (e.g. `hourlyCost`) are **not** here — observe those on `/events`.
If no wave has run yet, one is run so defaults exist.

### `GET /` — the built-in UI

The HTML page that mounts the default client. A host driving the notebook
programmatically ignores this and uses `/set` + `/events`.

### Guarantees and non-guarantees

- **Epochs supersede.** A newer edit bumps the epoch and cancels older in-flight
  waves; a client should treat the highest epoch seen for a cell as current and
  drop stale lower-epoch events. Rapid edits (a slider drag) coalesce in the
  scheduler.
- **No missed-event replay.** The SSE stream carries no `id:`/`retry:` fields, so
  there is no `Last-Event-ID` resumption. On reconnect the server replays current
  state by running a fresh wave (every cell re-emits its current output) — you get
  the current truth, not the events you missed. A driver should be **state-based**
  (react to the latest value per cell), not event-log-based.
- **Ordering** within a wave is per-cell; across waves, the epoch orders them.

## Browser (the WASM build: `globalThis.notebook`)

A WASM notebook publishes one plain-data object, `globalThis.notebook`, with the
same one-port contract. All values cross as plain JS (numbers, bools, arrays,
objects) — JS never sees a Go type.

```js
notebook.meta                 // CellMeta[] — the graph, labels, leaf symbols, widget kinds, leaf types
notebook.provenance           // build identity (source hash, commit)
notebook.set(leaf, value)     // edit a leaf (data in) — same coercer as POST /set
notebook.subscribe(fn)        // rendered events {epoch,cell,state,mime,data,err} → returns unsubscribe
notebook.subscribeValues(fn)  // TYPED value events {cell, value} → returns unsubscribe
notebook.values()             // synchronous snapshot of every leaf's current value
notebook.start()              // run the first wave, so cells paint their defaults
```

- `subscribe(fn)` delivers the **same wire-event shape** as `/events` (mime/data
  strings — what a human reads). `subscribe` returns an unsubscribe function.
- `subscribeValues(fn)` delivers **typed** values (`{cell, value}` where `value`
  is a real JS number/bool/object) — what a program computes on. See the
  [JS client](reference-js-client.html), which wraps this with types.
- The startup order matters: the port is published during WASM init; wait for
  `globalThis.notebook` to exist, install your subscriber, then call `start()`.

## Security

The built-in server is a **local development and trusted-network endpoint**, not
a multi-user service:

- **No authentication.** Any client that can reach the port may read `/events`
  and mutate **any** leaf via `/set`.
- **No TLS, no CORS restrictions, no per-request rate or size limits** beyond Go's
  defaults.
- It binds `127.0.0.1` by default, so it is not exposed off the machine unless you
  change `--addr` or put it behind something.

Before any network exposure, place authentication and TLS in front of it (a
reverse proxy — nginx, Caddy — terminating TLS and enforcing auth), and treat a
served notebook as a trusted local service. A notebook you did not write should be
treated like any other untrusted program: it is compiled Go with full host access
in the served/native topologies (the WASM topology is sandboxed by the browser).
See also [Rendering is trusted code](reference-rendering.html#rich-output-is-trusted-code).
