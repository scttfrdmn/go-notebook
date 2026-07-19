# How the names work

*A cell has several names, and they do different jobs. Confusing them is the most
common early stumble, so here they are in one place. The short version: the
**result name is the edge** — it is what wiring, `--set`, and the dependency
graph all key on — while the function name is for your Go tools and the doc
comment is just the human label.*

---

Take one cell:

```go
// Incoming jobs per hour.          ← doc comment: the human label
//notebook:slider min=0 max=5000    ← directive: refines the control
func arrivalRate() (lambda PerHour) { return 1200 }
//   └ function name    └ result name and type
```

Five distinct names are in play:

| Concept | Example | What it is used for |
|---------|---------|---------------------|
| **Function name** | `arrivalRate` | Cell identity for your Go tools — `go doc`, gopls, jump-to-definition, refactoring. Also the fallback label if there is no doc comment. |
| **Result name** | `lambda` | **The edge.** Wiring, `--set lambda=…`, and the dependency graph all key on this. |
| **Result type** | `PerHour` | Type-safe edge matching — a `lambda PerHour` only wires to a parameter `lambda PerHour`, never to a `lambda int`. |
| **Doc sentence** | *Incoming jobs per hour.* | The human-facing label (first sentence of the doc comment). Presentation only. |
| **Area name** | `controls` | Layout grouping (`//notebook:area=controls`). Presentation only. |

## The result name is the edge

This is the one to internalize. A downstream cell wires to a producer by the
producer's **result name**, not its function name:

```go
func offeredLoad(lambda PerHour) (rho float64) { ... }
//               └─ consumes the edge named `lambda`, produced by arrivalRate
```

`offeredLoad` never mentions `arrivalRate`. It asks for `lambda PerHour`, and the
type checker connects it to whichever cell produces `lambda PerHour`. Rename the
*function* `arrivalRate` and nothing breaks; rename the *result* `lambda` and
every consumer must change too — because the result name is the contract.

Three consequences worth remembering:

- **`--set` uses the result name.** To override an input from the command line
  you write `--set lambda=2000`, not `--set arrivalRate=2000`. The result name is
  the leaf's identity everywhere data crosses a boundary (the `set` port, the
  browser's `notebook.set`, `--set`).
- **Result names must be unique** across the notebook — two cells producing
  `(chart Chart)` collide, because two edges cannot share a name.
- **The doc comment does not make a cell.** A named result makes a cell; the doc
  comment only supplies the label. An undocumented cell still works — it is just
  labelled from its function name. (Documenting is strongly recommended for the
  label and tooltip, but it is not the marker.)

## Why so many names

Because each is doing a job Go already understands. The function name is what Go
navigation and refactoring tools already track. The result name is an ordinary
named return value. The type is an ordinary type. The label is an ordinary doc
comment. Nothing here is notebook-specific machinery — the system reads meaning
off the shapes your Go code already has.
