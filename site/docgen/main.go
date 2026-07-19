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
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// page is one entry in the docs nav: the source markdown, the output slug, the
// short title/blurb shown in the sidebar and index, and the section it groups under.
type page struct {
	src, slug, title, blurb, section string
}

// pages is the curated, ordered doc set, grouped by section: the guide first, a
// full feature reference, then the deeper reads.
var pages = []page{
	{"docs/authoring.md", "authoring", "Write your first notebook", "From an empty file to a running, built notebook — the from-scratch walkthrough.", "Guide"},
	{"docs/live-feeds.md", "live-feeds", "Live feeds", "Wire a sensor, socket, or polled API into a notebook: a feed is a driver on the set port.", "Guide"},

	{"docs/reference-directives.md", "reference-directives", "Directives", "The //notebook: comment directives — slider, height, area, layout, nocache — and the presentation-only rule they share.", "Reference"},
	{"docs/reference-controls.md", "reference-controls", "Controls", "How a value becomes an input, and which widget it renders as — decided by type, not directive.", "Reference"},
	{"docs/reference-rendering.md", "reference-rendering", "Rendering", "How a value is drawn: the Render method, the MIME types, and the degradation ladder.", "Reference"},
	{"docs/reference-build-run.md", "reference-build-run", "Build & run", "The check/run/build verbs, the binary's --headless/--set/--json flags, and the WASM gate.", "Reference"},
	{"docs/reference-layout.md", "reference-layout", "Layout", "Arrange a notebook with area + layout — presentation over source order, degrading to linear.", "Reference"},
	{"docs/reference-provenance.md", "reference-provenance", "Provenance", "What every built artifact records about its origin, and why it makes figures reproducible.", "Reference"},

	{"docs/design.md", "design", "The design", "How the system works: a cell is a function, the graph is derived, four topologies from one file.", "Deep reads"},
	{"docs/paper.md", "paper", "The paper", "Why it works this way — the system paper, with the corpus, the numbers, and the findings.", "Deep reads"},
}

func main() {
	root, err := os.Getwd()
	must(err)
	outDir := filepath.Join(root, "site", "docs")
	must(os.MkdirAll(outDir, 0o755))

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
			// Syntax-highlight fenced code at build time (chroma). WithClasses(false)
			// inlines the colors as style attrs, so no separate stylesheet ships and
			// the dark theme sits on our dark --code-bg. Build-time only — chroma
			// never enters the notebook binary.
			highlighting.NewHighlighting(
				highlighting.WithStyle("github-dark"),
				highlighting.WithFormatOptions(chromahtml.WithClasses(false)),
			),
		),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()), // our own docs, trusted
	)

	for _, p := range pages {
		srcBytes, err := os.ReadFile(filepath.Join(root, p.src))
		must(err)

		var body bytes.Buffer
		must(md.Convert(srcBytes, &body))
		out := shell(p.title, navLinks(p.slug), rewriteLinks(body.String()), p.slug, p.blurb)
		must(os.WriteFile(filepath.Join(outDir, p.slug+".html"), []byte(out), 0o644))
		fmt.Printf("docs: wrote site/docs/%s.html\n", p.slug)
	}

	// The docs index: cards grouped by section.
	var cards strings.Builder
	lastSection := ""
	for _, p := range pages {
		if p.section != lastSection {
			if lastSection != "" {
				cards.WriteString("</div>\n")
			}
			fmt.Fprintf(&cards, `<h2 class="docsection">%s</h2>`+"\n"+`<div class="doccards">`+"\n", html.EscapeString(p.section))
			lastSection = p.section
		}
		fmt.Fprintf(&cards,
			`<a class="doccard" href="%s.html"><h3>%s</h3><p>%s</p></a>`+"\n",
			p.slug, html.EscapeString(p.title), html.EscapeString(p.blurb))
	}
	if lastSection != "" {
		cards.WriteString("</div>\n")
	}
	index := shell("Documentation",
		navLinks(""),
		`<h1>Documentation</h1>
<p class="lead">New here? Start with <b>Write your first notebook</b>. The <b>Reference</b> covers every feature; the <b>Deep reads</b> explain why the system is shaped this way.</p>
`+cards.String(),
		"index", "Documentation for go-notebook — the guide, a full feature reference, and the design and paper.")
	must(os.WriteFile(filepath.Join(outDir, "index.html"), []byte(index), 0o644))
	fmt.Println("docs: wrote site/docs/index.html")

	checkLinks(outDir)
	writeDiscoveryFiles(filepath.Join(root, "site"))
}

// baseURL is the site's canonical origin (a custom domain on GitHub Pages). Used
// for absolute URLs in the sitemap, llms.txt, and JSON-LD.
const baseURL = "https://go-notebook.dev"

// writeDiscoveryFiles emits the crawler/LLM discovery files — robots.txt,
// sitemap.xml, llms.txt — from the SAME page list the docs are built from, so they
// cannot drift from what actually exists. All three live at the site root.
func writeDiscoveryFiles(siteDir string) {
	// Every canonical URL: the landing page + each generated doc page.
	urls := []string{baseURL + "/"}
	for _, p := range pages {
		urls = append(urls, baseURL+"/docs/"+p.slug+".html")
	}

	// robots.txt — allow everything, point at the sitemap.
	robots := "User-agent: *\nAllow: /\n\nSitemap: " + baseURL + "/sitemap.xml\n"
	must(os.WriteFile(filepath.Join(siteDir, "robots.txt"), []byte(robots), 0o644))

	// sitemap.xml
	var sm strings.Builder
	sm.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sm.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, u := range urls {
		fmt.Fprintf(&sm, "  <url><loc>%s</loc></url>\n", html.EscapeString(u))
	}
	sm.WriteString("</urlset>\n")
	must(os.WriteFile(filepath.Join(siteDir, "sitemap.xml"), []byte(sm.String()), 0o644))

	// llms.txt — a structured, plain-text index for language models: what the
	// project is, then a linked list of the docs with one-line descriptions.
	// (Convention: https://llmstxt.org)
	var lm strings.Builder
	lm.WriteString("# go-notebook\n\n")
	lm.WriteString("> A reactive notebook toolchain for Go. A notebook is an ordinary Go package and a cell is a top-level function; the dependency graph is derived by the Go type checker from function signatures. The same file compiles to an interactive WebAssembly app, a headless batch binary, or an HTTP-served page.\n\n")
	lm.WriteString("Key idea: a cell's named result feeds any cell that takes a parameter of the same name and type, so the graph cannot drift from the code. The artifact is a single static binary — no interpreter, kernel, or environment to reconstitute.\n\n")
	lm.WriteString("## Documentation\n\n")
	for _, p := range pages {
		fmt.Fprintf(&lm, "- [%s](%s/docs/%s.html): %s\n", p.title, baseURL, p.slug, p.blurb)
	}
	lm.WriteString("\n## Source\n\n")
	lm.WriteString("- [Repository](https://github.com/scttfrdmn/go-notebook): the full source, Apache-2.0.\n")
	lm.WriteString("- [Examples](https://github.com/scttfrdmn/go-notebook/tree/main/examples): ~39 notebooks read as Go.\n")
	must(os.WriteFile(filepath.Join(siteDir, "llms.txt"), []byte(lm.String()), 0o644))

	fmt.Println("docs: wrote robots.txt, sitemap.xml, llms.txt")
}

// hrefLocal matches href="something.html" (and #anchors) that point at a sibling
// doc page — not an absolute URL, not ../ outside the docs dir.
var hrefLocal = regexp.MustCompile(`href="([a-z0-9-]+)\.html(?:#[^"]*)?"`)

// checkLinks fails the build if any generated page links to a local .html that
// was not generated — so a renamed or dropped doc can't ship a dead link (the
// exact failure this pass was fixing: authoring linked composition.html, which
// no longer exists). A specification is a claim; this makes "the link works" one
// the build verifies rather than one we hope holds.
func checkLinks(outDir string) {
	var dead []string
	for _, p := range pages {
		htmlBytes, err := os.ReadFile(filepath.Join(outDir, p.slug+".html"))
		must(err)
		for _, m := range hrefLocal.FindAllStringSubmatch(string(htmlBytes), -1) {
			target := m[1] + ".html"
			if _, err := os.Stat(filepath.Join(outDir, target)); err != nil {
				dead = append(dead, fmt.Sprintf("%s.html → %s (no such page)", p.slug, target))
			}
		}
	}
	if len(dead) > 0 {
		fmt.Fprintln(os.Stderr, "docgen: dead intra-doc links:")
		for _, d := range dead {
			fmt.Fprintln(os.Stderr, "  "+d)
		}
		os.Exit(1)
	}
	fmt.Println("docs: link check passed")
}

// navLinks renders the docs sidebar, grouped by section, marking the current page.
func navLinks(current string) string {
	var b strings.Builder
	b.WriteString(`<a class="dochome" href="index.html">Documentation</a>`)
	lastSection := ""
	for _, p := range pages {
		if p.section != lastSection {
			fmt.Fprintf(&b, `<div class="docnav-section">%s</div>`, html.EscapeString(p.section))
			lastSection = p.section
		}
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
// and self-hosted Atkinson fonts as the landing page, plus a docs sidebar. slug
// and desc drive the per-page canonical URL, meta description, and JSON-LD so each
// page is individually discoverable (AI-readiness) and legible to assistive tech.
func shell(title, sidebar, content, slug, desc string) string {
	canonical := baseURL + "/docs/" + slug + ".html"
	if slug == "index" {
		canonical = baseURL + "/docs/"
	}
	// JSON-LD: a TechArticle node for each doc page, in the site's software context.
	jsonLD := `{"@context":"https://schema.org","@type":"TechArticle",` +
		`"headline":` + jsonStr(title) + `,"description":` + jsonStr(desc) +
		`,"url":` + jsonStr(canonical) +
		`,"author":{"@type":"Person","name":"Scott Friedman"},` +
		`"isPartOf":{"@type":"SoftwareSourceCode","name":"go-notebook","codeRepository":"https://github.com/scttfrdmn/go-notebook"}}`
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(title) + ` — go-notebook docs</title>
<meta name="description" content="` + html.EscapeString(desc) + `">
<link rel="canonical" href="` + canonical + `">
<script type="application/ld+json">` + jsonLD + `</script>
<style>
` + docCSS + `
</style>
</head>
<body>
<a class="skip" href="#main">Skip to content</a>
<nav class="nav" aria-label="Primary">
  <div class="inner">
    <a class="brand" href="../index.html">go-notebook <span class="fn">·  a cell is a function</span></a>
    <div class="links">
      <a href="../index.html#corpus">Demos</a>
      <a href="authoring.html">Get started</a>
      <a href="index.html">Docs</a>
      <a href="paper.html">Paper</a>
      <a class="gh" href="https://github.com/scttfrdmn/go-notebook">GitHub ↗</a>
    </div>
  </div>
</nav>
<div class="docwrap">
  <aside class="sidebar"><nav aria-label="Documentation">` + sidebar + `</nav></aside>
  <main class="doc" id="main">` + content + `</main>
</div>
<script>
// Add a copy-to-clipboard button to every code block. No dependency: wrap each
// <pre> in a relatively-positioned box and copy its textContent on click.
var copyIcon = '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>';
var checkIcon = '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
document.querySelectorAll('.doc pre').forEach(function (pre) {
  var box = document.createElement('div'); box.className = 'codebox';
  pre.parentNode.insertBefore(box, pre); box.appendChild(pre);
  var btn = document.createElement('button');
  btn.className = 'copybtn'; btn.type = 'button';
  btn.innerHTML = copyIcon; btn.setAttribute('aria-label', 'Copy code to clipboard'); btn.title = 'Copy';
  box.appendChild(btn);
  btn.addEventListener('click', function () {
    navigator.clipboard.writeText(pre.innerText).then(function () {
      btn.innerHTML = checkIcon; btn.classList.add('ok'); btn.title = 'Copied';
      setTimeout(function () { btn.innerHTML = copyIcon; btn.classList.remove('ok'); btn.title = 'Copy'; }, 1400);
    });
  });
});
</script>
</body>
</html>
`
}

// jsonStr JSON-encodes a string (with surrounding quotes) for embedding in the
// JSON-LD literal — handles the escaping so a title with a quote or backslash is safe.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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
    --navy:#1b3a6b; --blue:#0070b8; --go:#00add8; --go-text:#0b7a99; --ink:#1a1a2e; --muted:#5b6472;
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
  /* Accessibility: a skip link that appears on focus, and a visible focus ring on
     every interactive element. */
  .skip { position:absolute; left:-999px; top:0; z-index:100; background:var(--navy); color:#fff;
    padding:.6rem 1rem; border-radius:0 0 8px 0; text-decoration:none; }
  .skip:focus { left:0; }
  a:focus-visible, button:focus-visible, summary:focus-visible {
    outline:3px solid var(--go); outline-offset:2px; border-radius:3px; }
  /* Honor a reader's reduced-motion preference: no smooth-scroll, no transitions. */
  @media (prefers-reduced-motion: reduce) {
    html { scroll-behavior:auto; }
    * { transition:none !important; animation:none !important; }
  }

  .nav { position:sticky; top:0; z-index:10; background:rgba(255,255,255,.9);
    backdrop-filter:saturate(140%) blur(8px); border-bottom:1px solid var(--line); }
  .nav .inner { max-width:1100px; margin:0 auto; padding:.7rem 24px; display:flex; align-items:center; gap:1.5rem; }
  .nav .brand { font-weight:700; color:var(--navy); text-decoration:none; letter-spacing:-.01em; font-size:1.1875rem; }
  .nav .brand .fn { color:var(--go-text); }
  .nav .links { margin-left:auto; display:flex; align-items:center; gap:1.4rem; }
  .nav .links a { color:var(--muted); text-decoration:none; font-size:1.0625rem; }
  .nav .links a:hover { color:var(--navy); }
  .nav .links a.gh { color:var(--navy); font-weight:600; }

  .docwrap { max-width:1100px; margin:0 auto; padding:0 24px; display:grid;
    grid-template-columns:220px 1fr; gap:2.5rem; align-items:start; }
  .sidebar { position:sticky; top:4rem; padding:2rem 0; display:flex; flex-direction:column; gap:.1rem; }
  .sidebar .dochome { font-weight:700; color:var(--navy); text-decoration:none; margin-bottom:.4rem; font-size:1.0625rem; }
  .sidebar .docnav-section { font-size:.75rem; font-weight:700; letter-spacing:.06em; text-transform:uppercase;
    color:var(--muted); margin:1rem 0 .3rem .6rem; }
  .sidebar .docnav-item { color:var(--muted); text-decoration:none; font-size:.9375rem; padding:.32rem .6rem;
    border-radius:6px; border-left:2px solid transparent; }
  .sidebar .docnav-item:hover { color:var(--navy); background:#f7f9fc; }
  .sidebar .docnav-item.active { color:var(--navy); font-weight:600; border-left-color:var(--go); background:#f3f8fc; }
  .docsection { font-size:.8rem; font-weight:700; letter-spacing:.06em; text-transform:uppercase;
    color:var(--muted); margin:2rem 0 .3rem; }
  .docsection:first-of-type { margin-top:1.5rem; }

  .doc { padding:2rem 0 4rem; min-width:0; max-width:74ch; }
  .doc h1 { font-size:2rem; line-height:1.1; letter-spacing:-.02em; color:var(--ink); margin:.5rem 0 1rem; font-weight:800; }
  .doc h2 { font-size:1.4rem; color:var(--navy); margin:2.2rem 0 .6rem; }
  .doc h3 { font-size:1.1rem; color:var(--navy); margin:1.6rem 0 .4rem; }
  .doc p, .doc li { max-width:72ch; }
  .doc ul, .doc ol { padding-left:1.4rem; }
  .doc li { margin:.25rem 0; }
  .doc blockquote { margin:1rem 0; padding:.4rem 1rem; border-left:3px solid var(--line); color:var(--muted); }
  .doc code { background:#f3f5f9; padding:.12em .4em; border-radius:5px; font-size:.9em; }
  .doc pre { background:var(--code-bg); color:var(--code-fg); border-radius:10px; padding:1rem 1.15rem;
    overflow-x:auto; line-height:1.55; font-size:13.5px; margin:0; }
  .doc pre code { background:none; padding:0; font-size:inherit; color:inherit; }
  /* Copy-to-clipboard: an icon button in the top-right of each code block, the
     way modern docs do it — appears on hover, flips to a check on success. */
  .codebox { position:relative; margin:1rem 0; }
  .codebox .copybtn { position:absolute; top:.5rem; right:.5rem; z-index:1;
    display:inline-flex; align-items:center; justify-content:center; width:30px; height:30px;
    color:#c7d2e0; background:rgba(255,255,255,.06); border:1px solid rgba(255,255,255,.16);
    border-radius:7px; cursor:pointer; opacity:0; transition:opacity .12s, background .12s, color .12s; }
  .codebox:hover .copybtn, .codebox .copybtn:focus-visible { opacity:1; }
  .codebox .copybtn:hover { background:rgba(255,255,255,.14); color:#fff; }
  .codebox .copybtn.ok { color:#7ee787; opacity:1; }
  .codebox .copybtn svg { display:block; }
  .doc table { border-collapse:collapse; margin:1rem 0; font-size:.95rem; display:block; overflow-x:auto; }
  .doc th, .doc td { border:1px solid var(--line); padding:.4rem .7rem; text-align:left; }
  .doc th { background:#f7f9fc; color:var(--navy); }
  .doc img { max-width:100%; height:auto; }
  /* An embedded live notebook (a wasm demo) inside a doc page. */
  .doc .demoframe { border:1px solid var(--line); border-radius:10px; overflow:hidden;
    background:#fff; box-shadow:0 1px 3px rgba(20,30,60,.06); margin:1.25rem 0; }
  .doc .demoframe iframe { width:100%; height:520px; border:0; display:block; }
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
