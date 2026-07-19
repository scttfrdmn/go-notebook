//go:notebook
//
// selectbox — one choice from a list.
//
// A type carrying an Options() []string method and a scalar Value renders as a
// select. The engine probes the method structurally — no import, no registration.
// Reconcile keeps the current choice across a recompute (falling back to the
// default if the saved choice is gone).
//
//	go tool notebook run ./examples/minimal/selectbox
//
// Demonstrates: Options() -> select, Reconcile for state. See docs/reference-controls.html.

package selectbox

// Which environment to size for.
func env() (choice Pick) {
	return Pick{Value: "staging", All: []string{"dev", "staging", "prod"}}
}

// A multiplier that depends on the chosen environment.
func replicas(choice Pick) (n int) {
	switch choice.Value {
	case "prod":
		return 6
	case "staging":
		return 2
	default:
		return 1
	}
}

// Pick is a single-choice widget: Options() makes it a select; Value is the
// current choice; Reconcile keeps it across a wave.
type Pick struct {
	Value string
	All   []string
}

func (p Pick) Options() []string { return p.All }

func (p Pick) Reconcile(saved any) any {
	if s, ok := saved.(string); ok {
		for _, opt := range p.All {
			if opt == s {
				return Pick{Value: s, All: p.All}
			}
		}
	}
	return p
}
