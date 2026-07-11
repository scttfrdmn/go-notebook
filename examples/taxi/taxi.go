//go:notebook
//
// Taxis as a queueing system.
//
// The last test, and the only one that could still break the design. It is two tests at
// once, because they are the same test: a SQL cell must return rows OF SOME GO TYPE, and
// if the data does not fit in memory then that type cannot be a slice of the rows.
//
// 42 million trips. None of them enter Go's heap.
//
// ---------------------------------------------------------------------------
// What flows along the edge
// ---------------------------------------------------------------------------
//
// Rel[Trip] is a HANDLE, not data. Which should set off an alarm, because a handle on an
// edge is exactly what made the portfolio tracker chart a portfolio of secretly-Microsoft
// stocks: `parent_folder` was the constant Path("invest-data"), identical whether the
// download succeeded, failed, or wrote the wrong company.
//
// The distinction is the whole thing, and it is sharper than I could state it before:
//
//     A PATH IS NOT A HANDLE. A handle IDENTIFIES ITS CONTENTS.
//
// Rel carries (source, size, schema hash). Change the file and the handle changes and
// everything downstream invalidates. Carry a constant path and the graph is blind. Both
// are "a reference on an edge"; only one of them is a value.
//
// ---------------------------------------------------------------------------
// What makes the SQL safe
// ---------------------------------------------------------------------------
//
// The Trip struct below IS the schema. It is not documentation and it is not a scan
// target — it is the thing the toolchain checks the SQL against at BUILD time:
//
//   - a column name that isn't a Trip field         → compile error
//   - a result struct that doesn't match the SELECT → compile error
//   - avg(fare) assigned to an int                  → compile error
//
// No data is needed to compile: the struct is the schema. The parquet file's ACTUAL
// schema is validated against Trip exactly once, at load, so a file that doesn't match
// is an error rather than a silently-wrong column. Three-way agreement between the
// struct, the query, and the file.
//
// marimo cannot do this — not "does not," cannot. There is no compile step to hang it on.
// This is the single strongest claim in the design and it is a straight consequence of
// the decision to compile, made ten decisions ago for entirely unrelated reasons.
//
// The honest limits are at the bottom, and one of them is the first real damage the
// static-binary story has taken.

package taxi

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Trip is the schema. Every SQL cell below is typechecked against it.
type Trip struct {
	Pickup     time.Time `parquet:"tpep_pickup_datetime"`
	Dropoff    time.Time `parquet:"tpep_dropoff_datetime"`
	Distance   Miles     `parquet:"trip_distance"`
	Fare       USD       `parquet:"fare_amount"`
	Tip        USD       `parquet:"tip_amount"`
	Passengers int32     `parquet:"passenger_count"`
	PickupZone int32     `parquet:"PULocationID"`
}

const trips2024 = "https://d37ci6vzurychx.cloudfront.net/trip-data/yellow_tripdata_2024-*.parquet"

// ---------------------------------------------------------------------------
// Data
// ---------------------------------------------------------------------------

// Every yellow-cab trip in 2024.
//
// Opening this reads the parquet footers — schema and row counts — and nothing else.
// The bytes stay where they are.
func trips() (all Rel[Trip], err error) { return Open[Trip](trips2024) }

// Trips on file.
func scale(all Rel[Trip]) (rows int64) { return all.Rows }

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

// Month.
func month() (m Select[Month]) { return Select[Month]{All: months, Value: months[2]} }

// Drivers on the road. How many servers is this queue running?
//
//notebook:slider min=1000 max=30000 step=500
func drivers() (c int) { return 12000 }

// ---------------------------------------------------------------------------
// The query
// ---------------------------------------------------------------------------

// Demand and service time, by hour of day.
//
// The SELECT touches 42M rows and returns 24. The aggregate crosses into Go; the table
// never does. Pushing the compute to where the data lives is not an optimization here,
// it is the only reason a slice of the result is a legal Go value at all.
//
// The toolchain parses this string at build time and checks every identifier against
// Trip, and the result columns against HourStat. `pickup_hour` is not a Trip field, so
// the query below would fail to compile if I typed it; `hour(Pickup)` resolves.
func demand(all Rel[Trip], m Select[Month]) (hours []HourStat, err error) {
	return Query[HourStat](all, `
		SELECT
			hour(Pickup)                                       AS Hour,
			count(*)                                           AS Trips,
			avg(epoch(Dropoff) - epoch(Pickup)) / 60.0         AS MeanMinutes,
			quantile_cont(epoch(Dropoff) - epoch(Pickup), 0.95) / 60.0 AS P95Minutes,
			avg(Fare + Tip)                                    AS MeanRevenue
		FROM trips
		WHERE month(Pickup) = ? AND Dropoff > Pickup
		GROUP BY 1
		ORDER BY 1`, m.Value.N)
}

// ---------------------------------------------------------------------------
// The model — the same M/M/c arithmetic, now driven by 42M real arrivals
// ---------------------------------------------------------------------------

// Offered load, hour by hour. Erlangs are dimensionless: arrivals × service time.
func load(hours []HourStat, m Select[Month]) (curve []Hourly, err error) {
	days := float64(m.Value.Days)
	for _, h := range hours {
		lambda := PerHour(float64(h.Trips) / days) // arrivals per hour, averaged over the month
		mu := PerHour(60.0 / h.MeanMinutes)        // trips per hour, per driver
		curve = append(curve, Hourly{
			Hour:    h.Hour,
			Lambda:  lambda,
			Mu:      mu,
			Offered: Erlangs(float64(lambda) / float64(mu)),
		})
	}
	return curve, nil
}

// Utilization, and where the fleet is underwater.
func pressure(curve []Hourly, c int) (plot Chart) {
	plot = Chart{Title: "utilization by hour (dashed = saturation)", Rule: 1.0}
	for _, h := range curve {
		plot.X = append(plot.X, float64(h.Hour))
		plot.Y = append(plot.Y, float64(h.Offered)/float64(c))
	}
	return plot
}

// The hours that break.
func saturated(curve []Hourly, c int) (bad Readout) {
	var worst Hourly
	over := 0
	for _, h := range curve {
		if rho := float64(h.Offered) / float64(c); rho >= 1 {
			over++
			if h.Offered > worst.Offered {
				worst = h
			}
		}
	}
	peak := 0.0
	for _, h := range curve {
		peak = math.Max(peak, float64(h.Offered))
	}
	return Readout{Cards: []Card{
		{"hours over capacity", fmt.Sprintf("%d / 24", over), "ρ ≥ 1"},
		{"peak offered load", fmt.Sprintf("%.0f", peak), "erlangs"},
		{"fleet needed", fmt.Sprintf("%.0f", math.Ceil(peak*1.15)), "at 87% peak utilization"},
		{"worst hour", fmt.Sprintf("%02d:00", worst.Hour), "highest demand"},
	}}
}

// Arrivals and service time through the day.
//
//notebook:height=300
func rhythm(curve []Hourly) (plot Chart) {
	plot = Chart{Title: "arrivals/hr (indigo, left) · minutes per trip (violet, right)", Dual: true}
	for _, h := range curve {
		plot.X = append(plot.X, float64(h.Hour))
		plot.Y = append(plot.Y, float64(h.Lambda))
		plot.Y2 = append(plot.Y2, 60/float64(h.Mu))
	}
	return plot
}

// A hundred trips, for a look.
//
// The one place rows cross into Go as rows. It is a cell like any other; the handle just
// gets asked for less this time.
func peek(all Rel[Trip], m Select[Month]) (sample []Trip, err error) {
	return Query[Trip](all, `SELECT * FROM trips WHERE month(Pickup) = ? LIMIT 100`, m.Value.N)
}

// Taxis as a queueing system.
func intro() (md Markdown) {
	return `Forty-two million trips, treated as arrivals at an M/M/c queue with the fleet as
servers. Offered load is arrivals × service time, in erlangs — dimensionless, and the same
number whether you are sizing taxis or compute nodes.

The parquet stays on S3. Twenty-four rows come back.`
}

// ===========================================================================
// Types
// ===========================================================================

type (
	USD     float64
	Miles   float64
	PerHour float64
	Erlangs float64
)

// HourStat is the query's result schema. The toolchain checks it against the SELECT.
//
// Domain types survive a bare column reference or a type-preserving aggregate
// (min/max/avg/sum of USD is USD). MeanMinutes is a computed expression, so it lands as
// float64 — the toolchain will not pretend to infer units through arbitrary arithmetic,
// and I would not trust it if it did.
type HourStat struct {
	Hour        int
	Trips       int64
	MeanMinutes float64
	P95Minutes  float64
	MeanRevenue USD
}

type Hourly struct {
	Hour    int
	Lambda  PerHour
	Mu      PerHour
	Offered Erlangs
}

type Month struct {
	Name string
	N    int
	Days int
}

func (m Month) Label() string { return m.Name }

var months = []Month{
	{"January", 1, 31}, {"February", 2, 29}, {"March", 3, 31}, {"April", 4, 30},
	{"May", 5, 31}, {"June", 6, 30}, {"July", 7, 31}, {"August", 8, 31},
	{"September", 9, 30}, {"October", 10, 31}, {"November", 11, 30}, {"December", 12, 31},
}

// ---- The handle ----

// Rel[T] is a relation you have not read. It carries what identifies the data —
// source, row count, and the hash of the schema it was validated against — and not the
// data itself. That is what makes it a legal value on a graph edge: change the file and
// the handle changes and everything downstream knows.
type Rel[T any] struct {
	Source string
	Rows   int64
	Schema Hash
}

type Hash [32]byte

// Open validates the file's actual schema against T and reads nothing else.
func Open[T any](source string) (Rel[T], error) { panic("provided by the runtime") }

// Query typechecks against Rel's schema at BUILD time and runs at the data.
func Query[R any, T any](rel Rel[T], sql string, args ...any) ([]R, error) {
	panic("provided by the runtime")
}

// ---- Widgets and output ----

type Select[T interface{ Label() string }] struct {
	All   []T
	Value T
}

func (s Select[T]) Options() []string {
	out := make([]string, len(s.All))
	for i, v := range s.All {
		out[i] = v.Label()
	}
	return out
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Chart struct {
	Title  string
	X      []float64
	Y, Y2  []float64
	Rule   float64 // horizontal reference line, if non-zero
	Dual   bool
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 300.0, 44.0
	if len(c.X) == 0 {
		return Rendered{"text/markdown", "_no data_"}
	}
	hi1, hi2 := c.Rule, 0.0
	for i := range c.X {
		hi1 = math.Max(hi1, c.Y[i])
		if c.Dual {
			hi2 = math.Max(hi2, c.Y2[i])
		}
	}
	hi1 *= 1.1
	hi2 *= 1.15

	sx := func(v float64) float64 { return pad + v/23*(w-2*pad) }
	sy := func(v, hi float64) float64 { return h - pad - v/hi*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	line := func(ys []float64, hi float64, color string) {
		var d strings.Builder
		for i, v := range ys {
			verb := " L"
			if i == 0 {
				verb = "M"
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(c.X[i]), sy(v, hi))
		}
		fmt.Fprintf(&b, `<path d=%q fill="none" stroke=%q stroke-width="2.5"/>`, d.String(), color)
	}
	if c.Rule > 0 {
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#dc2626" `+
			`stroke-dasharray="5 4"/>`, pad, sy(c.Rule, hi1), w-pad, sy(c.Rule, hi1))
	}
	if c.Dual {
		line(c.Y2, hi2, "#c026d3")
	}
	line(c.Y, hi1, "#4338ca")

	for _, t := range []int{0, 6, 12, 18, 23} {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.0f" font-family="sans-serif" font-size="10" `+
			`fill="#64748b" text-anchor="middle">%02d</text>`, sx(float64(t)), h-pad+16, t)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12">%s</text>`,
		pad, c.Title)
	b.WriteString(`</svg>`)
	return Rendered{"image/svg+xml", b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
