# Design doc — a notebook as an ephemeral HTTP service

*Status: **PROPOSED**, not built (Scott + Claude, 2026-07-18). This pins the seam between a notebook binary and an ephemeral-compute substrate (spore.host) before either side changes, because the seam spans two repos and "it happens to work" is not the same as "the contract is clean." KC18 (SQ3) is the milestone this serves; it does not tick until observed against a real, billable spawn run.*

---

## The recommendation, up top

Build a **generic readiness/addressing contract** in go-notebook, and a **generic "long-lived HTTP service" workload type** in spore.host. The substrate learns nothing about notebooks; the notebook learns nothing about spore.host. They meet at one line of stdout.

Two small additions to go-notebook, both general (they make the binary a first-class citizen of *any* launcher, not just spawn):

1. **OS-assigned port.** `--addr :0` → the binary calls `net.Listen` itself, learns the real port, then serves. Today `ServeNotebook` uses `http.Server.ListenAndServe`, which hides the listener, so `:0` is useless — you cannot learn the chosen port.
2. **A readiness line on stdout.** When serving begins, print exactly one structured line:
   ```
   {"event":"ready","addr":"127.0.0.1:54321","provenance":{"sourceHash":"…","commit":"…"}}
   ```
   A launcher reads that instead of polling-and-hoping. Language-agnostic, no filesystem coordination, works under any spawner or none (a human sees a harmless JSON line).

On the spore.host side: a `--service` workload type that spawns the binary, reads `{ready, addr}` from stdout, opens an SSH `-L` tunnel to the reported port (spawn already has generic SSH tunneling — `withSSHTunnel`, `sshToInstance -L`), and returns a local URL. It spawns "a thing that serves HTTP and announces readiness," not "a notebook." go-notebook's `/set` + `/events` port (the named port, #148) rides inside that tunnel with zero further coupling.

**Why this over "zero changes."** The [KC18 paper-scope](https://github.com/scttfrdmn/go-notebook/issues/101) established that a notebook can be spawned *today* with zero go-notebook changes — the substrate polls the fixed `--addr` port and hopes it comes up. That works by luck, and it is exactly the child-owns-port smell the run-loop port-ownership finding named: the child binds a port and the parent guesses when it is live. The clean version inverts it — **the substrate owns addressing, the tunnel, and lifecycle; the child announces readiness and never makes the parent guess.** The cost is two small, general additions; the payoff is that the poll-and-hope disappears and the same contract serves any launcher.

---

## Claim vs. evidence

**Evidence, today:**

- The built notebook binary serves `/`, `/set`, `/events`, `/leaves` on `--addr` (default `127.0.0.1:8080`), blocking in `ServeNotebook` via `http.Server.ListenAndServe` (`engine/server/server.go:194`). Reaped by plain process kill; clean exit is exercised by the dev supervisor's intentional-kill path (`cmd/notebook/run.go`).
- The named port (#148) means `/set` (data in) + `/events` (data out) are one documented contract a remote driver can hold over any transport, including a tunnel.
- spore.host's `spawn` has generic SSH tunneling (`withSSHTunnel`, `sshToInstance` with `-L`) and a batch lifecycle (`launch --command --on-complete terminate --ttl`, `spored complete`). Its long-lived-service path (`spawn app` + `spawn:ready-url`) is **DCV-coupled** (89 DCV refs, RDP-port-hardcoded SSM forward) — not a generic HTTP-service primitive.

**Claim, not yet evidence:**

- That `--addr :0` + a stdout readiness line is enough for a substrate to spawn, address, and drive a notebook with no port-guessing. Untested; this doc proposes it.
- That the substrate needs no knowledge of the notebook API. Plausible (the tunnel is byte-transparent; `/set`+`/events` ride inside), but unproven until a real service-type spawn drives a notebook end-to-end.
- That the diff to go-notebook is exactly the two additions above and nothing in `engine/`. The port-report touches `engine/server` (swap `ListenAndServe` for `Listen`+`Serve`); the readiness line touches the generated `mainBody`. Predicted small; unbuilt.

KC18 does not tick from this doc. It ticks when spore.host spawns a notebook, drives it (set/subscribe over the tunnel), and reaps it — observed, billable, supervised.

---

## The seam, precisely

### Who owns what

| Concern | Owner | Why |
|---|---|---|
| Choosing the port | **the notebook** (`:0` → `net.Listen`) | only the process knows what's free on the box |
| Announcing readiness + address | **the notebook** (stdout line) | only the process knows when it is actually serving |
| The tunnel (box port → caller) | **the substrate** | it owns the network path and the credentials |
| Lifecycle (spawn, TTL, reap) | **the substrate** | it owns the billable resource; cost-safety is existential |
| Driving (set/subscribe) | **the caller**, through the tunnel | the notebook is subscribed/pulled, never pushes (F3 anti-goal) |

This is the run-loop port-ownership lesson applied across a machine boundary: the child owns *its* port because only it knows what's free, but it **reports** the port rather than fixing it, so the parent never guesses. The parent owns the externally-visible address (the tunnel endpoint).

### The readiness contract (the one shared thing)

A served workload prints, once, when it begins serving:

```json
{"event":"ready","addr":"<host:port it is listening on>","provenance":{…},"token":"<if --token>"}
```

- **`event`** is a discriminator so a launcher can ignore other stdout lines (log output, warnings). One line, newline-terminated, on stdout.
- **`addr`** is the actual listening address (post-`:0` resolution).
- **`provenance`** is the existing build identity (source hash, commit) — so the substrate can log *what* it spawned, the content-addressed identity a fixed URL can't convey.
- **`token`** is present **only when the notebook was started with `--token`**. When present, the substrate/driver must forward it on every request (an `X-Notebook-Token` header or `?token=`); the field is omitted entirely on the open default, so the line shape is unchanged for a tokenless notebook. This is how a credential rides the same one-line contract without the substrate learning anything notebook-specific — it forwards a token it treats as opaque.

Nothing about notebooks is in this line. Any HTTP workload that prints it is spawnable by the same substrate verb. That is the reusability the generic shape buys — and why it is preferred over teaching spore.host the `/set`+`/events` API directly (which would couple the substrate to go-notebook's surface for no gain: the tunnel already carries that API transparently).

### The two modes, unchanged by this

- **Batch KC18** needs none of the above — `--headless --json` already prints `{provenance, values}` and exits; `spawn launch --command … --on-complete terminate` reaps it. Predicted zero go-notebook changes, confirmed by the paper-scope. The readiness contract is only for the **interactive** (long-lived, set/subscribe) mode.
- **Interactive KC18** is what this doc enables cleanly.

---

## The predicted diffs (both repos)

**go-notebook** (small, general):
- `engine/server`: a `ServeNotebook` variant that `net.Listen`s first (so `:0` resolves) and reports the bound `addr` to its caller — e.g. via a callback or a returned address, so the generated `main` can print the readiness line. Keeps `ListenAndServe`'s behavior otherwise.
- generated `mainBody`: when serving (not `--headless`), print the readiness line once the listener is bound. Behind the existing serve path; no new flag beyond honoring `--addr :0`.
- No `engine/` core change. No new public engine API. The named port is untouched.

**spore.host** (a new generic capability):
- A `--service` (or `spawn serve`) workload type: launch a binary, read `{event:"ready", addr}` from stdout, open an SSH `-L` tunnel to `addr`, print/return the local URL, hold until TTL or explicit terminate, then reap. Reuses the existing SSH-tunnel and lifecycle machinery; adds no DCV dependency.
- Cost-safety unchanged: mandatory TTL, explicit terminate, orphan leak-check (per spore.host's existing discipline).

**Neither side imports or names the other.** The contract is one JSON line and one tunnel.

---

## Alternatives considered

- **Notebook-aware substrate** (spore.host understands `/set`+`/events`, exposes `spawn notebook set leaf=v` / `subscribe` as first-class verbs). More ergonomic for this one use case, but couples the substrate to go-notebook's API for no capability gain — the tunnel already carries that API transparently, and a generic service type serves every other HTTP workload too. Rejected as premature coupling; can be layered *on top* of the generic type later as a thin client if the ergonomics ever justify it.
- **Zero-change / poll-the-fixed-port** (the paper-scope's literal answer). Works, but preserves the child-owns-port poll-and-hope. Fine as the *first* KC18 run to get a green cheaply; not the shape to build toward. This doc is the clean target the first run motivates.
- **Filesystem readiness (`--ready-file`)** instead of stdout. Needs shared-filesystem coordination between child and substrate; stdout is already captured by any spawner and needs no shared mount. Rejected.

---

## Sequence

1. Agree this seam (this doc). ✅
2. **go-notebook: `--addr :0` + readiness line. DONE.** `ServeNotebookReady` (`engine/server/server.go`) binds with `net.Listen` first and reports the resolved address; the generated `main` prints one stdout line `{"event":"ready","addr":"<resolved>","provenance":{…}}` once serving. Verified locally, $0 (`TestServiceReadinessAndDrive`, `cmd/notebook/service_test.go`): the built binary launched with `127.0.0.1:0` reports a real port (never `:0`), and driving `/set` on that reported port takes effect. No EC2.
3. spore.host: the `--service` workload type, tested against the notebook binary locally first (spawn a local process, tunnel over loopback), then once against real EC2 under supervision.
4. KC18 ticks on the real, billable, supervised run — set a leaf over the tunnel, observe a rendered cell change, reap, leak-check.

Step 2 was independently useful and independently revertible: a notebook binary that reports its address and readiness is better under *any* launcher, spore.host or not.
