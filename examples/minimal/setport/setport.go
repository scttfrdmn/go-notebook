//go:notebook
//
// setport — the minimal live feed: one value in, one derived value out.
//
// A feed is not a cell. A feed is a program (a driver) that writes a leaf through
// the notebook's one data-in port as data arrives; the notebook stays pure cells
// that react. `n` is an ordinary input leaf — the same kind a slider drives —
// except the hand on the knob is the driver in ./driver, which POSTs a new value
// every second. Everything downstream (`squared`) is pure.
//
//	go tool notebook run ./examples/minimal/setport --no-open   # serve it
//	go run ./examples/minimal/setport/driver                    # drive it
//
// Then watch `squared` climb on the page (or GET /events). This is the whole
// pattern the sensorfeed/tickerfeed/apifeed examples scale up. See docs/live-feeds.html.

package setport

// The latest value from the feed. Its default is 0; the driver overrides it via
// the set port as data arrives. A slider lets you also drive it by hand.
//
//notebook:slider min=0 max=100 step=1
func n() (v int) { return 0 }

// The square of the latest value — pure, recomputed on every new v.
func squared(v int) (sq int) { return v * v }
