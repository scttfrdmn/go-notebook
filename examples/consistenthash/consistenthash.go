//go:notebook
//
// Consistent hashing: add a server, move a few keys — not all of them.
//
// The naive way to shard keys across N servers is key % N. It works until N
// changes: add one server and almost EVERY key moves, because the modulus
// shifted under all of them. On a cache that is a stampede; on a database that is
// a migration. Consistent hashing places servers and keys on a ring; a key is
// owned by the next server clockwise. Add a server and only the keys in ONE arc —
// about 1/N of them — change hands. Everything else stays put.
//
// This notebook puts both on the ring and counts the difference. Drag the server
// and key counts; add a server and watch the "keys moved" readout: consistent
// hashing moves ~1/(N+1) of them, modulo moves nearly all. The ring is the
// picture; the count is the proof.
//
// Pure and fixed-horizon, like nbody/turing: everything is a function of (servers,
// keys, seed, +1?), so there is no hidden state — flip "add a server" back and
// forth and the ring is exactly reproducible. The hash is a small deterministic
// mixer (no crypto import, no clock), so the layout is stable across runs.
//
//notebook:layout intro
//notebook:layout knobs | ring
//notebook:layout verdict

package consistenthash

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs.
// ---------------------------------------------------------------------------

// Number of servers on the ring.
//
//notebook:slider min=2 max=12 step=1 area=knobs
func servers() (n int) { return 4 }

// Number of keys placed on the ring.
//
//notebook:slider min=10 max=400 step=10 area=knobs
func keys() (k int) { return 120 }

// Add one server? Toggle it to watch how many keys change owner — the whole
// point. 0 = the base ring, 1 = one server added.
//
//notebook:slider min=0 max=1 step=1 area=knobs
func addServer() (add int) { return 0 }

// Virtual nodes per server — replicas of each server around the ring. More
// replicas smooth the load imbalance (the other half of the real technique);
// with 1 the arcs are lumpy, with 8 they even out.
//
//notebook:slider min=1 max=16 step=1 area=knobs
func vnodes() (v int) { return 1 }

// ---------------------------------------------------------------------------
// The ring — pure functions of the inputs.
// ---------------------------------------------------------------------------

// The base ring: n servers (each with v virtual nodes) placed by hashing their
// names onto [0,1), plus k keys placed the same way. This is the arrangement
// before adding a server.
func baseRing(n, k, v int) (base Ring) {
	return buildRing(n, k, v)
}

// The grown ring: the SAME keys, but with `add` more servers. Because keys hash
// to the same positions, only the arcs the new server lands in change owner.
func grownRing(n, k, v, add int) (grown Ring) {
	return buildRing(n+add, k, v)
}

// How many keys changed owner between the base ring and the grown one — the
// consistent-hashing cost of adding a server. Compared against what plain
// key % N would have moved (almost everything).
func churn(base, grown Ring, k, n, add int) (moved Churn) {
	movedCount := 0
	for i := 0; i < len(base.KeyOwner) && i < len(grown.KeyOwner); i++ {
		if base.KeyOwner[i] != grown.KeyOwner[i] {
			movedCount++
		}
	}
	// Modulo baseline: with key % N, changing N reshuffles all but the keys whose
	// index happens to land on the same server — in expectation only k/lcm stay,
	// so essentially all move. Compute it exactly for honesty.
	moduloMoved := 0
	if add != 0 {
		for i := 0; i < k; i++ {
			if i%n != i%(n+add) {
				moduloMoved++
			}
		}
	}
	return Churn{
		Consistent: movedCount,
		Modulo:     moduloMoved,
		Keys:       k,
		Added:      add,
	}
}

// The ring drawing — an SVG of the servers (labelled ticks) and keys (dots),
// each key colored by its owning server, so an arc of one color is one server's
// share. This is a view of the grown ring.
//
//notebook:height=420 area=ring
func ringView(grown Ring, n, add int) (plot RingChart) {
	return RingChart{R: grown, Servers: n + add}
}

// The verdict: keys moved, consistent vs modulo, stated plainly.
//
//notebook:height=150 area=verdict
func verdict(moved Churn) (report Readout) {
	pctC := 0.0
	pctM := 0.0
	if moved.Keys > 0 {
		pctC = 100 * float64(moved.Consistent) / float64(moved.Keys)
		pctM = 100 * float64(moved.Modulo) / float64(moved.Keys)
	}
	cards := []Card{
		{Label: "keys on the ring", Value: itoa(moved.Keys)},
	}
	if moved.Added == 0 {
		cards = append(cards, Card{Label: "add a server", Value: "toggle it →", Caption: "to see the churn"})
	} else {
		cards = append(cards,
			Card{Label: "moved — consistent hashing", Value: itoa(moved.Consistent) + " (" + pct0(pctC) + "%)", Good: true},
			Card{Label: "moved — plain key % N", Value: itoa(moved.Modulo) + " (" + pct0(pctM) + "%)", Bad: true},
		)
	}
	return Readout{Cards: cards}
}

// Orientation.
func intro() (md Markdown) {
	return `## Consistent hashing

Sharding by ` + "`key % N`" + ` breaks when N changes: add a server and nearly every
key moves. Put servers and keys on a **ring** and a key belongs to the next server
clockwise — add a server and only one arc (~1/N of the keys) changes hands. Drag
the counts, then toggle **add a server** and compare the two "keys moved" lines.`
}

// ===========================================================================
// Ring construction + hashing. Helpers (unnamed returns) — not cells.
// ===========================================================================

// buildRing places servers (with virtual nodes) and keys on [0,1) by hashing
// their names, then assigns each key to the next virtual node clockwise (whose
// real server owns it). The ticks are returned sorted by position.
func buildRing(n, k, v int) Ring {
	ticks := make([]ServerTick, 0, n*v)
	for s := 0; s < n; s++ {
		for r := 0; r < v; r++ {
			ticks = append(ticks, ServerTick{Pos: hashUnit("server-" + itoa(s) + "-vn-" + itoa(r)), Server: s})
		}
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i].Pos < ticks[j].Pos })

	keyPos := make([]float64, k)
	owner := make([]int, k)
	for i := 0; i < k; i++ {
		keyPos[i] = hashUnit("key-" + itoa(i))
		owner[i] = ownerOf(keyPos[i], ticks)
	}
	return Ring{Servers: ticks, KeyPos: keyPos, KeyOwner: owner}
}

// ownerOf returns the real server id of the first virtual node clockwise from p,
// wrapping past 1.0 back to the first tick. ticks must be sorted by Pos.
func ownerOf(p float64, ticks []ServerTick) int {
	if len(ticks) == 0 {
		return 0
	}
	// First tick with Pos >= p owns it; if none, wrap to ticks[0].
	i := sort.Search(len(ticks), func(i int) bool { return ticks[i].Pos >= p })
	if i == len(ticks) {
		i = 0
	}
	return ticks[i].Server
}

// hashUnit maps a string to a stable float in [0,1), deterministic (no crypto,
// no clock) so the ring is reproducible across runs. FNV-1a spreads the bytes,
// then a splitmix64-style finalizer avalanches the result — without it, near-
// identical inputs ("server-3-vn-0" vs "server-4-vn-0") hash to nearby values
// and the ring clusters, which would fake pathological churn. The finalizer is
// what makes even one virtual node per server land evenly.
func hashUnit(s string) float64 {
	const (
		offset = 1469598103934665603
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	// splitmix64 finalizer — strong avalanche so similar strings scatter.
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	// Top 53 bits → [0,1).
	return float64(h>>11) / float64(uint64(1)<<53)
}

// itoa / pct0 / pct helpers.
func itoa(n int) string     { return strconv.Itoa(n) }
func pct0(f float64) string { return strconv.FormatFloat(f, 'f', 0, 64) }

// ===========================================================================
// Types.
// ===========================================================================

// ServerTick is one virtual node on the ring: its position and which real server.
type ServerTick struct {
	Pos    float64
	Server int
}

// Ring is the placed arrangement: server ticks (sorted), key positions, and each
// key's owning server id.
type Ring struct {
	Servers  []ServerTick
	KeyPos   []float64
	KeyOwner []int
}

// Churn counts how many keys moved when a server was added, consistent vs modulo.
type Churn struct {
	Consistent int
	Modulo     int
	Keys       int
	Added      int
}

// Card / Readout — the verdict panel.
type Card struct {
	Label, Value, Caption string
	Good, Bad             bool
}
type Readout struct{ Cards []Card }

func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:14px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		color := "#1b3a6b"
		if c.Good {
			color = "#0ca30c"
		} else if c.Bad {
			color = "#d03b3b"
		}
		b.WriteString(`<div style="flex:1;min-width:150px;border:1px solid #e7ebf0;border-radius:8px;padding:12px 14px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 22px/1.2 -apple-system,system-ui,sans-serif;color:%s;margin:2px 0;font-variant-numeric:tabular-nums">%s</div>`, color, c.Value)
		if c.Caption != "" {
			fmt.Fprintf(&b, `<div style="font-size:11px;color:#5b6472">%s</div>`, c.Caption)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

// RingChart draws the ring: a circle, server ticks, and keys colored by owner.
type RingChart struct {
	R       Ring
	Servers int
}

func (rc RingChart) Render() Rendered {
	const w, h = 420.0, 420.0
	cx, cy, rad := w/2, h/2, 150.0
	// A fixed palette of server colors (brand-anchored categorical set).
	palette := []string{"#2a78d6", "#0797b8", "#eda100", "#008300", "#4a3aa7", "#e34948", "#e87ba4", "#eb6834", "#5b6472", "#1b3a6b", "#0ca30c", "#d03b3b"}
	col := func(s int) string { return palette[s%len(palette)] }

	// position on the ring for a unit value (0 at top, clockwise).
	pt := func(u, r float64) (float64, float64) {
		ang := u*2*math.Pi - math.Pi/2
		return cx + r*math.Cos(ang), cy + r*math.Sin(ang)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" style="max-width:%.0fpx">`, w, h, w)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	// the ring
	fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#e7ebf0" stroke-width="10"/>`, cx, cy, rad)
	// keys as dots on the ring, colored by owner
	for i, p := range rc.R.KeyPos {
		x, y := pt(p, rad)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3.5" fill="%s" fill-opacity="0.85"/>`, x, y, col(rc.R.KeyOwner[i]))
	}
	// server ticks (larger, labelled) just outside the ring
	for _, s := range rc.R.Servers {
		x, y := pt(s.Pos, rad)
		lx, ly := pt(s.Pos, rad+22)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="7" fill="%s" stroke="#fff" stroke-width="2"/>`, x, y, col(s.Server))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" font-weight="700" fill="%s" text-anchor="middle" dominant-baseline="middle">S%d</text>`, lx, ly, col(s.Server), s.Server)
	}
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// Rendered / Markdown.
type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
