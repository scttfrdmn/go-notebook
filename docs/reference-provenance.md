# Provenance

*Every built artifact records what produced it — the source content, the git state, the build time, and the toolchain — so a figure can be traced back to the exact source that made it. The artifact is immutable; provenance makes its origin legible without a network call.*

## What's recorded

Codegen fills a provenance record at build time; every transport displays it (a footer on a page, a field in the headless JSON). The fields:

| Field | Meaning |
|-------|---------|
| `sourceHash` | content hash of the notebook source — the identity of *what was built*, independent of path or filename. **Always present.** |
| `commit` | the git commit, when the notebook is in a repo |
| `dirty` | `true` if the working tree had uncommitted changes at build time |
| `builtAt` | build time (RFC3339) |
| `goVersion` | the toolchain that compiled the artifact |

All fields except `sourceHash` are best-effort: a notebook outside a git repo is a normal case, so the git fields are simply empty then. The content hash is always there — it is the artifact's true identity.

## In the headless output

`--headless --json` emits provenance alongside the values:

```json
{
  "provenance": {
    "sourceHash": "b6827ee1b90e…",
    "commit": "f7aab67",
    "builtAt": "2026-07-19T05:31:03Z",
    "goVersion": "go1.26.5"
  },
  "values": { "c": 20, "f": 68 }
}
```

## Why it matters

The point of "the same file is a job" ([build & run](reference-build-run.html)) is reproducibility: **the artifact that produced the figure is the artifact you rerun in six months.** Provenance is what lets you check that — a figure carries the source hash and commit that made it, so a stale build announces itself (`dirty`, or a hash that no longer matches your tree) rather than quietly misleading you.

The content-addressed filename of a built `.wasm` (`notebook-<hash>.wasm`) is the same idea applied to the served bytes: a different program gets a different URL, so a page can never be served stale under an unchanged name.
