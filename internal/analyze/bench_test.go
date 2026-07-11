package analyze

import "testing"

// BenchmarkAnalyzeCapacity measures KC1: full graph derivation on capacity.go.
// The kill-criteria target is < 50 ms. Note this includes go/packages loading,
// which shells out to `go list`; see the KC1 note reported on the milestone.
func BenchmarkAnalyzeCapacity(b *testing.B) {
	const dir = "testdata/graphs/capacity"
	// Warm the build/analysis cache once so the benchmark measures steady-state
	// derivation, not a cold `go list`.
	if _, _, err := (TypesAnalyzer{}).Analyze(dir); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := (TypesAnalyzer{}).Analyze(dir); err != nil {
			b.Fatal(err)
		}
	}
}
