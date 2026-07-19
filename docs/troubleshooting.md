# Troubleshooting

*An index of the messages `go tool notebook check`, `run`, and `build` emit, what
each means, and the fix. Most map directly to one of the [rules that
bite](authoring.html#3-the-four-rules-that-bite).*

---

## Wiring and graph errors

**`cell "X" needs `p T`, but no cell produces it.`**
A cell takes a parameter that no other cell produces as a result. Either the
producer's result name/type doesn't match, or you meant to make the parameter an
input. If `check` adds *"Did you mean `Y`, which produces `T`?"*, that cell
produces the right type under a different name — rename one to match. Remember the
edge is the **result name**, not the function name ([how names work](names.html)).

**`cell "X" produces `r T`, but `r` is already produced by cell "Y".`**
Two cells return a result with the same name. A result name is an edge, and edges
must be unique. Give one a different name.

**`cell "X" takes `p T`, but cell "Y" produces `p` as `U`.`**
The names match but the types don't (`p int` vs `p float64`). An edge matches on
**both** name and type. Make the types agree, or rename one side so they are
deliberately distinct edges.

**`cycle among cells: A → B → A.`**
Cells depend on each other in a loop. The graph must be acyclic — a value cannot
depend on itself, even transitively. A stateful feedback loop would need `Prev[T]`
folds, which are not supported yet; restructure as a one-directional computation
(often a fixed-horizon pure cell that computes all steps at once).

## "My function isn't showing up"

**A top-level function is missing from the graph.**
It produces no *named* result, so it is treated as an ordinary **helper** and left
out (this is intentional — it's how you write helpers). `check` lists such
functions under *"helpers (not cells — they name no result)"*. To make it a cell,
name its result: `func f() (out T)`, not `func f() T`.

**`cell "X" has an unnamed result; cell results must be named — the name is the edge.`**
A cell named *some* results but left a non-`error` result unnamed. Name every
result, or leave them all unnamed to make it a helper.

**A function I documented still isn't a cell.**
The doc comment does not make a cell — a **named result** does. A documented
function with unnamed returns is still a helper. (See [how names work](names.html).)

## Rich output not appearing

**A cell computes a value but shows nothing.**
A composite (a struct or slice) with no `Render()` method stays hidden by design —
never a broken box. To draw it, give its type a `Render()` method returning a
`{MIME, Data}` value ([rendering](reference-rendering.html)). A bare scalar always
shows as a readout; only non-scalars without a `Render` are hidden.

**`cell "X" has a Render() method, but …; it will not render.`**
The `Render()` method exists but its shape is wrong. The probe requires
`Render()` taking **no arguments** and returning **one struct** with string
fields named exactly **`MIME`** and **`Data`** (case-sensitive — `MIME`, not
`Mime`). The message names the specific mismatch.

## WASM / browser build

**`"pkg" is not WASM-able. These cells transitively touch net/os/cgo …`**
A cell's call graph reaches `net`, `os`, or cgo, which have no browser form. That
notebook can still build as a native binary; it just can't compile to WASM. Move
the offending work out of the cell, or accept it as native-only.

**A cell that only formats a number is flagged as not WASM-able.**
`fmt` transitively reaches `os`, and the portability gate is a conservative
over-approximation of the call graph — so `fmt` in a **cell body** trips it even
when no real I/O happens (the tool says so: *"flagged conservatively via
fmt→os"*). Fix: move formatting into a `Render()` method (which is not a cell), or
use `strconv` if a cell body genuinely must format a number.

**A built WASM page is blank, or the console shows a fetch/CORS error.**
WASM must be served over **HTTP**, not opened as a `file://` path. Serve the
build directory: `(cd site && python3 -m http.server 8080)` then open
`http://localhost:8080`.

## Package and file errors

**`no //go:notebook file found`**
No file in the package carries the `//go:notebook` marker. Add it as a comment
line at the top of your notebook file.

**`package has more than one //go:notebook file`**
Exactly one file per package carries the marker; the rest of the package is
ordinary Go (helpers, types, tests split across files are fine — just one file
gets `//go:notebook`).

## Stale results

**Edits don't take effect / an old value persists.**
`run` rebuilds on save; if a served value looks stale, confirm the save landed
and the rebuild finished (the terminal logs it). A built binary loads a persisted
head file (`notebook-head.json` by default) — a leftover head can inject old input
values. Pass `--head <fresh-path>` to start clean, or delete the stale head file.
Built artifacts are content-addressed (the WASM filename carries a hash of the
source), so a genuinely new build never collides with a cached old one.
