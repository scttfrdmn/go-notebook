//go:notebook
//
// Portfolio tracker.
//
// A port of marimo's gallery dashboard (notebooks/dashboard/portfolio.py). Enter your
// buys; see what they're worth.
//
// The original has a spectacular bug:
//
//     yf.Ticker("MSFT").history(period="72mo")      # hardcoded
//        .assign(Ticker=ticker)                     # relabeled as whatever you asked for
//        .to_csv(f"{parent_folder}/{ticker}.csv")
//
// Every ticker downloads Microsoft's price history and writes it to AAPL.csv, labeled
// AAPL. The tracker charts a portfolio in which every stock is secretly Microsoft. And
// because the fetch is guarded by `if not (...).exists()`, once the wrong file lands it
// is never re-downloaded. The bug is sticky.
//
// A type would not have caught that: Ticker("MSFT") is a perfectly good Ticker. So it is
// worth asking what DID enable it, because the answer is a design flaw and not a typo.
//
// Look at what flows along the graph edge. `download_tickers` returns None. It writes
// files. The next cell depends on `parent_folder` — which is the constant Path("invest-
// data"). That value is IDENTICAL whether the download succeeded, failed, fetched the
// wrong company, or did nothing at all. The reactive graph is being used to SEQUENCE A
// SIDE EFFECT, and the edge carries a token instead of data.
//
// Once the edge is a path, a directory is your cache, `exists()` is your invalidation
// policy, and a wrong file is indistinguishable from a right one. Every downstream
// guarantee the notebook makes is void, and the graph cannot tell.
//
// The fix is not vigilance. It is that `prices` RETURNS THE PRICES:
//
//     func prices(lots []Lot) (bars Prices, err error)
//
// Now the edge carries the bars. Memoization keys on the tickers, not on a filename, so
// a stale file cannot exist to be trusted. There is no directory, no exists() check, no
// glob. And the fetch is visibly a function of the ticker, because it has to return
// something that depends on it.
//
// Second, quieter bug, also fixed here: the original joins investments to prices on an
// exact date string. Buy on a Saturday and the left join finds no row, Investment fills
// to 0, and your money silently disappears. Here, matching a buy to a trading day either
// succeeds or is an error.

package portfolio

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Cells
// ---------------------------------------------------------------------------

// Your investments. Edit the table directly.
func holdings() (lots Table[Lot]) {
	return Table[Lot]{Value: []Lot{
		{Date: day("2021-02-01"), Ticker: "MSFT", Amount: 500},
		{Date: day("2023-02-01"), Ticker: "AAPL", Amount: 800},
		{Date: day("2024-02-01"), Ticker: "AAPL", Amount: 200},
	}}
}

// Daily closes for every ticker you hold.
//
// This cell RETURNS the prices. That single fact is the whole difference from the
// original: there is no folder, no CSV cache, no exists() check. Change a ticker and
// the memo key changes; there is no filename to go stale behind your back.
func prices(lots Table[Lot]) (bars Prices, err error) {
	bars = Prices{}
	for _, t := range tickers(lots.Value) {
		series, err := dailyCloses(t)
		if err != nil {
			return nil, fmt.Errorf("portfolio: %s: %w", t, err)
		}
		bars[t] = series
	}
	return bars, nil
}

// Portfolio value, day by day.
//
// A cumulative-shares fold that isn't a fold: shares held is a sum over the buys, and a
// sum is a sufficient statistic, so this is an ordinary pure cell over the whole series.
func performance(lots Table[Lot], bars Prices) (series []Snapshot, err error) {
	// Shares bought, resolved against a real trading day. The original's exact-date
	// join drops a weekend purchase in silence; this refuses to.
	type buy struct {
		on     Date
		ticker Ticker
		shares Shares
		cost   USD
	}
	var buys []buy
	for _, l := range lots.Value {
		b, ok := bars[l.Ticker].onOrAfter(l.Date)
		if !ok {
			return nil, fmt.Errorf("portfolio: no trading day on or after %s for %s",
				l.Date, l.Ticker)
		}
		buys = append(buys, buy{b.Date, l.Ticker, l.Amount.buy(b.Close), l.Amount})
	}

	for _, d := range tradingDays(bars) {
		snap := Snapshot{Date: d}
		held := map[Ticker]Shares{}
		for _, b := range buys {
			if !b.on.After(d) {
				held[b.ticker] += b.shares
				snap.Invested += b.cost
			}
		}
		if snap.Invested == 0 {
			continue // nothing owned yet
		}
		for t, sh := range held {
			if b, ok := bars[t].onOrBefore(d); ok {
				snap.Value += sh.valueAt(b.Close)
			}
		}
		snap.PnL = snap.Value - snap.Invested
		snap.Return = Pct(float64(snap.Value)/float64(snap.Invested) - 1)
		series = append(series, snap)
	}
	return series, nil
}

// Investment value over time.
//
//notebook:height=300
func valueChart(series []Snapshot) (plot Chart) {
	plot = Chart{Title: "value (indigo) vs. invested (dashed)", Unit: "$"}
	for _, s := range series {
		plot.A = append(plot.A, float64(s.Value))
		plot.B = append(plot.B, float64(s.Invested))
	}
	return plot
}

// Returns over time.
//
//notebook:height=220
func returnChart(series []Snapshot) (returns Chart) {
	returns = Chart{Title: "total return", Unit: "%", Zero: true}
	for _, s := range series {
		returns.A = append(returns.A, float64(s.Return)*100)
	}
	return returns
}

// Where you stand.
func summary(series []Snapshot) (now Readout) {
	if len(series) == 0 {
		return now
	}
	s := series[len(series)-1]
	return Readout{Cards: []Card{
		{"invested", fmt.Sprintf("$%.0f", float64(s.Invested)), "total cost basis"},
		{"value", fmt.Sprintf("$%.0f", float64(s.Value)), "at last close"},
		{"P&L", fmt.Sprintf("$%+.0f", float64(s.PnL)), "unrealized"},
		{"return", fmt.Sprintf("%+.1f%%", float64(s.Return)*100), "money-weighted"},
	}}
}

// Portfolio tracker.
func intro() (md Markdown) {
	return `Fill in your buys below. Prices are daily closes going back six years.

Returns are money-weighted: each purchase buys shares at that day's close, and the
portfolio is worth whatever those shares are worth now.`
}

// ===========================================================================
// Data
// ===========================================================================

// dailyCloses is the source seam for a ticker's price series. It has two paths
// that answer the same shape (Bars) so that `prices` stays a RETURNED value —
// the whole point of this notebook — and only the SOURCE changes underneath it:
//
//   - LIVE (default): fetchDaily hits Stooq. This is the honest path. "This
//     notebook fetches real prices" is a claim, and a fixture masquerading as
//     live would turn that claim into a lie — so the live path is what runs
//     unless you explicitly ask otherwise, and when Stooq is down it ERRORS.
//     It does not silently fall back to the fixture; a silent substitution is
//     exactly the failure this project is named after (the system produced a
//     thing, and the thing that reached you wasn't the thing it claimed).
//
//   - FIXTURE (PORTFOLIO_PRICES=fixture): reads checked-in monthly closes from
//     prices.csv. This is the REPRODUCIBLE path — deterministic, offline, and
//     what CI and `notebook run` verification use, because a corpus notebook's
//     interactivity can't be gated on a third party's bot detector (Stooq began
//     serving a JS bot-challenge page instead of CSV; see #96).
//
// The split is the compute/view seam in a new place: deterministic for the path
// that must reproduce, live for the path that must stay honest. Two different
// requirements that were always wearing the same fetch call.
func dailyCloses(t Ticker) (Bars, error) {
	if strings.EqualFold(os.Getenv("PORTFOLIO_PRICES"), "fixture") {
		return fixtureDaily(t)
	}
	return fetchDaily(t)
}

//go:embed prices.csv
var fixtureCSV embed.FS

// fixtureDaily serves a ticker's closes from the checked-in prices.csv — a
// coarse (monthly) but real-shaped series, enough to drive the graph to real
// numbers offline. It errors on an unknown ticker rather than returning an
// empty series, mirroring fetchDaily: a missing ticker is a fault, not a
// silent zero (the same discipline the whole notebook is about).
func fixtureDaily(t Ticker) (Bars, error) {
	f, err := fixtureCSV.Open("prices.csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rec, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rec) < 2 {
		return nil, fmt.Errorf("no fixture price data for %s", t)
	}
	want := Ticker(strings.ToUpper(string(t)))

	var out Bars
	for _, r := range rec[1:] { // Ticker,Date,Close
		if Ticker(strings.ToUpper(r[0])) != want {
			continue
		}
		d, err := time.Parse("2006-01-02", r[1])
		if err != nil {
			continue
		}
		close, err := strconv.ParseFloat(r[2], 64)
		if err != nil {
			continue
		}
		out = append(out, Bar{Date: Date(d), Close: Price(close)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no fixture price data for %s", t)
	}
	slices.SortFunc(out, func(a, b Bar) int { return a.Date.cmp(b.Date) })
	return out, nil
}

// fetchDaily pulls daily closes from Stooq, which serves plain CSV and needs no key.
// Note that the ticker is an ARGUMENT to the URL. It has to be — the function's return
// value depends on it. That is the property the original quietly lacked.
func fetchDaily(t Ticker) (Bars, error) {
	url := fmt.Sprintf("https://stooq.com/q/d/l/?s=%s.us&i=d", strings.ToLower(string(t)))
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rec, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil || len(rec) < 2 {
		return nil, fmt.Errorf("no price data for %s", t)
	}
	cutoff := time.Now().AddDate(-6, 0, 0)

	var out Bars
	for _, r := range rec[1:] { // Date,Open,High,Low,Close,Volume
		d, err := time.Parse("2006-01-02", r[0])
		if err != nil || d.Before(cutoff) {
			continue
		}
		close, err := strconv.ParseFloat(r[4], 64)
		if err != nil {
			continue
		}
		out = append(out, Bar{Date: Date(d), Close: Price(close)})
	}
	slices.SortFunc(out, func(a, b Bar) int { return a.Date.cmp(b.Date) })
	return out, nil
}

func tickers(lots []Lot) []Ticker {
	seen := map[Ticker]bool{}
	var out []Ticker
	for _, l := range lots {
		if t := Ticker(strings.ToUpper(string(l.Ticker))); !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

func tradingDays(bars Prices) []Date {
	seen := map[Date]bool{}
	var out []Date
	for _, series := range bars {
		for _, b := range series {
			if !seen[b.Date] {
				seen[b.Date] = true
				out = append(out, b.Date)
			}
		}
	}
	slices.SortFunc(out, func(a, b Date) int { return a.cmp(b) })
	return out
}

// ===========================================================================
// Types. The units are the point.
// ===========================================================================

type (
	Ticker string
	USD    float64 // money
	Price  float64 // money PER SHARE — a different thing, and the compiler knows
	Shares float64
	Pct    float64
	Date   time.Time
)

// The only two ways money and shares may meet. `shares * close` will not compile;
// you have to say which conversion you meant.
func (u USD) buy(p Price) Shares     { return Shares(float64(u) / float64(p)) }
func (s Shares) valueAt(p Price) USD { return USD(float64(s) * float64(p)) }

type Lot struct {
	Date   Date
	Ticker Ticker
	Amount USD
}

type Bar struct {
	Date  Date
	Close Price
}

type Bars []Bar
type Prices map[Ticker]Bars

// onOrAfter resolves a purchase date to a real trading day. A weekend buy moves to
// Monday; a buy past the end of the data is an error, not a silent zero.
func (b Bars) onOrAfter(d Date) (Bar, bool) {
	for _, x := range b {
		if !x.Date.Before(d) {
			return x, true
		}
	}
	return Bar{}, false
}

func (b Bars) onOrBefore(d Date) (Bar, bool) {
	for i := len(b) - 1; i >= 0; i-- {
		if !b[i].Date.After(d) {
			return b[i], true
		}
	}
	return Bar{}, false
}

func (d Date) Before(o Date) bool { return time.Time(d).Before(time.Time(o)) }
func (d Date) After(o Date) bool  { return time.Time(d).After(time.Time(o)) }
func (d Date) String() string     { return time.Time(d).Format("2006-01-02") }

// MarshalJSON / UnmarshalJSON put a Date on the wire as the plain "2006-01-02"
// string the grid shows and the user types — NOT time.Time's default RFC3339
// timestamp, which the editable cell would neither display nor accept. This is
// what lets a Table[Lot] row round-trip through the coercer and Reconcile: the
// client sends {"Date":"2021-02-01",...}, Reconcile's json.Unmarshal decodes it
// back to a Date, and an edit to the date column survives.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*d = day(s)
	return nil
}
func (d Date) cmp(o Date) int {
	switch {
	case d.Before(o):
		return -1
	case d.After(o):
		return 1
	}
	return 0
}

func day(s string) Date {
	t, _ := time.Parse("2006-01-02", s)
	return Date(t)
}

type Snapshot struct {
	Date     Date
	Invested USD
	Value    USD
	PnL      USD
	Return   Pct
}

// ---- Widgets ----

// WidgetView is a widget's state on the wire — the notebook's own copy of the
// shape the runtime probes for (no import of this project; the match is by field
// shape). A Table only ever sets Value (its rows); the other fields exist so the
// struct matches the probe's shape across the zero-import boundary.
type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

// Table is an editable grid. Same leaf discipline as every other widget: the cell
// supplies the starting rows, the head holds your edits.
type Table[T any] struct {
	Value []T
}

// WidgetView states a Table's live state for the client: its rows, as the value.
// The client draws a grid from the static column schema (the row type's fields,
// derived at codegen) and fills it from these rows; editing a cell re-emits the
// whole row set. State only — the rows ARE the state a table holds, and its
// columns are appearance the client already knows from the type.
func (t Table[T]) WidgetView() WidgetView {
	return WidgetView{Value: t.Value}
}

// Reconcile REBUILDS the rows from the saved selection, resetting to the cell's
// default rows when the selection can't be decoded. This is the Table's entry in
// the per-widget reconcile taxonomy, and it's a deliberately different shape from
// the others: a Table[T]'s schema is its COLUMNS, and columns are the row type T
// — fixed at compile time, so they cannot shift under a live selection the way a
// Range's data-derived bounds can. There is no "a column appeared" case to clamp
// or filter against within a session. So reconcile guards the ONE thing that can
// go wrong: the wire→T boundary. The selection arrives as []map[string]any (rows
// the user typed); we round-trip it through each field's own JSON codec into []T,
// which keeps the rows if they decode and RESETS to the default rows if they
// don't. "A row survives a field-type change" is incoherent in the same way
// "control point #3 of a quintic is control point #3 of a septic" was — a reset,
// not partial retention, is the coherent answer.
func (t Table[T]) Reconcile(saved any) any {
	rows, ok := saved.([]map[string]any)
	if !ok {
		return t // selection isn't a row set — the cell's default rows stand
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return t
	}
	var out []T
	if err := json.Unmarshal(b, &out); err != nil {
		return t // a row didn't fit the row type — reset, never keep stale rows
	}
	t.Value = out
	return t
}

// ---- Outputs ----

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Chart struct {
	Title string
	Unit  string
	Zero  bool // draw a baseline at zero
	A, B  []float64
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 300.0, 44.0
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, xs := range [][]float64{c.A, c.B} {
		for _, v := range xs {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if c.Zero {
		lo = math.Min(lo, 0)
	}
	if hi <= lo {
		hi = lo + 1
	}
	sy := func(v float64) float64 { return h - pad - (v-lo)/(hi-lo)*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	line := func(ys []float64, color, dash string) {
		if len(ys) < 2 {
			return
		}
		var d strings.Builder
		for i, v := range ys {
			x := pad + float64(i)/float64(len(ys)-1)*(w-2*pad)
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, x, sy(v))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2" stroke-dasharray=%q/>`,
			d.String(), color, dash)
	}
	if c.Zero && lo < 0 {
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#cbd5e1"/>`,
			pad, sy(0), w-pad, sy(0))
	}
	line(c.B, "#0f172a", "5 4")
	line(c.A, "#4338ca", "")
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12">%s</text>`,
		pad, c.Title)
	fmt.Fprintf(&b, `<text x="%.0f" y="%.1f" font-family="sans-serif" font-size="10" `+
		`fill="#64748b">%s%.0f</text>`, 6.0, sy(hi)+4, c.Unit, hi)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
