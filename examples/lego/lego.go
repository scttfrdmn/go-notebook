//go:notebook
//
// Exploring price differences in Lego sets.
//
// A port of marimo's gallery dashboard (notebooks/dashboard/lego/notebook.py).
// Same data, same controls, same charts. Stdlib only — no dataframe library,
// no plotting library, no notebook library.
//
// Three things this port has to prove, because the original leans on all three:
//
//   1. Widgets whose options/bounds come from the data (theme picker, price
//      ranges). A cell may RETURN a widget: the cell computes its bounds from
//      data, the head holds the user's selection, and the runtime reconciles
//      the two. Unlike marimo, changing the data does not reset your selection —
//      it clamps it.
//
//   2. Axis selection. marimo picks columns by string: alt.X(xaxis.value).
//      Here a dropdown selects a *typed accessor* — a func(Set) float64 — so a
//      bad axis is a compile error, not a runtime KeyError.
//
//   3. Brush selection on a chart. The chart is not a widget. It is an output;
//      the brush is a separate input leaf; the view layer draws them together.
//      Reactivity lives only in the graph.

package lego

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

const dataURL = "https://raw.githubusercontent.com/marimo-team/gallery-examples/" +
	"refs/heads/main/notebooks/dashboard/lego/lego_sets.csv"

// ---------------------------------------------------------------------------
// Data
// ---------------------------------------------------------------------------

// Lego sets with a US retail price, from themes with at least ten sets.
// A trailing error result is not a graph edge: it puts this cell in a failed
// state and marks everything downstream as blocked rather than wrong.
func sets() (all []Set, err error) {
	rows, err := fetchCSV(dataURL)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["category"] != "Normal" || r["US_retailPrice"] == "" {
			continue
		}
		price, err := strconv.ParseFloat(r["US_retailPrice"], 64)
		if err != nil {
			continue
		}
		pieces, _ := strconv.Atoi(r["pieces"])
		year, _ := strconv.Atoi(r["year"])
		if pieces <= 0 {
			continue
		}
		all = append(all, Set{
			ID: r["set_id"], Name: r["name"], Theme: Theme(r["theme"]),
			Year: Year(year), Pieces: pieces, Price: USD(price), Image: r["imageURL"],
		})
	}
	// Keep only themes with >= 10 sets (pl.len().over("theme") >= 10).
	count := map[Theme]int{}
	for _, s := range all {
		count[s.Theme]++
	}
	return slices.DeleteFunc(all, func(s Set) bool { return count[s.Theme] < 10 }), nil
}

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

// Themes to explore. Options are derived from the data, so this cell has a
// parameter — a widget whose choices are computed is just a cell that returns one.
func themePicker(all []Set) (themes Multi[Theme]) {
	var opts []Theme
	for _, t := range distinct(all, func(s Set) Theme { return s.Theme }) {
		opts = append(opts, t)
	}
	slices.Sort(opts)
	return Multi[Theme]{
		All:   opts,
		Value: []Theme{"Duplo", "Star Wars", "City"}, // default until the user picks
		Max:   5,
	}
}

// Year range.
func yearRange() (years Range[Year]) {
	return Range[Year]{Lo: 1970, Hi: 2022, From: 2001, To: 2022}
}

// Sets within the selected themes and years.
func subset(all []Set, themes Multi[Theme], years Range[Year]) (rows []Set) {
	for _, s := range all {
		if s.Year >= years.From && s.Year <= years.To && slices.Contains(themes.Value, s.Theme) {
			rows = append(rows, s)
		}
	}
	return rows
}

// Price range. Bounds track the data; your selection survives the data changing.
func priceRange(rows []Set) (prices Range[USD]) {
	return Range[USD]{Lo: 0, Hi: maxOf(rows, Set.price), From: 0, To: 150}
}

// Piece price range.
func piecePriceRange(rows []Set) (piecePrices Range[USD]) {
	return Range[USD]{Lo: 0, Hi: maxOf(rows, Set.piecePrice), From: 0, To: 2}
}

// X-axis. The dropdown selects a function, not a column name.
func xAxis() (x Select[Axis]) {
	return Select[Axis]{All: axes, Value: axes[1]} // pieces
}

// Y-axis.
func yAxis() (y Select[Axis]) {
	return Select[Axis]{All: axes[1:], Value: axes[2]} // price
}

// Show trend line.
func showTrend() (trend bool) { return false }

// Assume inflation.
func adjustForInflation() (adjust bool) { return false }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Sets passing every filter, with prices optionally inflation-adjusted.
func shownSets(rows []Set, prices, piecePrices Range[USD], adjust bool) (shown []Set) {
	for _, s := range rows {
		if s.Price < prices.From || s.Price > prices.To {
			continue
		}
		if pp := s.piecePrice(); pp < piecePrices.From || pp > piecePrices.To {
			continue
		}
		if adjust {
			// NB: the original multiplies price by an already-price-scaled
			// "inflation" column, i.e. price² · factor. Typing the factor as
			// Factor rather than USD makes that expression fail to compile,
			// which is the entire argument for typed columns in one line.
			s.Price = s.Price.scale(inflation(s.Year))
		}
		shown = append(shown, s)
	}
	return shown
}

// Average price per piece, by theme.
func averagePiecePrice(shown []Set) (cards Stats) {
	sum := map[Theme]USD{}
	n := map[Theme]int{}
	for _, s := range shown {
		sum[s.Theme] += s.piecePrice()
		n[s.Theme]++
	}
	for _, t := range sortedKeys(sum) {
		cards.Cards = append(cards.Cards, Card{
			Label:   string(t),
			Value:   fmt.Sprintf("$%.2f", float64(sum[t])/float64(n[t])),
			Caption: "Average piece price",
		})
	}
	return cards
}

// ---------------------------------------------------------------------------
// Chart, and the brush drawn on it
// ---------------------------------------------------------------------------

// Selection drawn on the scatter plot.
// The chart is an output; this is the input. The view layer overlays them —
// which is why a chart you can select on needs no two-way binding, and no cycle.
//
//notebook:brush on=scatter
func brush() (region Brush) { return Brush{} }

// Each set, plotted on the chosen axes.
//
//notebook:height=420
func scatter(shown []Set, x, y Select[Axis], trend bool, region Brush) (plot Chart) {
	plot = Chart{XLabel: x.Value.Name, YLabel: y.Value.Name, Region: region}
	byTheme := groupBy(shown, func(s Set) Theme { return s.Theme })
	for i, t := range sortedKeys(byTheme) {
		var pts []Pt
		for _, s := range byTheme[t] {
			pts = append(pts, Pt{X: x.Value.Of(s), Y: y.Value.Of(s)})
		}
		ser := Series{Theme: string(t), Color: palette(i), Points: pts}
		if trend {
			ser.Trend = loess(pts, 0.4)
		}
		plot.Series = append(plot.Series, ser)
	}
	return plot
}

// Sets inside the selection.
func selection(shown []Set, x, y Select[Axis], region Brush) (picked Table) {
	// A brush is stamped with the axes it was drawn against, so a stale brush is
	// representable rather than requiring the runtime to reset a leaf.
	if !region.Active || region.XName != x.Value.Name || region.YName != y.Value.Name {
		return Table{}
	}
	picked.Columns = []string{"set_id", "name", "theme", "image"}
	for _, s := range shown {
		if region.contains(x.Value.Of(s), y.Value.Of(s)) {
			picked.Rows = append(picked.Rows,
				[]string{s.ID, s.Name, string(s.Theme), s.Image})
		}
	}
	return picked
}

// Exploring price differences in Lego sets.
func intro() (md Markdown) {
	return `Lego produces sets across a wide range of themes and price points — not just
movie tie-ins like Star Wars or Harry Potter, but generic themes like City or Technic.

Are some themes simply more expensive? Does it track the piece count, or is there a
license fee buried in there? Pick some themes and find out.`
}

// ===========================================================================
// Helpers (undocumented => not cells)
// ===========================================================================

// axes is the column vocabulary: a name and the function that reads it.
// This list is the reason there is no way to typo an axis.
var axes = []Axis{
	{"year", func(s Set) float64 { return float64(s.Year) }},
	{"pieces", func(s Set) float64 { return float64(s.Pieces) }},
	{"price", func(s Set) float64 { return float64(s.Price) }},
	{"pieceprice", func(s Set) float64 { return float64(s.piecePrice()) }},
}

// loess fits a tricube-weighted local linear regression — the smoother behind
// Altair's transform_loess, in thirty lines, because it was never the hard part.
func loess(pts []Pt, bandwidth float64) []Pt {
	if len(pts) < 4 {
		return nil
	}
	in := slices.Clone(pts)
	slices.SortFunc(in, func(a, b Pt) int { return cmpFloat(a.X, b.X) })
	span := max(2, int(bandwidth*float64(len(in))))

	out := make([]Pt, 0, 60)
	lo, hi := in[0].X, in[len(in)-1].X
	for i := range 60 {
		x := lo + (hi-lo)*float64(i)/59
		nbrs := nearest(in, x, span)
		dmax := math.Max(math.Abs(nbrs[0].X-x), math.Abs(nbrs[len(nbrs)-1].X-x))
		if dmax == 0 {
			continue
		}
		var sw, swx, swy, swxx, swxy float64
		for _, p := range nbrs {
			w := tricube(math.Abs(p.X-x) / dmax)
			sw += w
			swx += w * p.X
			swy += w * p.Y
			swxx += w * p.X * p.X
			swxy += w * p.X * p.Y
		}
		den := sw*swxx - swx*swx
		if den == 0 {
			continue
		}
		b := (sw*swxy - swx*swy) / den
		a := (swy - b*swx) / sw
		out = append(out, Pt{X: x, Y: a + b*x})
	}
	return out
}

func tricube(u float64) float64 {
	if u >= 1 {
		return 0
	}
	t := 1 - u*u*u
	return t * t * t
}

// nearest returns the k points closest in x, still sorted by x.
func nearest(sorted []Pt, x float64, k int) []Pt {
	i, _ := slices.BinarySearchFunc(sorted, x, func(p Pt, x float64) int { return cmpFloat(p.X, x) })
	lo, hi := i, i
	for hi-lo < k && (lo > 0 || hi < len(sorted)) {
		switch {
		case lo == 0:
			hi++
		case hi == len(sorted):
			lo--
		case x-sorted[lo-1].X <= sorted[hi].X-x:
			lo--
		default:
			hi++
		}
	}
	return sorted[lo:hi]
}

func fetchCSV(url string) ([]map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rec, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil || len(rec) < 2 {
		return nil, fmt.Errorf("lego: reading %s: %w", url, err)
	}
	head := rec[0]
	out := make([]map[string]string, 0, len(rec)-1)
	for _, row := range rec[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(row) {
				m[strings.TrimSpace(h)] = row[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func svg(c Chart) string {
	const w, h, pad = 720.0, 420.0, 48.0
	xlo, xhi, ylo, yhi := c.extent()
	sx := func(v float64) float64 { return pad + (v-xlo)/(xhi-xlo)*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - (v-ylo)/(yhi-ylo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	if c.Region.Active {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" `+
			`fill="#4338ca" fill-opacity="0.08" stroke="#4338ca" stroke-dasharray="4 3"/>`,
			sx(c.Region.X1), sy(c.Region.Y2),
			sx(c.Region.X2)-sx(c.Region.X1), sy(c.Region.Y1)-sy(c.Region.Y2))
	}
	for _, s := range c.Series {
		op := "0.75"
		if s.Trend != nil {
			op = "0.2"
		}
		for _, p := range s.Points {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3" fill=%q fill-opacity=%q/>`,
				sx(p.X), sy(p.Y), s.Color, op)
		}
		if s.Trend != nil {
			var d strings.Builder
			for i, p := range s.Trend {
				fmt.Fprintf(&d, "%s%.1f %.1f", map[bool]string{true: "M", false: " L"}[i == 0], sx(p.X), sy(p.Y))
			}
			fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2"/>`, d.String(), s.Color)
		}
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="12">%s</text>`,
		w/2, h-12, c.XLabel)
	fmt.Fprintf(&b, `<text x="14" y="%.0f" font-family="sans-serif" font-size="12" `+
		`transform="rotate(-90 14 %.0f)">%s</text>`, h/2, h/2, c.YLabel)
	for i, s := range c.Series {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="4" fill=%q/>`+
			`<text x="%.0f" y="%.0f" font-family="sans-serif" font-size="11">%s</text>`,
			pad+8, 20+float64(i)*16, s.Color, pad+18, 24+float64(i)*16, s.Theme)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func distinct[T comparable, E any](in []E, key func(E) T) []T {
	seen := map[T]bool{}
	var out []T
	for _, e := range in {
		if k := key(e); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func groupBy[K comparable, E any](in []E, key func(E) K) map[K][]E {
	out := map[K][]E{}
	for _, e := range in {
		out[key(e)] = append(out[key(e)], e)
	}
	return out
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func maxOf(rows []Set, f func(Set) USD) USD {
	var m USD
	for _, s := range rows {
		m = max(m, f(s))
	}
	return m
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func palette(i int) string {
	c := []string{"#4338ca", "#c026d3", "#0891b2", "#ea580c", "#65a30d"}
	return c[i%len(c)]
}

// ===========================================================================
// Types. The "dataframe" is a slice of structs; the schema is the struct.
// ===========================================================================

type (
	USD    float64
	Factor float64
	Year   int
	Theme  string
)

type Set struct {
	ID, Name string
	Theme    Theme
	Year     Year
	Pieces   int
	Price    USD
	Image    string
}

func (s Set) piecePrice() USD { return s.Price / USD(s.Pieces) }
func (s Set) price() USD      { return s.Price }

// scale is the only way to apply a Factor to a USD, so `price * inflation`
// cannot be written by accident.
func (p USD) scale(f Factor) USD { return USD(float64(p) * float64(f)) }

func inflation(y Year) Factor { return Factor(math.Pow(1.03, float64(y-1970))) }

func (t Theme) Label() string { return string(t) }
func (a Axis) Label() string  { return a.Name }

type Axis struct {
	Name string
	Of   func(Set) float64
}

// ---- Widgets. Each is a plain value; the runtime finds it by method shape. ----

type Number interface{ ~int | ~float64 }

// Range is a bounded interval with a selection inside it. The cell supplies
// Lo/Hi (and an initial From/To); the head supplies From/To thereafter, clamped
// into Lo/Hi whenever the cell recomputes.
type Range[T Number] struct {
	Lo, Hi   T
	From, To T
}

func (r Range[T]) Bounds() (float64, float64) { return float64(r.Lo), float64(r.Hi) }

// WidgetView states a Range's live state for the client: its current selection
// [From,To] as the value, and its data-derived bounds. State only — no label,
// color, or step; the client decides how a range looks.
func (r Range[T]) WidgetView() WidgetView {
	lo, hi := float64(r.Lo), float64(r.Hi)
	return WidgetView{
		Value: []float64{float64(r.From), float64(r.To)},
		Lo:    &lo,
		Hi:    &hi,
	}
}

// Select and Multi carry their own options, which is what lets a cell compute
// them from data. Selections not present in All are dropped on reconcile.
type Select[T interface{ Label() string }] struct {
	All   []T
	Value T
}

func (s Select[T]) Options() []string { return labels(s.All) }

// WidgetView states a Select's live state: the current choice's label and the
// available option labels. The client selects by label; the runtime maps a
// chosen label back to a T. State only.
func (s Select[T]) WidgetView() WidgetView {
	return WidgetView{Value: s.Value.Label(), Options: labels(s.All)}
}

type Multi[T interface{ Label() string }] struct {
	All   []T
	Value []T
	Max   int
}

func (m Multi[T]) Options() []string { return labels(m.All) }

// WidgetView states a Multi's live state: the selected labels, the available
// option labels, and the selection cap. State only — no appearance.
func (m Multi[T]) WidgetView() WidgetView {
	wv := WidgetView{Value: labels(m.Value), Options: labels(m.All)}
	if m.Max > 0 {
		wv.Max = &m.Max
	}
	return wv
}

func labels[T interface{ Label() string }](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.Label()
	}
	return out
}

type Brush struct {
	Active         bool
	X1, Y1, X2, Y2 float64
	XName, YName   string // the axes this was drawn against
}

func (b Brush) contains(x, y float64) bool {
	return x >= b.X1 && x <= b.X2 && y >= b.Y1 && y <= b.Y2
}

// ---- Outputs. Anything with Render() Rendered draws as rich content. ----

type Rendered struct{ MIME, Data string }

// WidgetView is a widget's state on the wire — matched structurally by the
// runtime (like Rendered), so the notebook defines its own and imports nothing.
// State only: the current selection, the choices/bounds, hard constraints —
// never appearance. Each widget kind fills the fields it uses.
type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

type Pt struct{ X, Y float64 }

type Series struct {
	Theme  string
	Color  string
	Points []Pt
	Trend  []Pt
}

type Chart struct {
	XLabel, YLabel string
	Series         []Series
	Region         Brush
}

func (c Chart) extent() (xlo, xhi, ylo, yhi float64) {
	xlo, ylo = math.Inf(1), math.Inf(1)
	xhi, yhi = math.Inf(-1), math.Inf(-1)
	for _, s := range c.Series {
		for _, p := range s.Points {
			xlo, xhi = math.Min(xlo, p.X), math.Max(xhi, p.X)
			ylo, yhi = math.Min(ylo, p.Y), math.Max(yhi, p.Y)
		}
	}
	if xhi <= xlo {
		xhi = xlo + 1
	}
	if yhi <= ylo {
		yhi = ylo + 1
	}
	return xlo, xhi, ylo, yhi
}

func (c Chart) Render() Rendered { return Rendered{"image/svg+xml", svg(c)} }

type Card struct{ Label, Value, Caption string }
type Stats struct{ Cards []Card }

type Table struct {
	Columns []string
	Rows    [][]string
}

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
