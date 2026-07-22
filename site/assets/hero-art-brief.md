# Hero illustration — art brief

The homepage hero (`site/index.html`, `.hero .art img`) shows an illustration of
the go-notebook idea beside the headline. The current asset (`hero-art.png`) is
serviceable but has generous baked-in whitespace, so the CSS crops it with
`object-fit: cover` to fill its column. A bespoke asset drawn *for this slot* would
drop in cleanly and let the cover-crop be removed.

This brief defines what a replacement must satisfy. It is for an illustrator or an
image tool — the point is a drop-in file, not a redesign of the page.

## The slot it fills

- **Position:** left column of the hero's two-column grid (`.herotop`), beside the
  headline "Reactive notebooks that compile." on the right. Vertically centered,
  aligned to roughly the height of the headline + tagline + pills block.
- **Aspect ratio:** design to **5 : 4** (landscape-ish, e.g. 1000 × 800). On
  screens ≤ 720px the layout stacks and crops to 16:9, so keep the subject
  centered enough to survive a wider crop.
- **Bleed / safe area:** the art is allowed to **crop on its right edge** against
  the headline column — that meeting is intentional. Keep the subject and any text
  in the art within the **left ~75%**; the right ~25% may be quiet background that
  can crop away. **No meaningful margin on the left or bottom** — the art should
  reach those edges so it fills the column rather than floating.
- **Background:** transparent, or the page white (`#ffffff`). No box, border, or
  drop shadow — the page provides the whitespace.

## Subject and motif

The one idea to convey: **a cell is a function, and the dependency graph is the
notebook.** The current art captures it well and should be kept in spirit — the Go
gopher holding an open book whose pages *are* a dependency graph:

- Green **input nodes** feeding into typed cells (`int`, `string`) via arrows.
- `fn` tabs along the top (functions), reinforcing "a cell is a function."
- The graph should read as a real DAG (nodes + directed edges), not decoration —
  it is the product's whole thesis that the graph *is* the code.

Keep it friendly and clean (line art + flat fills), matching the Atkinson
Hyperlegible, high-legibility tone of the site. Not skeuomorphic, not busy.

## Palette (exact)

Use the site's tokens so the art sits in the same system:

| Role | Hex | Token |
|------|-----|-------|
| Primary cyan (fills, accents) | `#00add8` | `--go` |
| Deep navy (outlines, `int`/`string` cells, headline) | `#1b3a6b` | `--navy` |
| Input-node green | `#3fa845` | `--green` |
| Highlight orange (sparingly — e.g. the bookmark) | `#f26b21` | `--orange` |
| Background | `#ffffff` | `--bg` |
| Hairlines / faint page rules | `#e7ebf0` | `--line` |

## Format and weight

- **SVG strongly preferred** — this is line-art + flat fills, ideal for vector:
  sharp at any DPI, tiny, and stylable. Target **< 60 KB**.
- If raster is unavoidable, PNG with transparency at 2× the display size; keep it
  **under ~150 KB** (the current PNG is ~380 KB, larger than warranted).
- Deliver as `site/assets/hero-art.svg` (or `.png`); update the `<img src>` and, if
  the new art has no baked-in whitespace, drop the `object-fit: cover` /
  `object-position` / `aspect-ratio` rules on `.hero .art img` so it renders at its
  natural proportions.

## Accessibility

- The `alt` text already describes the scene; keep it accurate to the final art.
- Ensure the graph's colors are distinguishable without relying on hue alone
  (shape + label carry meaning too), per the site's own rendering-a11y guidance.
