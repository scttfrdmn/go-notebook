//go:notebook
//
// checkbox — a boolean input.
//
// A parameterless cell whose result is a bool renders as a checkbox. No
// directive, no type: the bool kind is the whole signal.
//
//	go tool notebook run ./examples/minimal/checkbox
//
// Demonstrates: bool input -> checkbox. See docs/reference-controls.html.

package checkbox

// Include tax in the total?
func taxable() (on bool) { return true }

// Base price in cents.
//
//notebook:slider min=0 max=10000 step=100
func base() (cents int) { return 2000 }

// Total, with tax applied when the checkbox is on.
func total(cents int, on bool) (out int) {
	if on {
		return cents * 108 / 100
	}
	return cents
}
