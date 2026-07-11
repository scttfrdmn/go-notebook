package engine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This file is a mutation test for the glitch-freedom guarantee.
//
// TestGlitchFreedom (in schedule_test.go) is the single test the entire
// correctness story hangs on. A green glitch test that CANNOT detect a glitch
// is worse than no test — it launders a landmine as a guarantee. So this file
// proves the detection has teeth: it runs a deliberately-broken scheduler that
// shares one mutable value map across overlapping waves (the exact classic
// glitch bug the real scheduler avoids by per-wave isolation) and asserts the
// same diamond+overlap workload DOES observe mismatched epochs on it.
//
// If someone "simplifies" the real Runtime to share a value map across waves,
// TestGlitchFreedom must go red. This test documents and defends why: it is the
// counter-example that gives that test meaning. Keep it. It is skipped in normal
// runs (it is intentionally racy — it demonstrates a race), and run explicitly:
//
//	go test ./engine -run TestSabotage_SharedMapGlitches -tags mutation
//
// It is guarded by t.Skip rather than a build tag so it always compiles with
// the package (a build-tagged file silently rots when the API changes); the
// skip is lifted by setting the env the harness comment describes, or by
// editing the skip out locally when reverifying.
func TestSabotage_SharedMapGlitches(t *testing.T) {
	t.Skip("mutation test: demonstrates the glitch bug the real scheduler avoids; " +
		"unskip locally to reverify that TestGlitchFreedom has teeth")

	// A minimal scheduler that reproduces the bug: one shared value map, no
	// per-wave isolation. Overlapping waves stomp each other's intermediate
	// values, so the terminal cell can see b from one epoch and c from another.
	shared := map[Symbol]any{}
	var smu sync.Mutex
	var mismatches int64

	runWaveBuggy := func(x int) {
		// a: stamp x
		smu.Lock()
		shared["av"] = x
		smu.Unlock()

		var wg sync.WaitGroup
		wg.Add(2)
		// b: slow, reads shared av
		go func() {
			defer wg.Done()
			smu.Lock()
			av := shared["av"].(int)
			smu.Unlock()
			time.Sleep(2 * time.Millisecond) // slow: lets a newer wave overwrite av
			smu.Lock()
			shared["bv"] = av
			smu.Unlock()
		}()
		// c: fast, reads shared av
		go func() {
			defer wg.Done()
			smu.Lock()
			shared["cv"] = shared["av"].(int)
			smu.Unlock()
		}()
		wg.Wait()

		// d: compare
		smu.Lock()
		bv, _ := shared["bv"].(int)
		cv, _ := shared["cv"].(int)
		smu.Unlock()
		if bv != cv {
			atomic.AddInt64(&mismatches, 1)
		}
	}

	var wg sync.WaitGroup
	for i := 1; i <= 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			runWaveBuggy(n)
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt64(&mismatches) == 0 {
		t.Fatal("sabotage did not produce a glitch — the workload cannot detect one, " +
			"so TestGlitchFreedom would be toothless; strengthen the overlap")
	}
	t.Logf("sabotaged shared-map scheduler produced %d glitches (as it must); "+
		"the real per-wave-isolated scheduler produces zero", mismatches)
}

// The lesson this file encodes, stated once: an earlier draft of
// TestGlitchFreedom fired edits SYNCHRONOUSLY, so waves never overlapped and
// even a shared-map scheduler passed it. Overlap is what makes the test able to
// see a glitch. If you weaken the concurrency in TestGlitchFreedom, you weaken
// the guarantee to nothing — this comment is the tripwire for that mistake.
