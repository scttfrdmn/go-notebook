# Publish & deploy

*You ran `build --target=wasm` and got a directory. This is how it gets online. A built WASM notebook is three static files — serve them from anywhere that serves static files, with one required MIME type and a two-line caching rule.*

## What `build --target=wasm` emits

```bash
go tool notebook build --target=wasm -o ./out ./mynotebook
```

The `out/` directory holds exactly three files, and nothing else:

| File | What it is |
|------|------------|
| `index.html` | the page: the UI, the client, and the WASM loader glue, inlined |
| `notebook-<hash>.wasm` | the compiled notebook — the whole Go program |
| `wasm_exec.js` | Go's WebAssembly runtime shim, version-matched to your toolchain |

That is the entire deployable. There is no server process, no build step on the host, no runtime dependency to install — the notebook runs entirely in the browser. `index.html` references the other two by **relative** path (`wasm_exec.js`, `notebook-<hash>.wasm`), so the directory can be served from a domain root or any subpath without rewriting anything.

## The one required setting: `application/wasm`

The page loads the WASM with `WebAssembly.instantiateStreaming`, which **requires the server to send `Content-Type: application/wasm`** for the `.wasm` file. Serve it as `application/octet-stream` (many hosts' default for an unknown extension) and streaming instantiation fails.

go-notebook hedges this: the loader catches the streaming failure and falls back to `fetch()` + `arrayBuffer()` + `WebAssembly.instantiate`, which works under any content type. So a misconfigured host still runs — just more slowly, buffering the whole 4 MB before compiling instead of compiling as it downloads. **Set the MIME type anyway; the fallback is a safety net, not the plan.** Most modern hosts (GitHub Pages, current Python `http.server`, S3 with the right metadata) already send `application/wasm`.

## Caching: immutable `.wasm`, never-cache `index.html`

The `.wasm` filename is **content-addressed** — `notebook-<hash>.wasm`, where the hash is of the compiled bytes. Change the notebook, rebuild, and the filename changes. That makes the URL immutable: a given `notebook-<hash>.wasm` is byte-for-byte the same forever, so it is safe to cache aggressively.

`index.html`, by contrast, is the file that points at the current hash. It must **not** be cached, or a returning visitor's browser will fetch the old `index.html`, ask for last build's `.wasm` hash, and 404.

```
notebook-<hash>.wasm   Cache-Control: public, max-age=31536000, immutable
wasm_exec.js           Cache-Control: public, max-age=31536000, immutable
index.html             Cache-Control: no-cache
```

(`wasm_exec.js` changes only when your Go toolchain version does; caching it long is fine and it is small.) On a plain static host with no per-file header control, everything gets the host's default — usually a short TTL, which is correct-but-slow for the `.wasm` and fine for `index.html`. The header rules above are the optimization, not a requirement to work.

## GitHub Pages

The simplest path: commit a workflow that builds the notebook and publishes the output directory. This mirrors how go-notebook.dev itself deploys.

```yaml
# .github/workflows/deploy-notebook.yml
name: deploy-notebook
on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

# One deploy at a time; let an in-progress run finish.
concurrency:
  group: pages
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: build the notebook to WASM
        run: go run github.com/scttfrdmn/go-notebook/cmd/notebook build --target=wasm -o ./out ./mynotebook
      - uses: actions/configure-pages@v5
      - uses: actions/upload-pages-artifact@v3
        with:
          path: out
  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deploy.outputs.page_url }}
    steps:
      - id: deploy
        uses: actions/deploy-pages@v4
```

Enable Pages for the repository with **Settings → Pages → Source: GitHub Actions**, then push to `main`. The notebook is rebuilt from source on every push, so the published page can never drift from the committed code. GitHub Pages already serves `.wasm` as `application/wasm`.

One caveat specific to GitHub Pages: it serves HTML with a fixed `Cache-Control: max-age=600` and gives you no way to override it (there is no `_headers` file support). Because the `.wasm` is content-addressed, this never serves a stale *notebook* — a new build is a new URL. But it does mean a reader who loaded `index.html` can hold it for up to ten minutes across a deploy, so immediately after pushing you may briefly see the previous HTML shell around the new WASM. If you need `no-cache` on the HTML itself (so a deploy is visible instantly), front Pages with a CDN you control, or use one of the static-host recipes below where the cache header is yours to set.

### Project sites live at a subpath

A user or org Pages site is served at the domain root (`https://you.github.io/`). A **project** site is served at `https://you.github.io/<repo>/` — a subpath. Because `index.html` references its assets by relative path, this needs **no configuration**: the page fetches `notebook-<hash>.wasm` relative to wherever `index.html` was loaded from, subpath or not. A custom domain (a `CNAME` file in the output) puts the site back at a root. The only thing that would break under a subpath is an absolute asset URL, and the build emits none.

## S3 + CloudFront

Upload the directory and set the MIME + cache headers as you go:

```bash
aws s3 cp ./out/ s3://my-bucket/ --recursive \
  --exclude "*.wasm" --exclude "index.html" \
  --cache-control "public, max-age=31536000, immutable"

# the .wasm: immutable, and the required content type
aws s3 cp ./out/ s3://my-bucket/ --recursive --exclude "*" --include "*.wasm" \
  --content-type "application/wasm" \
  --cache-control "public, max-age=31536000, immutable"

# index.html: never cache — it names the current .wasm hash
aws s3 cp ./out/index.html s3://my-bucket/index.html \
  --content-type "text/html" \
  --cache-control "no-cache"
```

Put CloudFront in front for TLS and a CDN. Set the default root object to `index.html`. Because the `.wasm` URL is content-addressed, a CloudFront invalidation is never needed for it — a new build is a new URL. Invalidate only `/index.html` on deploy (or leave it `no-cache` and skip invalidations entirely).

## Any static host (Netlify, Cloudflare Pages, nginx, …)

The rule is the same everywhere: **serve the directory, send `application/wasm` for `.wasm`, don't cache `index.html`.** Two examples:

nginx:

```nginx
location / {
    root /var/www/mynotebook;
    types { application/wasm wasm; }   # if not already in mime.types
}
location = /index.html { add_header Cache-Control "no-cache"; }
location ~ \.wasm$      { add_header Cache-Control "public, max-age=31536000, immutable"; }
```

Netlify / Cloudflare Pages: point the deploy at the output directory. Both send `application/wasm` by default; add a `_headers` file for the cache rules if you want them.

## Verify a deploy locally before you ship it

You do not need a host to test this — a static file server reproduces the exact conditions. From the output directory:

```bash
cd out && python3 -m http.server 8080   # then open http://127.0.0.1:8080
```

The page should reach **"ready — compiled Go, no server"** and render its cells. This matters because `run` (the dev server) serves an editor at `/__source` that a static host does not: opening the built page against a plain file server is the only way to see what a real visitor sees. (The editor probe 404s harmlessly against a static host — the page is designed to stay read-only when there is no supervisor behind it. That is expected, not a deployment error.) If it renders here, it renders on Pages.

## Embedding a built notebook in another site

The output directory drops into any page as an `<iframe>` — it is self-contained and has no same-origin requirement:

```html
<iframe src="/mynotebook/index.html" title="a live notebook"
        style="width:100%; height:520px; border:0"></iframe>
```

The built page posts its content height to the parent window (`message` type `notebook:height`), so a host can size the frame to fit. That is how the demos on this site are embedded.

If you want the host page to **drive** the notebook — read its inputs, feed it values, compute on its output stream — don't iframe it; mount it as a component through the JS client instead. See [JS client](reference-js-client.html).
