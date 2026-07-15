//go:notebook
//
// Fleet cost and carbon — the two numbers on the procurement request.
//
// You're sizing a compute fleet for a month of jobs. Two numbers decide whether it's
// approved: what it costs, and what it emits. They come from the same fleet but they
// are NOT the same knob, and conflating them is the mistake this notebook exists to
// prevent:
//
//   - **Cost** depends on what you *pay per hour* — and spot instances are far
//     cheaper than on-demand, so shifting the mix toward spot drops the bill fast.
//   - **Carbon** depends on *energy used* — power drawn × hours run × the grid's
//     carbon intensity. Spot vs on-demand is the same silicon drawing the same
//     watts, so **moving to spot saves money and changes emissions not at all.**
//     The only levers on carbon are fewer node-hours, less power, or a cleaner grid.
//
// That decoupling is the point, and the units enforce it. `USD` comes from a
// `USDPerHour` times `Hours`; `KgCO2` comes from `Kilowatts` times `Hours` times a
// `KgCO2PerKWh` grid factor. There is no path from a price to an emission — you
// cannot accidentally make the cheaper fleet look greener, because dollars and
// kilograms are different types and no arithmetic crosses them. The lego port's
// dollars-times-dollars bug is unwriteable here; this is that lesson as a tool a
// capacity planner actually files.
//
// It is dead-center the project's niche — systems/cluster procurement — and it's the
// callable-model story: drag the sliders here, or run the same file headless as
// `fleet --headless --set nodes=512 --json` to drop the two numbers straight into a
// budget spreadsheet. Pure arithmetic, WASM-live.

package fleet

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Fleet size — number of compute nodes.
//
//notebook:slider min=8 max=1024 step=8
func nodes() (n int) { return 256 }

// Fraction of the fleet on spot instances, in percent. Spot is cheaper but
// preemptible; the rest is on-demand. Cost lever only — it does not touch carbon.
//
//notebook:slider min=0 max=100 step=5
func spotPct() (spot int) { return 60 }

// Hours the fleet runs this month (per node).
//
//notebook:slider min=24 max=744 step=24
func hoursPerMonth() (hrs int) { return 336 }

// Grid carbon intensity, in grams CO₂ per kWh. Coal-heavy grids are ~800; a clean
// grid (hydro/nuclear/wind) can be under 50. The real carbon lever.
//
//notebook:slider min=20 max=800 step=10
func gridCO2() (g int) { return 350 }

// ---------------------------------------------------------------------------
// Fixed fleet characteristics (an h7i-class HPC node).
// ---------------------------------------------------------------------------

// On-demand price per node-hour.
func onDemandRate() (od USDPerHour) { return 3.06 }

// Spot price per node-hour — the discount that makes the cost lever.
func spotRate() (sp USDPerHour) { return 1.10 }

// Node power draw under load, in kilowatts (CPU + memory + share of cooling/PSU).
func nodePower() (p Kilowatts) { return 0.55 }

// ---------------------------------------------------------------------------
// Compute (Go) — cost and carbon, in units.
// ---------------------------------------------------------------------------

// The bill: node-hours split by the spot fraction, each priced at its rate, summed.
// Cost is a USD — USDPerHour × Hours — and the spot mix is the only thing that moves
// it. Pure.
func cost(n int, spot int, hrs int, od USDPerHour, sp USDPerHour) (bill Bill) {
	nodeHours := Hours(hrs) // per node
	spotNodes := float64(n) * float64(spot) / 100
	odNodes := float64(n) - spotNodes

	spotCost := sp.over(nodeHours).times(spotNodes)
	odCost := od.over(nodeHours).times(odNodes)
	return Bill{Total: spotCost + odCost, Spot: spotCost, OnDemand: odCost}
}

// The emissions: total energy (power × node-hours across the whole fleet) times the
// grid's carbon intensity. Note what's ABSENT — the spot fraction and the prices.
// Carbon is a KgCO2 and there is no way to derive it from the Bill; it comes from
// energy alone. Pure.
func carbon(n int, hrs int, p Kilowatts, g int) (em Emissions) {
	energy := p.over(Hours(hrs)).times(float64(n)) // kWh across the fleet
	grid := KgCO2PerKWh(float64(g) / 1000)         // g/kWh → kg/kWh
	return Emissions{Energy: energy, Total: grid.times(energy)}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The two numbers a request needs, side by side, with a breakdown: monthly cost
// (split spot vs on-demand) and monthly carbon (with the energy behind it). Drag the
// spot slider and watch cost move while carbon sits perfectly still — the decoupling,
// made visible.
//
//notebook:height=320
func summary(bill Bill, em Emissions) (chart Chart) {
	return Chart{Bill: bill, Em: em}
}

// The procurement line items, as they'd appear on the request.
func request(bill Bill, em Emissions, spot int) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "monthly cost", Value: money(bill.Total), Caption: pct(float64(spot)/100) + " spot — the only cost lever here"},
		{Label: "monthly carbon", Value: tonnes(em.Total), Caption: "energy × grid intensity — spot doesn't touch this"},
		{Label: "energy", Value: f0(float64(em.Energy)) + " kWh"},
		{Label: "on-demand vs spot", Value: money(bill.OnDemand) + " / " + money(bill.Spot)},
	}}
}

// Fleet cost and carbon — the two numbers on the procurement request.
func intro() (md Markdown) {
	return `Sizing a fleet for a month of jobs. Two numbers get it approved: **cost**
and **carbon** — and they're different knobs. Drag the **spot fraction**: the bill
drops (spot is cheap), but the carbon doesn't move a gram, because it's the same
silicon drawing the same watts. Carbon only responds to **node-hours**, **power**, or
a **cleaner grid**.

The units enforce the separation: cost is ` + "`USDPerHour × Hours`" + `, carbon is
` + "`Kilowatts × Hours × KgCO2PerKWh`" + `, and no arithmetic crosses dollars and
kilograms — you *can't* make the cheaper fleet look greener by accident. Same file,
callable headless (` + "`fleet --headless --set nodes=512 --json`" + `) to drop both
numbers into a budget. Pure; scrub freely.`
}

// ===========================================================================
// Units — cost and carbon live in separate type universes on purpose.
// ===========================================================================

type (
	USD         float64 // dollars
	USDPerHour  float64 // a price rate
	Hours       float64 // a duration
	Kilowatts   float64 // power draw
	KWh         float64 // energy
	KgCO2       float64 // emissions
	KgCO2PerKWh float64 // grid carbon intensity
)

// over: a price rate across a duration is a per-node cost (USD); the only way to turn
// a USDPerHour into money.
func (r USDPerHour) over(h Hours) USD { return USD(float64(r) * float64(h)) }

// times scales a per-node USD by a node count.
func (u USD) times(nodes float64) USD { return USD(float64(u) * nodes) }

// over: power across a duration is energy (kWh) — for one node.
func (p Kilowatts) over(h Hours) KWh { return KWh(float64(p) * float64(h)) }

// times scales per-node energy by the fleet size.
func (e KWh) times(nodes float64) KWh { return KWh(float64(e) * nodes) }

// times: a grid intensity applied to energy is emissions (KgCO2). This is the ONLY
// bridge from energy to carbon, and nothing bridges cost to carbon at all.
func (g KgCO2PerKWh) times(e KWh) KgCO2 { return KgCO2(float64(g) * float64(e)) }

// ===========================================================================
// Helpers
// ===========================================================================

func f0(v float64) string  { return strconv.FormatFloat(v, 'f', 0, 64) }
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

func money(u USD) string {
	v := float64(u)
	switch {
	case v >= 1e6:
		return "$" + strconv.FormatFloat(v/1e6, 'f', 2, 64) + "M"
	case v >= 1e3:
		return "$" + strconv.FormatFloat(v/1e3, 'f', 1, 64) + "k"
	default:
		return "$" + strconv.FormatFloat(v, 'f', 0, 64)
	}
}

func tonnes(k KgCO2) string {
	t := float64(k) / 1000
	return strconv.FormatFloat(t, 'f', 1, 64) + " t CO₂"
}

// ===========================================================================
// Types
// ===========================================================================

type Bill struct {
	Total, Spot, OnDemand USD
}

type Emissions struct {
	Energy KWh
	Total  KgCO2
}

// Chart draws the two headline numbers as labeled bars with breakdowns.
type Chart struct {
	Bill Bill
	Em   Emissions
}

func (c Chart) Render() Rendered {
	const w, h, pad = 720.0, 320.0, 40.0

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	// Two panels: cost (left) and carbon (right).
	panelW := (w - 2*pad - 40) / 2

	// --- cost panel: a stacked bar, on-demand + spot ---
	cx := pad
	drawPanel := func(x float64, title, big, sub string, accent string) {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="13" fill="#5b6472">%s</text>`, x, 40.0, title)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="34" font-weight="700" fill=%q>%s</text>`, x, 84.0, accent, big)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" fill="#64748b">%s</text>`, x, 108.0, sub)
	}
	drawPanel(cx, "monthly cost", money(c.Bill.Total), "spot vs on-demand mix", "#1b3a6b")
	drawPanel(cx+panelW+40, "monthly carbon", tonnes(c.Em.Total), f0(float64(c.Em.Energy))+" kWh of energy", "#237a2b")

	// cost stacked bar (spot green portion, on-demand navy).
	barY, barH := 150.0, 40.0
	total := float64(c.Bill.Total)
	if total > 0 {
		odFrac := float64(c.Bill.OnDemand) / total
		odW := odFrac * (w - 2*pad)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#1b3a6b"/>`, pad, barY, odW, barH)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#3fa845"/>`, pad+odW, barY, (w-2*pad)-odW, barH)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#fff">on-demand %s</text>`, pad+6, barY+24, money(c.Bill.OnDemand))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#fff" text-anchor="end">spot %s</text>`, w-pad-6, barY+24, money(c.Bill.Spot))
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" fill="#64748b">the bar splits the bill by pricing; carbon has no such split — it's one number set by energy and the grid.</text>`,
		pad, barY+80)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
