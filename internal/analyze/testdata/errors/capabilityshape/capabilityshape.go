//go:notebook
//
// Control-capability methods named right but shaped wrong. Each is recognized by
// name as an intent to be that control, but its signature fails the engine's
// runtime interface probe (engine.Bounded/Optioned/Reconciler), so the control
// would silently never appear. check reports the near-miss instead — the input
// side of the render-shape check, for Bounds/Options/Reconcile.

package capabilityshape

// Rate has a Bounds method, but returns ints — a range control needs float64.
type Rate struct{ Value int }

func (r Rate) Bounds() (int, int) { return 0, 100 }

// rate is a range-ish leaf with the wrong Bounds signature.
func rate() (r Rate) { return Rate{Value: 50} }

// Mode has an Options method, but returns []int — a select needs []string.
type Mode struct{ Value int }

func (m Mode) Options() []int { return []int{1, 2, 3} }

// mode is a select-ish leaf with the wrong Options signature.
func mode() (m Mode) { return Mode{Value: 1} }

// consumer keeps rate and mode wired so they are real cells.
func consumer(r Rate, m Mode) (out int) { return r.Value + m.Value }
