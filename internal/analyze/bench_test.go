package analyze

import "testing"

const capacityDir = "testdata/graphs/capacity"

// BenchmarkKC1ColdAnalyze measures KC1: a full, cold graph derivation via
// packages.Load (graph mode, no NeedDeps). The relaxed target is < 1s — a cold
// load happens once at launch, hidden behind the browser opening, and is not
// the interactive path.
func BenchmarkKC1ColdAnalyze(b *testing.B) {
	if _, _, err := (TypesAnalyzer{}).Analyze(capacityDir); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := (TypesAnalyzer{}).Analyze(capacityDir); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKC2Reanalyze measures KC2 — the number this milestone exists to
// produce: re-analysis after a one-cell edit. The Session is primed once
// (outside the timed loop), then each iteration re-parses and re-typechecks the
// notebook package against the cached importer, exactly as an edit would.
// Target: < 100 ms.
func BenchmarkKC2Reanalyze(b *testing.B) {
	sess, err := NewSession(capacityDir)
	if err != nil {
		b.Fatal(err)
	}
	if _, _, err := sess.Reanalyze(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := sess.Reanalyze(); err != nil {
			b.Fatal(err)
		}
	}
}
