//go:notebook
//
// A notebook with a missing producer: utilization needs `a Erlangs`, but no
// cell produces it (offeredLoad produces `load Erlangs` under a different name).

package missing

type Erlangs float64

// Offered load, under the wrong name.
func offeredLoad() (load Erlangs) { return 40 }

// Server utilization. Needs `a Erlangs`, which no cell produces — but a
// same-type near-miss exists, so the diagnostic should suggest offeredLoad.
func utilization(a Erlangs, c int) (rho float64) { return float64(a) / float64(c) }

// Fleet size.
func servers() (c int) { return 80 }
