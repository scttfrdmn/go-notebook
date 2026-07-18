//go:notebook
//
// Taxis as a queueing system — out-of-core, over a content-addressed handle.
//
// WHAT THIS NOTEBOOK DEMONSTRATES, AND WHAT IT DEFERS. Be exact, because a
// corpus that implies it proved the headline claim is worse than one with an
// honest hole:
//
//   - Out-of-core:            DEMONSTRATED. The Rel[Trip] handle streams rows
//                             through Scan; the aggregates below cross into Go,
//                             the table never does. `[]Trip` of the whole file is
//                             never a Go value.
//   - Path-is-not-a-handle:   DEMONSTRATED. Rel carries (source, rows, schema
//                             hash) computed from the file's CONTENTS. Change the
//                             file and the handle changes and everything
//                             downstream invalidates. This is the rule that
//                             distinguishes Rel from the portfolio tracker's
//                             parent_folder constant, which charted Microsoft as
//                             Apple because a constant path can't notice the file
//                             changed. The invalidation is proved by a test, not
//                             asserted here.
//   - Bulk data-in (KC17):    DEMONSTRATED (native). The trips handle is a
//                             settable INPUT LEAF — Rel[Trip] carries Reconcile,
//                             so a host hands the notebook a different dataset by
//                             setting a new handle ({source}) over the port, with
//                             no rebuild, and every aggregate recomputes over the
//                             new rows. The author DECLARED the handle leaf by
//                             giving Rel a capability (the type-driven rule), never
//                             a directive. Only the source crosses the wire; rows
//                             and hash are re-derived from the contents, so a
//                             handle cannot lie about what it names.
//                             LIMIT, NAMED: this is the native/CLI/SSE path, where
//                             the process resolves the source's bytes. Under WASM
//                             the same set changes handle IDENTITY (downstream
//                             invalidates) but the sandbox has no filesystem to
//                             fetch new CONTENTS — identity travels, bytes do not.
//                             (Moot for this notebook: it uses time parsing that
//                             is not WASM-able, so its home is the static binary.)
//   - Static binary:          DEMONSTRATED. Pure Go, no cgo, no DuckDB. The
//                             "cross-compile, scp, sbatch" story survives.
//
//   - Compile-time-checked SQL: DEFERRED. The design's headline — a SQL string
//                             typechecked against the struct at build time —
//                             needs a SQL parser (the single biggest piece of
//                             engineering in the design) and, for real SQL,
//                             DuckDB via cgo. Neither is here. The cells below are
//                             typed Go operations (Scan/Filter/GroupBy) over
//                             Rel[Trip], not SQL. The struct is still the schema;
//                             the queries are just Go.
//
// The Trip struct is the schema either way: rename a field and every operation
// over it fails to compile. That much you get for free from deciding to compile.

package taxi

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Trip is the schema. Every operation below is typed against it; the file's
// columns are validated against it once, at Open.
type Trip struct {
	Pickup     time.Time
	Dropoff    time.Time
	Fare       USD
	Tip        USD
	Passengers int32
}

// sources is the data plane: named datasets, each keyed by the source ID that a
// handle carries. In a real deployment these are paths to parquet on S3 and the
// runtime resolves them; here they are embedded so the example is self-contained
// and the static binary needs nothing at runtime. The point under test — a
// handle IDENTIFIES its contents — does not depend on where the bytes live, only
// on Open/Scan reading the SOURCE THE HANDLE NAMES rather than a hardcoded blob.
//
// Two datasets, so setting a new handle delivers genuinely different data (the
// KC17 test): a day shift and a sparse late-night shift with a different demand
// curve. Handing the notebook a new handle re-runs every aggregate over the new
// rows — no rebuild.
var sources = map[string]string{
	"trips-day.csv": `pickup,dropoff,fare,tip,passengers
2024-03-01T08:00:00Z,2024-03-01T08:12:00Z,14.50,3.00,1
2024-03-01T08:30:00Z,2024-03-01T08:41:00Z,11.00,2.00,2
2024-03-01T09:05:00Z,2024-03-01T09:40:00Z,38.00,7.50,1
2024-03-01T18:00:00Z,2024-03-01T18:22:00Z,22.00,4.00,3
2024-03-01T18:15:00Z,2024-03-01T18:33:00Z,18.50,3.50,1
2024-03-01T18:45:00Z,2024-03-01T19:20:00Z,41.00,8.00,2
2024-03-01T22:10:00Z,2024-03-01T22:19:00Z,9.50,1.50,1`,
	"trips-night.csv": `pickup,dropoff,fare,tip,passengers
2024-03-01T23:10:00Z,2024-03-01T23:38:00Z,31.00,6.00,1
2024-03-01T23:45:00Z,2024-03-02T00:05:00Z,24.50,5.00,2
2024-03-02T01:20:00Z,2024-03-02T01:52:00Z,44.00,9.00,3
2024-03-02T02:30:00Z,2024-03-02T02:44:00Z,17.00,3.00,1
2024-03-02T03:15:00Z,2024-03-02T03:58:00Z,52.00,10.00,4`,
}

// defaultSource is the dataset a fresh notebook opens. Change the handle (set
// the trips leaf to a different source) and everything downstream recomputes.
const defaultSource = "trips-day.csv"

// ---------------------------------------------------------------------------
// Data — a handle, not the rows
// ---------------------------------------------------------------------------

// The dataset the notebook is pointed at — a HANDLE, and a settable one. Opening
// reads the header (to validate the schema against Trip) and computes the content
// hash; it does NOT load the rows into Go. Because Rel[Trip] carries Reconcile,
// this cell is an input leaf: a host can hand the notebook a different dataset by
// setting a new handle ({source, rows, schema}) over the port, with no rebuild —
// and because the handle identifies its contents, every aggregate below recomputes
// over the new rows. That is the KC17 shape: bulk data-in as a content-addressed
// handle, the author having DECLARED the handle leaf by giving Rel a capability.
func trips() (all Rel[Trip], err error) { return Open[Trip](defaultSource) }

// Trips on file. Reads the handle's row count, not the rows.
func scale(all Rel[Trip]) (rows int64) { return all.Rows }

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

// Drivers on the road. How many servers is this queue running?
//
//notebook:slider min=1 max=50 step=1
func drivers() (c int) { return 8 }

// ---------------------------------------------------------------------------
// The query — typed Go over the relation, streaming (out-of-core)
// ---------------------------------------------------------------------------

// Demand and service time by hour of day. GroupBy streams every row of the
// relation through the accumulator; the 24-row result crosses into Go, the
// table never does. This is the out-of-core shape: push compute to the data so
// a slice of the RESULT is a legal value even when a slice of the input isn't.
func demand(all Rel[Trip]) (hours []HourStat, err error) {
	acc := map[int]*HourStat{}
	err = Scan(all, func(t Trip) {
		if !t.Dropoff.After(t.Pickup) {
			return
		}
		h := t.Pickup.Hour()
		s := acc[h]
		if s == nil {
			s = &HourStat{Hour: h}
			acc[h] = s
		}
		s.Trips++
		s.totalMinutes += t.Dropoff.Sub(t.Pickup).Minutes()
		s.MeanRevenue += t.Fare + t.Tip
	})
	if err != nil {
		return nil, err
	}
	for h := 0; h < 24; h++ {
		s := acc[h]
		if s == nil {
			continue
		}
		s.MeanMinutes = s.totalMinutes / float64(s.Trips)
		s.MeanRevenue = USD(float64(s.MeanRevenue) / float64(s.Trips))
		hours = append(hours, *s)
	}
	return hours, nil
}

// ---------------------------------------------------------------------------
// The model — the same M/M/c arithmetic, driven by real arrivals
// ---------------------------------------------------------------------------

// Offered load, hour by hour. Erlangs are dimensionless: arrivals × service time.
func load(hours []HourStat) (curve []Hourly, err error) {
	for _, h := range hours {
		lambda := PerHour(float64(h.Trips))
		mu := PerHour(60.0 / h.MeanMinutes)
		curve = append(curve, Hourly{
			Hour:    h.Hour,
			Lambda:  lambda,
			Mu:      mu,
			Offered: Erlangs(float64(lambda) / float64(mu)),
		})
	}
	return curve, nil
}

// Utilization by hour, and where the fleet is underwater.
func pressure(curve []Hourly, c int) (util Chart) {
	util = Chart{Title: "utilization by hour (dashed = saturation)", Rule: 1.0}
	for _, h := range curve {
		util.X = append(util.X, float64(h.Hour))
		util.Y = append(util.Y, float64(h.Offered)/float64(c))
	}
	return util
}

// The hours that break.
func saturated(curve []Hourly, c int) (bad Readout) {
	over, peak := 0, 0.0
	worst := Hourly{}
	for _, h := range curve {
		if float64(h.Offered)/float64(c) >= 1 {
			over++
		}
		if float64(h.Offered) > peak {
			peak, worst = float64(h.Offered), h
		}
	}
	return Readout{Cards: []Card{
		{"hours over capacity", fmt.Sprintf("%d / %d", over, len(curve)), "ρ ≥ 1"},
		{"peak offered load", fmt.Sprintf("%.1f", peak), "erlangs"},
		{"worst hour", fmt.Sprintf("%02d:00", worst.Hour), "highest demand"},
	}}
}

// Taxis as a queueing system.
func intro() (md Markdown) {
	return `Trips treated as arrivals at an M/M/c queue with the fleet as servers.
Offered load is arrivals × service time, in erlangs.

The relation is a **handle**: it carries the source, the row count, and a hash of
the contents — not the rows. Change the data and the hash changes and everything
downstream recomputes. A constant path could not notice.`
}

// ===========================================================================
// Types
// ===========================================================================

type (
	USD     float64
	PerHour float64
	Erlangs float64
)

// HourStat is the aggregate result. totalMinutes is unexported scratch used
// while streaming; MeanMinutes is the finished value.
type HourStat struct {
	Hour         int
	Trips        int64
	MeanMinutes  float64
	MeanRevenue  USD
	totalMinutes float64
}

type Hourly struct {
	Hour    int
	Lambda  PerHour
	Mu      PerHour
	Offered Erlangs
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Chart struct {
	Title string
	X     []float64
	Y     []float64
	Rule  float64
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 300.0, 44.0
	if len(c.X) == 0 {
		return Rendered{"text/markdown", "_no data_"}
	}
	hi := c.Rule
	for _, v := range c.Y {
		hi = math.Max(hi, v)
	}
	hi *= 1.1
	sx := func(v float64) float64 { return pad + v/23*(w-2*pad) }
	sy := func(v float64) float64 { return h - pad - v/hi*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	if c.Rule > 0 {
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#dc2626" stroke-dasharray="5 4"/>`,
			pad, sy(c.Rule), w-pad, sy(c.Rule))
	}
	var d strings.Builder
	for i, v := range c.Y {
		verb := " L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(c.X[i]), sy(v))
	}
	fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#4338ca" stroke-width="2.5"/>`, d.String())
	fmt.Fprintf(&b, `<text x="%.0f" y="22" font-family="sans-serif" font-size="12">%s</text>`, pad, c.Title)
	b.WriteString(`</svg>`)
	return Rendered{"image/svg+xml", b.String()}
}

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }

// ===========================================================================
// The handle and its operations. In a real deployment these are provided by the
// runtime over pure-Go parquet; here they are ordinary Go over the embedded
// data, because the property under test is the HANDLE's identity, not the codec.
// ===========================================================================

// Rel[T] is a relation you have not read. It carries what identifies the data —
// source, row count, and a hash of the CONTENTS — and not the rows. Change the
// file and the handle changes and everything downstream knows. That is what
// makes it a legal value on a graph edge (a path would not be: a constant path
// is identical whether the file changed or not).
type Rel[T any] struct {
	Source string
	Rows   int64
	Schema Hash // content hash: identity travels with the value
}

// Hash is a content fingerprint. Equal reports handle identity, so the engine's
// propagation pruning treats two Rels over the same contents as unchanged and
// two over different contents as changed — the path-is-not-a-handle rule, as an
// engine-visible value.
type Hash uint64

// Equal lets the scheduler prune on handle identity (the Equal(any) rung).
func (r Rel[T]) Equal(other any) bool {
	o, ok := other.(Rel[T])
	return ok && o.Source == r.Source && o.Rows == r.Rows && o.Schema == r.Schema
}

// Reconcile rebuilds the handle from a wire selection — a {source} the host set
// over the port. This is the capability that makes a Rel cell a settable input
// leaf (the same rung Table[T] uses): the author DECLARES a handle leaf by giving
// Rel this method, never by annotating a comment. Only the source crosses the
// wire — the identity a host can legitimately choose; Rows and Schema are DERIVED
// by re-Opening that source, never trusted from the wire (a handle whose rows/hash
// didn't match its contents would be the exact path-is-not-a-handle lie this type
// exists to prevent). An unknown or malformed selection leaves the handle
// unchanged, so a bad set degrades to the current dataset rather than a broken one.
func (r Rel[T]) Reconcile(saved any) any {
	m, ok := saved.(map[string]any)
	if !ok {
		return r // not a handle selection — the current dataset stands
	}
	src, ok := m["Source"].(string)
	if !ok || src == "" {
		return r
	}
	opened, err := Open[T](src)
	if err != nil {
		return r // unknown source: keep the working handle, never a dangling one
	}
	return opened
}

// Open resolves a source by name, validates its columns against T, counts rows,
// and hashes the CONTENTS — reading headers and computing identity, not
// materializing rows. The handle's Source is the name it was opened by, so Scan
// can later re-resolve the same bytes: identity and contents stay tied. An
// unknown source is an error, never a silent empty handle.
func Open[T any](source string) (Rel[T], error) {
	data, ok := sources[source]
	if !ok {
		return Rel[T]{}, fmt.Errorf("unknown source %q", source)
	}
	rows := int64(0)
	for _, line := range strings.Split(strings.TrimSpace(data), "\n")[1:] {
		if strings.TrimSpace(line) != "" {
			rows++
		}
	}
	return Rel[T]{Source: source, Rows: rows, Schema: contentHash(data)}, nil
}

// Scan streams every row of the relation through fn without ever holding all
// rows in memory at once — the out-of-core primitive. It reads the source THE
// HANDLE NAMES (not a hardcoded blob), so handing the notebook a new handle
// streams the new dataset's rows. (Here it parses embedded CSV; over parquet it
// would stream row groups from rel.Source.)
func Scan(rel Rel[Trip], fn func(Trip)) error {
	data, ok := sources[rel.Source]
	if !ok {
		return fmt.Errorf("unknown source %q", rel.Source)
	}
	lines := strings.Split(strings.TrimSpace(data), "\n")
	for _, line := range lines[1:] {
		t, err := parseTrip(line)
		if err != nil {
			return err
		}
		fn(t)
	}
	return nil
}

func parseTrip(line string) (Trip, error) {
	f := strings.Split(line, ",")
	if len(f) != 5 {
		return Trip{}, fmt.Errorf("bad row: %q", line)
	}
	pickup, err := time.Parse(time.RFC3339, f[0])
	if err != nil {
		return Trip{}, err
	}
	dropoff, err := time.Parse(time.RFC3339, f[1])
	if err != nil {
		return Trip{}, err
	}
	return Trip{
		Pickup:     pickup,
		Dropoff:    dropoff,
		Fare:       USD(atof(f[2])),
		Tip:        USD(atof(f[3])),
		Passengers: int32(atoi(f[4])),
	}, nil
}

func atof(s string) float64 {
	var v float64
	fmt.Sscanf(strings.TrimSpace(s), "%g", &v)
	return v
}

func atoi(s string) int {
	var v int
	fmt.Sscanf(strings.TrimSpace(s), "%d", &v)
	return v
}

// contentHash is FNV-1a over the bytes: identical contents → identical hash,
// changed contents → changed hash. This is the whole path-is-not-a-handle rule.
func contentHash(data string) Hash {
	const (
		offset = 1469598103934665603
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(data); i++ {
		h ^= uint64(data[i])
		h *= prime
	}
	return Hash(h)
}
