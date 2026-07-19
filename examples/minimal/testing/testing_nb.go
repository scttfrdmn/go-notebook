//go:notebook
//
// testing — a notebook is ordinary Go, so its cells have ordinary unit tests.
//
// Because a cell is just a top-level function, you test it by calling it — no
// kernel, no notebook runtime, no fixture. `go test ./examples/minimal/testing`
// runs the tests in testing_test.go against the cells below. This is a real
// differentiator from stateful notebook formats: the computation is testable the
// same way any Go function is.
//
//	go tool notebook run ./examples/minimal/testing    # interactive
//	go test ./examples/minimal/testing                 # test the cells
//
// Demonstrates: cells as plain functions, unit-testing a notebook.

package testing

// A principal amount, in dollars.
//
//notebook:slider min=0 max=100000 step=1000
func principal() (amount float64) { return 1000 }

// Annual interest rate (0.05 = 5%).
//
//notebook:slider min=0 max=0.2 step=0.01
func rate() (apr float64) { return 0.05 }

// Compound interest after one year.
func afterOneYear(amount, apr float64) (total float64) { return amount * (1 + apr) }

// The interest earned — a second cell to test.
func interest(amount, total float64) (earned float64) { return total - amount }
