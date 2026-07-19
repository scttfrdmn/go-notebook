//go:notebook
//
// slider — a bounded numeric input.
//
// A plain scalar input renders as a number box. The //notebook:slider directive
// refines that control into a ranged slider — it does NOT make the cell an input
// (its being a consumed parameterless scalar does). Delete the directive and
// `rate` is still an input, just a number box.
//
//	go tool notebook run ./examples/minimal/slider
//
// Demonstrates: scalar input, the slider directive. See docs/reference-controls.html.

package slider

// Requests per second.
//
//notebook:slider min=0 max=1000 step=10
func rate() (rps int) { return 200 }

// Requests per minute — derived.
func perMinute(rps int) (rpm int) { return rps * 60 }
