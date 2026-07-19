//go:notebook
//
// headless — the same notebook as an interactive app and a batch job.
//
// The differentiating property: one Go file is a browser app AND a schedulable
// binary. Run it interactively, or build it and invoke it from a shell with
// --headless, overriding inputs with --set and printing values with --json. The
// same computation, two front ends.
//
// --set keys are RESULT names, not function names — the result name IS the edge
// (see docs/names.html). Here the function `principal` produces the result
// `amount`, so you override it with --set amount=…, not --set principal=….
//
//	go tool notebook run   ./examples/minimal/headless      # interactive
//	go tool notebook build -o loan ./examples/minimal/headless
//	./loan --headless --json                                # run once, print
//	./loan --headless --set amount=250000 --set apr=0.065 --json
//
// Demonstrates: --headless/--set/--json, the CLI<->notebook equivalence.

package headless

// Loan principal, in dollars.
//
//notebook:slider min=10000 max=1000000 step=10000
func principal() (amount float64) { return 300000 }

// Annual interest rate (e.g. 0.06 = 6%).
//
//notebook:slider min=0.01 max=0.15 step=0.005
func rate() (apr float64) { return 0.06 }

// Loan term, in years.
//
//notebook:slider min=5 max=40 step=5
func years() (n int) { return 30 }

// The monthly payment (standard amortization formula).
func payment(amount, apr float64, n int) (monthly float64) {
	months := float64(n * 12)
	r := apr / 12
	if r == 0 {
		return amount / months
	}
	f := pow1p(r, months)
	return amount * r * f / (f - 1)
}

// pow1p computes (1+r)^k — an ordinary helper, invisible to the graph.
func pow1p(r, k float64) float64 {
	out := 1.0
	for i := 0.0; i < k; i++ {
		out *= 1 + r
	}
	return out
}
