# Live feeds: a feed is a driver on the `set` port

*How to wire an external, real-time data source — a sensor, a WebSocket, a polled API — into a notebook. The short version: **a feed is not a cell. A feed is a program that calls the notebook's `set` port as data arrives, and the notebook is pure cells that react.***

---

## Why a feed can't be a cell

Two facts of the design decide this, and they decide it cleanly:

1. **Cells are pure.** A cell is a function of its inputs, recomputed each wave — that purity is what makes scrubbing reversible and the cache sound. A cell that reached out and fetched a URL would be neither.
2. **There are no timers.** `Tick`-clocked folds are a deferred milestone; nothing in a cell fires on a schedule. A cell cannot poll a feed even if it wanted to.

So a feed lives *outside* the graph, which is exactly where the design already puts impurity. The [design doc's F3 rule](https://github.com/scttfrdmn/go-notebook/blob/main/docs/notebook-as-service.md): **the transport owns the impure boundary; a cell is subscribed and pulled, never pushes.** A live feed is the same rule read forwards — the impure edge (the socket, the HTTP call, the clock) belongs to a driver, and the notebook stays a pure function of a leaf whose value the driver writes.

## The pattern

A **feed leaf** is an ordinary input leaf — the same kind a slider drives — except the hand on the knob is a program instead of a mouse:

```go
// The latest reading from a live feed. Its default is 0; a driver process
// overrides it via the set port as data arrives.
//notebook:slider min=0 max=100 step=1
func reading() (v int) { return 0 }

// Everything downstream is pure and reacts to each new value.
func doubled(v int) (d int) { return v * 2 }
```

A **driver** connects to the source and writes each new value through the notebook's one data-in port:

- **served (`notebook run`, or a built binary serving HTTP):** `POST /set {"leaf":"v","value":42}` — coalesced by the scheduler, returns immediately. Derived results stream back on `GET /events`.
- **wasm / in-browser:** `globalThis.notebook.set("v", 42)` in, `notebook.subscribe(fn)` out (the [named port](https://github.com/scttfrdmn/go-notebook/blob/main/docs/notebook-as-service.md)).

That is the whole contract. It is the [one-port concept](https://github.com/scttfrdmn/go-notebook/blob/main/docs/notebook-as-service.md) with a program on the other end: **you `set` a leaf (data in) and `subscribe` to a cell (data out)** — the same two calls whether the counterparty is a human at a slider or a market feed.

```
  ┌────────────┐   POST /set {leaf,value}    ┌──────────────────────┐
  │  driver    │ ──────────────────────────▶ │  notebook (pure cells)│
  │ (the feed) │                             │  reading → doubled → …│
  │  ws/http/  │ ◀────────────────────────── │                       │
  │  sensor    │        GET /events          └──────────────────────┘
  └────────────┘   (derived values stream)
```

Verified end-to-end: a driver POSTing `v=42` to a served notebook recomputes the derived `doubled` cell to `84` on the event stream — leaf set from outside, pure cell reacts, result streams out.

## Two honest notes

- **`/leaves` returns leaf values; `/events` carries everything.** A driver *writes* leaves via `/set`; to observe a *derived* cell it reads `/events` (or `notebook.subscribe`). `/leaves` is only the current leaf positions (for seeding controls), so a driver watching a computed result watches the stream, not `/leaves`.
- **The wasm host has no per-notebook feed-glue hook yet.** In the served topology the driver is just another process hitting `/set` — no framework support needed, and it keeps the feed's impurity in its own process (the F3 boundary, honored). In the browser, `notebook.set` is callable from any page script, but the *default* wasm host page (`cmd/notebook/wasm_ui.go`) wires only our UI; injecting a feed driver into that page is not a first-class option today. **So the natural home for a feed example is the served binary, not the tab.** (A `PageOpts.Glue`-style per-notebook hook for the wasm host is a possible additive feature if in-browser feeds become a need.)

## Worked examples

Four drivers, one pattern, in `examples/`:

- **`sensorfeed`** — a driver samples the host's own CPU/memory and POSTs it every second; the notebook is a pure dashboard (gauges + a rolling window). Keyless, offline, runs anywhere — the machine itself is the feed.
- **`tickerfeed`** — a driver subscribes to a public price WebSocket and pushes each tick; the notebook computes price, a moving average, and the spread, pure.
- **`apifeed`** — a driver polls a keyless public JSON API (USGS earthquakes / ISS position) on an interval; the notebook renders the latest reading and a derived view.
- **`homefeed`** — a bidirectional feed: the driver both pushes live readings *in* and reacts to notebook outputs *out* (a smart-home loop over Matter/Hue), exercising `set` and `subscribe` in both directions.

Each ships the notebook (pure), the driver (`driver/main.go`, the only impure part), and a short README showing `notebook run` + starting the driver. The notebook alone is a static, portable artifact; the driver is what makes it live.
