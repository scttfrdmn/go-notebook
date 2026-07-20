#!/usr/bin/env bash
# Regenerate the landing-page demos and the docs pages. index.html is
# hand-written source; site/demos/ (wasm builds) and site/docs/ (rendered from
# docs/*.md) are generated output — both gitignored, both recreated here.
#
#   ./site/build.sh        # from the repo root
#
# Then serve the directory over HTTP (wasm needs http, not file://):
#   (cd site && python3 -m http.server 8080) → open http://localhost:8080
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

go build -o /tmp/notebook-build ./cmd/notebook
for nb in capacity curvefit bayes anscombe nbody turing surface gpulife percolation lotka clt boundary fourier kmeans spectrogram reliability little critpath amdahl roofline fleet pid retrystorm backfill cachepolicy latencybw aimd bdp fattree mandelbrot invoice simpson consistenthash punchcard sensorfeed homefeed tickerfeed apifeed; do
  /tmp/notebook-build build --target=wasm --showcase -o "site/demos/$nb" "examples/$nb"
done

# Two minimal recipes that are WASM-able and worth showing live. embedded-data
# demonstrates go:embed staying browser-portable; csv-native is deliberately
# ABSENT here — it uses os.Open + encoding/csv, so the WASM gate refuses it (that
# native-only tradeoff is the recipe's whole point).
for nb in embedded-data; do
  /tmp/notebook-build build --target=wasm --showcase -o "site/demos/$nb" "examples/minimal/$nb"
done

# tempconv is the notebook the authoring tutorial builds; it's embedded in that
# doc page (and teased at the end of the quickstart) as a working view.
/tmp/notebook-build build --target=wasm -o "site/demos/tempconv" "examples/tempconv"

# sales-analysis is the "normal analysis" example embedded live in the charts
# doc page (the full parse → filter → summarize → chart workflow).
/tmp/notebook-build build --target=wasm --showcase -o "site/demos/sales-analysis" "examples/minimal/sales-analysis"

# The component demo: a host page that drives a notebook through the JS client
# rather than mounting the built-in UI (site/component/{index,app}.js are
# committed source). It needs three generated runtime files: the wasm, Go's
# wasm_exec.js, and the client. The wasm the toolchain emits is content-addressed
# (its name is a hash of the bytes), so we build to a temp dir and copy the one
# .wasm to the stable name site/component/notebook.wasm the page fetches.
compdir="$(mktemp -d)"
/tmp/notebook-build build --target=wasm -o "$compdir" "examples/capacity"
cp "$compdir"/notebook-*.wasm site/component/notebook.wasm
cp "$compdir"/wasm_exec.js site/component/wasm_exec.js
cp client/notebook.js site/component/notebook.js
rm -rf "$compdir"

# Render the curated docs (docs/*.md → site/docs/*.html), styled to match.
go run ./site/docgen

echo "demos + docs rebuilt. serve: (cd site && python3 -m http.server 8080)"
