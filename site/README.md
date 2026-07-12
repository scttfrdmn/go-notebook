# site — the go-notebook.dev landing page

A static directory. Not a framework, not a build pipeline — that's the argument,
so the site is made the same way.

- `index.html` — the page (hand-written; committed).
- `demos/` — the three notebooks compiled to WebAssembly (generated; gitignored).
  Each `demos/<nb>/` is the *unmodified* output of
  `notebook build --target=wasm` — the same artifact anyone gets, dropped in an
  iframe. Nothing about the demos is special-cased for the site.

## Build & serve

```
./site/build.sh                       # regenerate demos/ (three wasm builds)
(cd site && python3 -m http.server 8080)
open http://localhost:8080
```

WebAssembly must be served over HTTP, not opened as `file://`.

## What the page is for

It answers one question: does the pitch survive a stranger with thirty seconds?
Demos first (drag before you read), then the source with the callout on its
absences (no framework import, no `.ipynb`), then the numbers — including the
one that doesn't flatter (single-threaded in the browser, no goroutine fan-out).
Then the second act: the same file runs as an `sbatch` job. Keep that register;
the credibility is the point.
