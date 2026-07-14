# Decision doc — checked SQL, or typed Go ops?

*Status: **DECIDED — SQL typechecker withdrawn** (Scott, 2026-07-13). Not deferred. The rest of this doc is the framing that led to the decision, kept as the record of how the call was made; the decision itself is at the top.*

---

## Decision: withdrawn, not deferred

**Do not build the SQL typechecker.** The reasoning is stronger than "typed Go ops are good enough":

> **The typechecker is not what SQL costs, and it is not what SQL buys.**

A notebook is ordinary Go. A cell can call DuckDB today, unchecked, from a helper — the cgo cost lands on *that* notebook and nobody else pays it. The typechecker adds exactly one thing on top: compile-time checking of the SQL *string*. To get it, the toolchain takes on a dialect-specific SQL parser (the largest single piece of work in the project) and cgo becomes a first-class toolchain concern instead of one notebook's private problem. The trade: a large permanent toolchain burden plus a wound to the static-binary premise, in exchange for checking a string the user can avoid needing checked by writing typed Go.

`taxi` refuted the claim empirically — built as a stopgap, it delivers the headline guarantee (rename a column, every cell that used it fails to compile) with no parser, no cgo, no dialect, because the compiler already does it. Same shape as the grip token and `Table.Reconcile`: designed against a problem, the code showed which problem was real, and the general mechanism already covered it.

Recorded in `design.md` as the sixth correction and as the reversal that closes the cgo wound (static binary intact, all four topologies clean). This was the last piece of unexercised confident prose in the project — *a specification is a claim, and this is the one that was never cashed.*

---

## Claim vs. evidence

**Evidence:** `examples/taxi/taxi.go` demonstrates out-of-core today via `Rel[T]` with typed Go `Open` / `Scan` / filter / group-by — **no SQL, no parser, no cgo, static binary intact**. It runs. `Rel[T]` carries source + row count + content hash, so a changed file changes the handle and invalidates downstream (`Rel.Equal` plugs into the scheduler's pruning ladder). That path is exercised.

**Claim:** everything `docs/design.md` says about a SQL parser+typechecker — "rename a column and every SQL cell fails to compile," three-way struct/query/file agreement, `avg(Fare) → USD`. None of it is built. `design.md:350` calls it "the strongest claim in the design"; `design.md:432` calls it "the single biggest piece of engineering in this document, bigger than the scheduler." Both of those are *claims about a thing that does not exist*. This doc exists because "our strongest claim" and "our biggest unbuilt thing" being the same sentence is precisely when you stop and ask a human.

---

## The question — not "build it," but "should it exist?"

> **What does checked SQL buy that typed Go operations don't?**

`taxi`'s typed-Go path was built as a *stopgap*. The uncomfortable, legitimate possibility this doc must hold open: **the stopgap may be the better answer, and the design doc may be wrong.** Finding that out is a valuable outcome, not a failure to deliver.

### The case for typed Go ops (what `taxi` already proves)

- **Composable** — `Scan`/`Filter`/`GroupBy` are ordinary Go, chained like any other code.
- **Checked by the compiler you already have** — rename a `Trip` field and every op referencing it fails to compile *today*, with zero new toolchain. The headline SQL claim ("rename a column, cells fail to compile") is **already true** for the Go path.
- **Dialect-free** — no DuckDB-vs-SQLite-vs-Postgres surface to track.
- **cgo-free** — `CGO_ENABLED=0` holds, so the one-file `scp`-and-`sbatch` story survives intact. This is the story the whole project leans on (`design.md:364`, "the wound").
- **Zero new toolchain** — no SQL parser, the largest single risk in the doc, never gets built.

### The case for checked SQL (what Go ops can't give)

- **Familiarity** — people know SQL. A researcher reads `SELECT ... GROUP BY` faster than a `Scan` closure. This is a real adoption cost, not nothing.
- **Expressiveness** — window functions, complex multi-table joins, `PARTITION BY`. Expressible in Go, but verbosely; SQL is genuinely denser here.
- **The headline** — "rename a column and every SQL cell fails to compile" is rhetorically powerful *precisely because it's SQL* — the surprise is that a string got typechecked. The Go version is true but unsurprising (of course Go code fails to compile).

**Both lists are real.** Neither obviously wins. That is why this is a conversation and not a plan.

---

## What evidence would settle it

Not opinion — a test that discriminates. Candidates, in rough order of cost:

1. **Take a real query from taxi (or a marimo-gallery notebook) that needs a window function or a two-table join, and write it both ways.** If the Go version is unbearable and the SQL version is clean, that is evidence for SQL. If the Go version is fine, the expressiveness argument evaporates and the case for a parser collapses.
2. **Count the notebooks in the target niche (systems/simulation/cluster, per `design.md:442`) that actually need SQL-shaped queries** vs. those served by scan/filter/aggregate. If the niche rarely joins, the strongest claim is a claim we shouldn't cash.
3. **Prototype the cgo cost honestly** — cross-compile DuckDB via `zig cc` for one target and measure what it does to the "one file, `scp`, `sbatch`" story. `design.md` admits this is "not free"; nobody has measured the price.

---

## What is needed from Scott (the human decision)

- **Is the headline claim load-bearing for the pitch, or is it a nice-to-have?** If the project's story to a user is "compile-checked SQL," killing it changes the pitch. If the story is "one file that runs as a job," the Go path already delivers it and SQL is optional polish. *This is a positioning call, not an engineering one — it cannot be derived from the code.*
- **Is the cgo/static-binary tradeoff acceptable at all?** If a static binary is a hard constraint (it currently reads like one), then DuckDB-via-cgo is disqualified for the portable tiers regardless of how nice the SQL is — and "checked SQL" would mean *pure-Go-parquet SQL only*, a much smaller and different thing than the design doc describes.
- **Whose familiarity are we optimizing for?** The niche is systems/HPC people who write Go anyway. Do *they* want SQL, or is SQL-familiarity an assumption imported from the data-science framing the honest-position section explicitly steps away from?

Until those three are answered, framing "the SQL typechecker" as work would be manufacturing a milestone for a decision that hasn't been made. Labelled `needs-conversation` and left there on purpose.
