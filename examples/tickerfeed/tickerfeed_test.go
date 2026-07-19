package tickerfeed

import "testing"

// TestMovingAvgAndSpread pins the two derived analytics on a known window.
func TestMovingAvgAndSpread(t *testing.T) {
	s := Series{CSV: "100,200,300"}
	if got := movingAvg(s); got != 200 {
		t.Errorf("movingAvg = %d, want 200", got)
	}
	if got := spread(s); got != 200 {
		t.Errorf("spread = %d, want 200 (300-100)", got)
	}
	// Empty window is safe (no divide-by-zero) — the feed hasn't started.
	if movingAvg(Series{}) != 0 || spread(Series{}) != 0 {
		t.Error("empty series should yield 0, not panic")
	}
}

// TestSeriesReconcile confirms the driver can set the rolling window as a plain
// string over the wire, and a non-string selection is ignored (keeps the value).
func TestSeriesReconcile(t *testing.T) {
	got := Series{}.Reconcile("1,2,3").(Series)
	if got.CSV != "1,2,3" {
		t.Errorf("Reconcile(string) = %q, want 1,2,3", got.CSV)
	}
	kept := Series{CSV: "keep"}
	if kept.Reconcile(42).(Series).CSV != "keep" {
		t.Error("Reconcile of a non-string should leave the window unchanged")
	}
}

// TestDollarsFormatsCents pins the cents→$ formatting with thousands separators.
func TestDollarsFormatsCents(t *testing.T) {
	if got := dollars(6500000); got != "$65,000.00" {
		t.Errorf("dollars(6500000) = %q, want $65,000.00", got)
	}
}
