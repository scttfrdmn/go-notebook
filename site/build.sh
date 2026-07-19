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

# tempconv is the notebook the authoring tutorial builds; it's embedded in that
# doc page (and teased at the end of the quickstart) as a working view.
/tmp/notebook-build build --target=wasm -o "site/demos/tempconv" "examples/tempconv"

# csv is the "normal analysis" example embedded live in the charts doc page.
/tmp/notebook-build build --target=wasm --showcase -o "site/demos/csv" "examples/minimal/csv"

# Render the curated docs (docs/*.md → site/docs/*.html), styled to match.
go run ./site/docgen

echo "demos + docs rebuilt. serve: (cd site && python3 -m http.server 8080)"
