# Provenance

*Every built artifact records what produced it — the source content, the git state, the build time, and the toolchain — so a figure can be traced back to the exact source that made it. The artifact is immutable; provenance makes its origin legible without a network call.*

## What's recorded

Codegen fills a provenance record at build time; every transport displays it (a footer on a page, a field in the headless JSON). The fields:

| Field | Meaning |
|-------|---------|
| `sourceHash` | content hash of the notebook **package source** — all its non-generated `.go` files, independent of path or filename. **Always present.** (Scope below.) |
| `commit` | the git commit, when the notebook is in a repo |
| `dirty` | `true` if the working tree had uncommitted changes at build time |
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
rather than a broad one it can't back. The **content-addressed `.wasm` filename**
(`notebook-<hash>.wasm`) is the closest thing to a full artifact identity today,
since it hashes the compiled bytes. Extending the record to the remaining layers
(module graph, and an artifact-bytes hash for native/headless builds) is tracked in
[issue #224](https://github.com/scttfrdmn/go-notebook/issues/224).

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
