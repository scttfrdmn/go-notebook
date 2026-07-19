// Command docgen renders the curated, reader-facing docs (docs/*.md) into styled
// HTML pages under site/docs/, sharing the landing page's typeface and palette.
// It runs only at site-build time (site/build.sh) and never ships in a notebook
// binary, so its goldmark dependency stays out of the toolchain proper.
//
// Curated, not automatic: only the docs a reader needs are listed in `pages`.
// The design docs for unbuilt features (composition, notebook-as-service) are
// deliberately excluded — publishing a spec for a feature that does not exist yet
// would be documentation that claims more than the code delivers.
//
// Usage (from repo root):
//
//	go run ./site/docgen
package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// page is one entry in the docs nav: the source markdown, the output slug, and
// the short title/blurb shown in the sidebar and index.
type page struct {
	src, slug, title, blurb string
}

// pages is the curated, ordered doc set — how-to first, then the deeper reads.
var pages = []page{
	{"docs/authoring.md", "authoring", "Write your first notebook", "From an empty file to a running, built notebook — the from-scratch walkthrough."},
	{"docs/live-feeds.md", "live-feeds", "Live feeds", "Wire a sensor, socket, or polled API into a notebook: a feed is a driver on the set port."},
	{"docs/design.md", "design", "The design", "How the system works: a cell is a function, the graph is derived, four topologies from one file."},
	{"docs/paper.md", "paper", "The paper", "Why it works this way — the system paper, with the corpus, the numbers, and the findings."},
}

func main() {
	root, err := os.Getwd()
	must(err)
	outDir := filepath.Join(root, "site", "docs")
	must(os.MkdirAll(outDir, 0o755))

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Typographer),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()), // our own docs, trusted
	)

	for _, p := range pages {
		srcBytes, err := os.ReadFile(filepath.Join(root, p.src))
		must(err)

		var body bytes.Buffer
		must(md.Convert(srcBytes, &body))
		out := shell(p.title, navLinks(p.slug), rewriteLinks(body.String()))
		must(os.WriteFile(filepath.Join(outDir, p.slug+".html"), []byte(out), 0o644))
		fmt.Printf("docs: wrote site/docs/%s.html\n", p.slug)
	}

	// The docs index: a card per page.
	var cards strings.Builder
	for _, p := range pages {
		fmt.Fprintf(&cards,
			`<a class="doccard" href="%s.html"><h3>%s</h3><p>%s</p></a>`+"\n",
			p.slug, html.EscapeString(p.title), html.EscapeString(p.blurb))
	}
	index := shell("Documentation",
		navLinks(""),
		`<h1>Documentation</h1>
<p class="lead">Start with <b>Write your first notebook</b>. Read <b>The paper</b> for why the system is shaped this way, and <b>The design</b> for how.</p>
<div class="doccards">`+cards.String()+`</div>`)
	must(os.WriteFile(filepath.Join(outDir, "index.html"), []byte(index), 0o644))
	fmt.Println("docs: wrote site/docs/index.html")
}

// navLinks renders the docs sidebar, marking the current page.
func navLinks(current string) string {
	var b strings.Builder
	b.WriteString(`<a class="dochome" href="index.html">Documentation</a>`)
	for _, p := range pages {
		cls := "docnav-item"
		if p.slug == current {
			cls += " active"
		}
		fmt.Fprintf(&b, `<a class="%s" href="%s.html">%s</a>`, cls, p.slug, html.EscapeString(p.title))
	}
	return b.String()
}

// mdLink matches href="something.md" and href="something.md#anchor" so intra-doc
// links point at the generated pages, not the raw markdown.
var mdLink = regexp.MustCompile(`href="([^"]+)\.md(#[^"]*)?"`)

func rewriteLinks(htmlBody string) string {
	return mdLink.ReplaceAllString(htmlBody, `href="$1.html$2"`)
}

// shell wraps rendered doc HTML in the page chrome: the same nav bar, palette,
// and self-hosted Atkinson fonts as the landing page, plus a docs sidebar.
func shell(title, sidebar, content string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(title) + ` — go-notebook docs</title>
<style>
` + docCSS + `
</style>
</head>
<body>
<nav class="nav">
  <div class="inner">
    <a class="brand" href="../index.html">go-notebook <span class="fn">·  a cell is a function</span></a>
    <div class="links">
      <a href="../index.html#corpus">Demos</a>
      <a href="index.html">Docs</a>
      <a class="gh" href="https://github.com/scttfrdmn/go-notebook">GitHub ↗</a>
    </div>
  </div>
</nav>
<div class="docwrap">
  <aside class="sidebar">` + sidebar + `</aside>
  <main class="doc">` + content + `</main>
</div>
</body>
</html>
`
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

// docCSS is the docs stylesheet: the landing page's fonts/palette (fonts live one
// level up in ../assets/fonts) plus a two-column doc layout with a sticky sidebar.
const docCSS = `
  @font-face { font-family:"Atkinson Hyperlegible"; font-style:normal; font-weight:400;
    font-display:swap; src:url("../assets/fonts/atkinson-regular.woff2") format("woff2"); }
  @font-face { font-family:"Atkinson Hyperlegible"; font-style:normal; font-weight:700;
    font-display:swap; src:url("../assets/fonts/atkinson-bold.woff2") format("woff2"); }
  @font-face { font-family:"Atkinson Hyperlegible"; font-style:italic; font-weight:400;
    font-display:swap; src:url("../assets/fonts/atkinson-italic.woff2") format("woff2"); }
  @font-face { font-family:"Atkinson Hyperlegible Mono"; font-style:normal; font-weight:400 700;
    font-display:swap; src:url("../assets/fonts/atkinson-mono.woff2") format("woff2"); }
  :root {
    --navy:#1b3a6b; --blue:#007ec6; --go:#00add8; --ink:#1a1a2e; --muted:#5b6472;
    --line:#e7ebf0; --bg:#fff; --code-bg:#0f1524; --code-fg:#e6ebf5;
    --font:"Atkinson Hyperlegible", -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
    --mono:"Atkinson Hyperlegible Mono", "SF Mono", ui-monospace, Menlo, monospace;
  }
  * { box-sizing:border-box; }
  html { scroll-behavior:smooth; }
  body { font-family:var(--font); font-size:1.0625rem; line-height:1.7; color:var(--ink);
    background:var(--bg); margin:0; -webkit-font-smoothing:antialiased; }
  a { color:var(--blue); }
  code, pre { font-family:var(--mono); }

  .nav { position:sticky; top:0; z-index:10; background:rgba(255,255,255,.9);
    backdrop-filter:saturate(140%) blur(8px); border-bottom:1px solid var(--line); }
  .nav .inner { max-width:1100px; margin:0 auto; padding:.6rem 24px; display:flex; align-items:center; gap:1.25rem; }
  .nav .brand { font-weight:700; color:var(--navy); text-decoration:none; letter-spacing:-.01em; font-size:.9375rem; }
  .nav .brand .fn { color:var(--go); }
  .nav .links { margin-left:auto; display:flex; align-items:center; gap:1.1rem; }
  .nav .links a { color:var(--muted); text-decoration:none; font-size:.9375rem; }
  .nav .links a:hover { color:var(--navy); }
  .nav .links a.gh { color:var(--navy); font-weight:600; }

  .docwrap { max-width:1100px; margin:0 auto; padding:0 24px; display:grid;
    grid-template-columns:220px 1fr; gap:2.5rem; align-items:start; }
  .sidebar { position:sticky; top:4rem; padding:2rem 0; display:flex; flex-direction:column; gap:.15rem; }
  .sidebar .dochome { font-weight:700; color:var(--navy); text-decoration:none; margin-bottom:.5rem; font-size:1.0625rem; }
  .sidebar .docnav-item { color:var(--muted); text-decoration:none; font-size:.9375rem; padding:.35rem .6rem;
    border-radius:6px; border-left:2px solid transparent; }
  .sidebar .docnav-item:hover { color:var(--navy); background:#f7f9fc; }
  .sidebar .docnav-item.active { color:var(--navy); font-weight:600; border-left-color:var(--go); background:#f3f8fc; }

  .doc { padding:2rem 0 4rem; min-width:0; max-width:74ch; }
  .doc h1 { font-size:2rem; line-height:1.1; letter-spacing:-.02em; color:var(--ink); margin:.5rem 0 1rem; font-weight:800; }
  .doc h2 { font-size:1.4rem; color:var(--navy); margin:2.2rem 0 .6rem; padding-top:.4rem; border-top:1px solid var(--line); }
  .doc h3 { font-size:1.1rem; color:var(--navy); margin:1.6rem 0 .4rem; }
  .doc p, .doc li { max-width:72ch; }
  .doc ul, .doc ol { padding-left:1.4rem; }
  .doc li { margin:.25rem 0; }
  .doc blockquote { margin:1rem 0; padding:.4rem 1rem; border-left:3px solid var(--line); color:var(--muted); }
  .doc code { background:#f3f5f9; padding:.12em .4em; border-radius:5px; font-size:.9em; }
  .doc pre { background:var(--code-bg); color:var(--code-fg); border-radius:10px; padding:1rem 1.15rem;
    overflow-x:auto; line-height:1.55; font-size:13.5px; }
  .doc pre code { background:none; padding:0; font-size:inherit; color:inherit; }
  .doc table { border-collapse:collapse; margin:1rem 0; font-size:.95rem; display:block; overflow-x:auto; }
  .doc th, .doc td { border:1px solid var(--line); padding:.4rem .7rem; text-align:left; }
  .doc th { background:#f7f9fc; color:var(--navy); }
  .doc img { max-width:100%; height:auto; }
  .doc hr { border:none; border-top:1px solid var(--line); margin:2rem 0; }
  .doc a { text-decoration:none; }
  .doc a:hover { text-decoration:underline; }
  .doc > p:first-of-type em { color:var(--muted); }

  .doc .lead { font-size:1.1875rem; color:var(--muted); max-width:64ch; }
  .doccards { display:grid; grid-template-columns:repeat(auto-fill, minmax(240px,1fr)); gap:1rem; margin-top:1.5rem; }
  .doccard { display:block; border:1px solid var(--line); border-radius:10px; padding:1.1rem 1.2rem;
    text-decoration:none; color:inherit; background:#fff; transition:border-color .12s, box-shadow .12s, transform .12s; }
  .doccard:hover { border-color:var(--go); box-shadow:0 3px 10px rgba(20,30,60,.10); transform:translateY(-1px); }
  .doccard h3 { margin:0 0 .3rem; color:var(--navy); font-size:1.0625rem; }
  .doccard p { margin:0; color:var(--muted); font-size:.9375rem; line-height:1.45; }

  @media (max-width:720px) {
    .docwrap { grid-template-columns:1fr; gap:0; }
    .sidebar { position:static; flex-direction:row; flex-wrap:wrap; gap:.5rem; padding:1rem 0; border-bottom:1px solid var(--line); }
  }
`
