# Presentation deck — build directions

*A slide-by-slide brief for building a go-notebook talk, derived from `docs/paper.md`. Hand this (or the paper) to Claude Design. Target: a ~15–18 slide technical talk, 20–25 minutes. Audience: engineers who know notebooks (Jupyter/marimo) and care about systems/HPC. Tone: confident but honest — this project's whole credibility rests on "no overclaim, the source is one click away," so the deck must not oversell.*

---

## Design language (tell Claude Design this)

- **Palette (brand):** navy `#1b3a6b` (primary/ink, titles), go-cyan `#00add8` (accent), ink `#1a1a2e` (body), muted `#5b6472` (secondary), line `#e7ebf0` (hairlines/grid), white ground. Status where needed: good `#0ca30c`, warning `#fab219`, critical `#d03b3b`. **Data-series colors** (if any chart appears): blue `#2a78d6`, aqua `#0797b8`, yellow `#eda100`, green `#008300`, violet `#4a3aa7`, red `#e34948` — assigned in that order, never navy for a series (navy is ink).
- **Type:** system sans (`system-ui, -apple-system, "Segoe UI"`), no display/serif face. Code in a monospace. Large numbers use proportional figures; tabular figures only for aligned columns.
- **Feel:** restrained, lots of white space, one idea per slide, hairline rules not boxes. It should look like the landing page ([go-notebook.dev](https://go-notebook.dev)) — calm, precise, engineered. Cards (subtle border, rounded, soft shadow) for grouped content.
- **No stock photography, no gradients-for-decoration, no clip art.** Diagrams are simple SVG-style: boxes, arrows, a dependency graph.

---

## The narrative arc (the spine)

One sentence unfolds into a system: **a cell is a function → the graph is derived, not maintained → the same file runs three ways → the view is a readout of types → and the discipline that kept it honest is "running is not passing."** Every slide should serve that arc; cut anything that doesn't.

---

## Slides

### 1 — Title
- **go-notebook** / *A reactive notebook where a cell is a function*
- Subtitle: reactive notebooks that compile — one Go file, running in your browser, or as a cluster job.
- Small: the logo/wordmark; a URL (go-notebook.dev).

### 2 — The problem (why another notebook)
- Jupyter: the dependency graph lives in your head; hidden state; JSON envelope; ships an interpreter.
- The frame: *what if the notebook were just source code, and the graph were derived from it?*
- Keep it to 3–4 lines. Don't bash Jupyter; state the gap.

### 3 — The bet (the hero slide)
- Huge, centered: **A cell is a function.**
- One line under it: *Everything else is a consequence.*
- This is the emotional center of the talk. Let it breathe — almost empty slide.

### 4 — The wiring rule
- **A cell's named result feeds any cell that takes a parameter of the same name and type.**
- Show a tiny code sample (3 cells: `arrivalRate → offeredLoad ← serviceRate`) beside the derived graph (3 boxes, 2 edges).
- Punchline: *the graph is a projection of the code — it can't drift from it, because it is it.*

### 5 — Live demo moment (or a screenshot standing in for it)
- The capacity notebook: drag a slider, the wave lights up the dependency graph, the chart moves.
- If live: do it here. If not: an annotated screenshot with the graph at top, sliders, a chart.
- Caption: *the graph isn't a diagram of the notebook; it is the notebook — and the best debugger in the system.*

### 6 — The view is a readout of types
- Structural capability probing, not declaration: `Render()` → rich view; `Bounds()` → slider; `Options()` → select; `Reconcile()` → stateful widget.
- Key line: **adding a control kind means adding a capability probe, never editing a switch.**
- The degradation ladder: *losing the view costs polish, never correctness.*

### 7 — The same file, three ways (the differentiator)
- A single file, three arrows out: **browser (WASM)** · **batch job (`--headless --json`)** · **HTTP server**.
- The number that lands: `notebook build ./capacity` → one static binary, `scp` to a cluster, `sbatch`. No conda, no kernel, no spawner.
- Contrast, stated fairly: Jupyter-on-HPC is environments and spawners; this is one file you copy.

### 8 — Numbers (measured, including the one that doesn't flatter)
- Stat tiles: **~1 MB** gzipped wasm · **~40 ms** cold-to-interactive · **~300 µs** slider→repaint · **0** servers/kernels/files.
- The honest caveat, in its own tinted box: *in the browser, `GOOS=js` is single-threaded — the goroutine fan-out is absent there.* Native builds fan out.
- Point: the honesty IS the credibility. Don't hide the caveat; feature it.

### 9 — Composition: a notebook is also a dashboard
- Before/after, side by side: capacity as a plain source-order stack vs. capacity arranged (`controls | readouts` / chart, as cards).
- The three directive lines that did it, shown small.
- Line: *presentation-only, fully optional — strip it and it degrades to the stack. It names regions and order, never geometry.*

### 10 — Deferring design to HTML
- The idea: a `Render()` can emit HTML, so the answer can be a **document**, not a chart.
- Three thumbnails: **invoice** (styled bill), **simpson** (paradox table where the TOTAL flips), **punchcard** (CSS heatmap).
- Line: **Go owns the science; HTML owns the presentation.** The view is a projection of the cells, never a second source of truth.

### 11 — The corpus as a falsification instrument
- 44 notebooks, 38 live — not a demo gallery, a stress test.
- The finding that matters: **19 notebooks across GPU / DFT / ODE / gradient-descent / cellular-automata → zero engine changes, zero "a cell is a function" violations.**
- Sub-point: porting found real *bugs in the originals* — `lego` multiplied dollars by dollars (a unit type made it a compile error).

### 12 — Running is not passing (the real contribution)
- Big statement: **a system producing a thing is not the same as the thing reaching anyone.**
- Two or three concrete catches: spectrogram's leakage metric was inverted vs. its own prose; a `Readout` computed correctly and *reached no one* (no `Render`, left hidden); consistent-hashing's first hash clustered and faked 100% churn.
- The two disciplines: *(1) assert the teaching claim, not the mechanism; (2) verify the instrument against a known-good before trusting a green.*
- This is the slide a systems audience remembers. Give it weight.

### 13 — The notebook as a component
- **One port:** `set` a leaf (data in), `subscribe` to a cell (data out) — same two calls for a human, a program, a foreign page, a batch job.
- A foreign page with our UI *absent* drives the compute (observed). Our default UI is one consumer of the port, not privileged.
- Optional: the notebook-as-service seam (announces its address on stdout; any launcher can spawn and drive it).

### 14 — What it cost / the honest position
- One new concept (`Prev` + `Tick`). Six corrections the work forced. One withdrawal: **compile-checked SQL, withdrawn** — typed Go over `Rel[T]` gave the same guarantee with no parser, no cgo. Standing costs named: no per-cell stdout, no browser parallelism.
- The structural point: *the same cut kept working at every layer; every "how do we avoid X" was "you already avoided it three decisions ago." Fewer layers is the point, not a gap.*

### 15 — Close
- Back to the hero line: **A cell is a function.** *Everything else compounded from a single sentence.*
- Where it fits: not "replace Jupyter for data science" — **systems, simulation, cluster work**, where there is no incumbent.
- CTA: go-notebook.dev · the source is one click away.

---

## Optional / appendix slides (if the talk runs long or Q&A wants depth)

- **Glitch-freedom:** the epoch'd immutable snapshot — the one correctness obligation, and why the test was written before the scheduler worked.
- **A path is not a handle:** `Rel[T]` content-addressing; the portfolio-tracker bug (charted the wrong company because a constant path can't notice the file changed).
- **The gofmt finding:** why the layout syntax is one directive per row (an ASCII-art block gets reflowed and reordered by gofmt).
- **Purity vs. portability:** two different call-graph verdicts, not one — a cell using `time` is impure but WASM-able.

---

## What to cut if you need to (priority order, keep the top)

1. Slides 3, 4, 7, 12 are the spine — never cut these.
2. 5 (demo), 9 (composition), 10 (HTML) are the "wow" — keep at least two.
3. 6, 8, 11, 13 are supporting — cut to fit.
4. 14 can compress to two lines on the close slide if needed.

## Speaker-note themes (one per slide, the thing to *say* not show)

- The recurring beat: *this fell out of the bet; we didn't design it separately.*
- The credibility beat: *here's the number that doesn't flatter, and here's the bug our own tests missed.*
- Never claim a benchmark you didn't measure; every number in the deck is in CI.
