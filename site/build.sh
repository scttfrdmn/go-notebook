#!/usr/bin/env bash
# Regenerate the landing-page demos. There is no build pipeline — this is three
# `notebook build --target=wasm` calls. index.html is hand-written source;
# demos/ is generated output (gitignored) and this script recreates it.
#
#   ./site/build.sh        # from the repo root
#
# Then serve the directory over HTTP (wasm needs http, not file://):
#   (cd site && python3 -m http.server 8080) → open http://localhost:8080
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

go build -o /tmp/notebook-build ./cmd/notebook
for nb in capacity curvefit bayes anscombe nbody turing surface gpulife percolation lotka clt boundary fourier kmeans spectrogram reliability little critpath; do
  /tmp/notebook-build build --target=wasm -o "site/demos/$nb" "examples/$nb"
done

echo "demos rebuilt. serve: (cd site && python3 -m http.server 8080)"
