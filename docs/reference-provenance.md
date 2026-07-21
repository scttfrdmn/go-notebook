# Provenance

*Every built artifact records what produced it — the source content, the git state, the build time, and the toolchain — so a figure can be traced back to the exact source that made it. The artifact is immutable; provenance makes its origin legible without a network call.*

## What's recorded

Codegen fills a provenance record at build time; every transport displays it (a footer on a page, a field in the headless JSON). The fields:

| Field | Meaning |
|-------|---------|
| `sourceHash` | content hash of the notebook **package source** — all its non-generated `.go` files, independent of path or filename. **Always present.** (Scope below.) |
| `commit` | the git commit, when the notebook is in a repo |
| `dirty` | `true` if the repository's working tree had **any** uncommitted change at build time — whole-tree, not just the notebook's directory, so a change to a shared helper elsewhere in the repo marks the build dirty (it matches Go's own `vcs.modified`) |
| `builtAt` | build time (RFC3339) |
| `goVersion` | the Go toolchain that compiled the artifact |
| `toolVersion` | the go-notebook toolchain that generated it — codegen changes can change behavior for identical source. Present only for a released tool (a dev build omits it). |

All fields except `sourceHash` are best-effort: a notebook outside a git repo is a normal case, so the git fields are simply empty then, and `toolVersion` is empty for an un-versioned dev build. The source hash is always present.

### What `sourceHash` covers — and what it doesn't

`sourceHash` is the content identity of the **package source and its embedded
data**. It hashes:

- *every* non-generated `.go` file in the notebook's package (not just the file
  carrying `//go:notebook`), so a helper or type in a sibling `.go` changes the hash;
- *every* file baked in with `go:embed`, so a dataset compiled into the binary is
  part of its identity — change one byte of an embedded CSV and the hash changes.

Change one character in any `.go` file, or one byte of any embedded asset, and the
hash changes; move or rename the directory and it does not.

It is still deliberately **not** a full build-input identity. It does **not** cover:

- `go.mod` / `go.sum` / imported module versions,
- build tags or compiler flags.

So `sourceHash` answers *"is this the same package source and embedded data?"*, not
*"is this bit-for-bit the same computation?"* — an honest claim about what it hashes
rather than a broad one it can't back.

Two things a reader might expect are deliberately **not** in `sourceHash`, because
each is better served elsewhere:

- **The module graph** (imported third-party versions). Go already records this in
  the native binary itself — `go version -m <binary>` prints every dependency
  version plus the build's own VCS revision and settings. Duplicating it into
  `sourceHash` would add a hard-to-verify surface for no gain over the Go
  toolchain's own record; and it can't be read uniformly (the `.wasm` isn't in a
  format `go version -m` parses). So the module identity is the Go binary's job,
  and `commit`/`dirty` above already pin the repository state at build time.
- **A bit-for-bit artifact hash.** The **content-addressed `.wasm` filename**
  (`notebook-<hash>.wasm`) already is one — it hashes the compiled bytes, so a
  different program gets a different URL. A native binary has no equivalent, and
  adding one would mean a sidecar file (the hash can't live inside the artifact it
  describes) with its own staleness and loss concerns — a new artifact type for a
  guarantee `sourceHash` + `toolVersion` + the Go build record already approximate.

In short: `sourceHash` covers what the notebook *author* controls (source + embedded
data); the Go toolchain's own binary record covers the *build* (modules, VCS,
settings); and the WASM filename covers the *bytes*. Three records, each owned by
whoever is authoritative for it, rather than one hash straining to be all three.

## In the headless output

`--headless --json` emits provenance alongside the values:

```json
{
  "provenance": {
    "sourceHash": "b6827ee1b90e…",
    "commit": "f7aab67",
    "builtAt": "2026-07-19T05:31:03Z",
    "goVersion": "go1.26.5",
    "toolVersion": "v0.5.0"
  },
  "values": { "c": 20, "f": 68 }
}
```

## Why it matters

The point of "the same file is a job" ([build & run](reference-build-run.html)) is reproducibility: **the artifact that produced the figure is the artifact you rerun in six months.** Provenance is what lets you check that — a figure carries the source hash and commit that made it, so a stale build announces itself (`dirty`, or a hash that no longer matches your tree) rather than quietly misleading you.

The content-addressed filename of a built `.wasm` (`notebook-<hash>.wasm`) is the same idea applied to the served bytes: a different program gets a different URL, so a page can never be served stale under an unchanged name.
