package testing

import "testing"

// TestAfterOneYear calls the cell directly — it is an ordinary function. No
// notebook runtime is involved; this is exactly how you'd test any Go function.
func TestAfterOneYear(t *testing.T) {
	if got := afterOneYear(1000, 0.05); got != 1050 {
		t.Errorf("afterOneYear(1000, 0.05) = %v, want 1050", got)
	}
}

// TestInterest tests a downstream cell in isolation by supplying its inputs — the
// same values an upstream cell would have produced. Testing a chain is just
// feeding one cell's output into the next.
func TestInterest(t *testing.T) {
	amount := 1000.0
	total := afterOneYear(amount, 0.05)
	if got := interest(amount, total); got != 50 {
		t.Errorf("interest = %v, want 50", got)
	}
}

// TestZeroRate pins an edge case: no interest at a zero rate.
func TestZeroRate(t *testing.T) {
	if got := afterOneYear(500, 0); got != 500 {
		t.Errorf("afterOneYear(500, 0) = %v, want 500 (no growth)", got)
	}
}
