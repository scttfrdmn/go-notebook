//go:notebook
//
// multiselect — many choices from a list.
//
// Options() with a *slice* Value renders as a multi (several choices) rather than
// a select (one). That is the only difference: scalar Value -> select, slice
// Value -> multi. Reconcile filters out any saved choice no longer offered.
//
//	go tool notebook run ./examples/minimal/multiselect
//
// Demonstrates: Options() + slice Value -> multi. See docs/reference-controls.html.

package multiselect

// Which regions to include.
func regions() (picks Multi) {
	return Multi{
		Value: []string{"us-east", "eu-west"},
		All:   []string{"us-east", "us-west", "eu-west", "ap-south"},
	}
}

// How many regions are selected.
func count(picks Multi) (n int) { return len(picks.Value) }

// Multi is a multi-choice widget: Options() + a slice Value.
type Multi struct {
	Value []string
	All   []string
}

func (m Multi) Options() []string { return m.All }

func (m Multi) Reconcile(saved any) any {
	sel, ok := saved.([]string)
	if !ok {
		return m
	}
	var kept []string
	for _, opt := range m.All {
		for _, s := range sel {
			if opt == s {
				kept = append(kept, opt)
			}
		}
	}
	return Multi{Value: kept, All: m.All}
}
