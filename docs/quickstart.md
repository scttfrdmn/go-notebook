# Five-minute quickstart

*The shortest path to a running notebook. One input, one derived value, a live
slider — nothing else. For the full walkthrough (rendering, layout, building),
read [Write your first notebook](authoring.html) next.*

---

You need **Go 1.25 or newer**. A notebook is an ordinary Go package, so it lives
in a Go module.

```bash
mkdir hello && cd hello
go mod init hello
go get -tool github.com/scttfrdmn/go-notebook/cmd/notebook@latest
```

Create `hello.go`:

```go
//go:notebook
package hello

// Input value.
//notebook:slider min=0 max=100
func input() (x int) { return 20 }

// Twice the input.
func doubled(x int) (y int) { return x * 2 }
```

Run it:

```bash
go tool notebook run .
```

A browser opens showing the graph `input → doubled`, a slider, and a live value.
Drag the slider — `doubled` recomputes.

That is the whole model. The edge exists because `input` produces a result named
`x` and `doubled` takes a parameter named `x`:

> **A cell's named result feeds any cell that takes a parameter of the same name
> and type.**

You wrote no wiring, no callback, no reactive framework. The graph is derived
from the function signatures by the Go type checker, so it cannot drift from the
code.

## What just happened

- `//go:notebook` marks the file as a notebook — the only mention of this project
  anywhere. There is no import.
- `input` is a **cell**: a top-level function with a named result. Because it
  takes no parameters and its result is consumed, it is an **input** — and because
  its type is a plain scalar, it renders as a control (the `//notebook:slider`
  directive just refines it into a slider).
- `doubled` is a **derived** cell: it takes a parameter, so it sits downstream.

## See one run

Your two-cell notebook is deliberately tiny. Here is the next step — the
Celsius→Fahrenheit notebook the [first-notebook walkthrough](authoring.html)
builds — compiled to WebAssembly and running right here. Same rule (a result
named `c` feeds the parameter `c`), one more cell, a rendered gauge. Drag it:

<div class="demoframe"><iframe src="../demos/tempconv/index.html" loading="lazy" title="the tempconv notebook, live"></iframe></div>

## Next

- [Write your first notebook](authoring.html) — the same idea, then rendering,
  layout, and building a binary.
- [How the names work](names.html) — the function name, the result name, the
  type, and the label are four different things; this is the one distinction
  worth learning early.
- [The minimal examples](https://github.com/scttfrdmn/go-notebook/tree/main/examples/minimal)
  — one copyable notebook per mechanism.
