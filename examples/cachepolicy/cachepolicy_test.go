package cachepolicy

import (
	"strings"
	"testing"
)

const (
	wlWorkingSet = 0
	wlZipf       = 1
)

// TestStreamIsDeterministic: the access stream is a pure function of (workload, ws) —
// a seeded LCG, no clock, no math/rand global. Same inputs ⇒ byte-identical trace, so
// a policy comparison replays the SAME references (the anti-pass: only the policy
// differs between the two curves, never the traffic).
func TestStreamIsDeterministic(t *testing.T) {
	a := stream(wlWorkingSet, 50)
	b := stream(wlWorkingSet, 50)
	if len(a.Refs) != len(b.Refs) {
		t.Fatalf("lengths differ: %d vs %d", len(a.Refs), len(b.Refs))
	}
	for i := range a.Refs {
		if a.Refs[i] != b.Refs[i] {
			t.Fatalf("stream not deterministic at %d: %d vs %d", i, a.Refs[i], b.Refs[i])
		}
	}
}

// TestHitRatesAreReplayed: hit-rates are earned by running the eviction rule over the
// references, not a formula. Bound them to [0,1], and assert the structural truths a
// real replay must satisfy: capacity 0 ⇒ no hits; a cache as large as the universe ⇒
// no evictions, so hit-rate = (refs − distinct)/refs for both policies (they agree
// when nothing is ever evicted).
func TestHitRatesAreReplayed(t *testing.T) {
	refs := stream(wlWorkingSet, 50).Refs
	if r := hitRateLRU(refs, 0); r != 0 {
		t.Errorf("capacity 0 should give 0 hit-rate, got %.3f", r)
	}
	// With capacity ≥ universe nothing is ever evicted, so both policies hit exactly
	// the non-first references. They must agree.
	big := universe + 10
	lru := hitRateLRU(refs, big)
	lfu := hitRateLFU(refs, big)
	if lru != lfu {
		t.Errorf("with no evictions LRU and LFU must agree: %.4f vs %.4f", lru, lfu)
	}
	if lru <= 0 || lru >= 1 {
		t.Errorf("no-eviction hit-rate %.3f should be strictly in (0,1)", lru)
	}
}

// TestLRUWinsOnWorkingSet is THE teaching claim, half one: on a recency workload LRU
// beats LFU across cache sizes.
func TestLRUWinsOnWorkingSet(t *testing.T) {
	refs := stream(wlWorkingSet, 50).Refs
	for _, c := range []int{25, 50, 75, 100} {
		lru := hitRateLRU(refs, c)
		lfu := hitRateLFU(refs, c)
		if !(lru > lfu) {
			t.Errorf("working-set @ size %d: LRU (%.3f) should beat LFU (%.3f)", c, lru, lfu)
		}
	}
}

// TestLFUWinsOnZipf is THE teaching claim, half two: on a popularity workload LFU
// beats LRU across cache sizes. Together with the above, this is the winner-flip — the
// point of the notebook.
func TestLFUWinsOnZipf(t *testing.T) {
	refs := stream(wlZipf, 50).Refs
	for _, c := range []int{25, 50, 75, 100} {
		lru := hitRateLRU(refs, c)
		lfu := hitRateLFU(refs, c)
		if !(lfu > lru) {
			t.Errorf("Zipf @ size %d: LFU (%.3f) should beat LRU (%.3f)", c, lfu, lru)
		}
	}
}

// TestTheWinnerFlips states the headline directly: at a fixed cache size, flipping the
// workload flips which policy wins. If this ever fails, the notebook's whole thesis is
// wrong and the prose must change — not ship.
func TestTheWinnerFlips(t *testing.T) {
	const size = 50
	wsRefs := stream(wlWorkingSet, 50).Refs
	zRefs := stream(wlZipf, 50).Refs

	lruWinsWS := hitRateLRU(wsRefs, size) > hitRateLFU(wsRefs, size)
	lfuWinsZipf := hitRateLFU(zRefs, size) > hitRateLRU(zRefs, size)
	if !(lruWinsWS && lfuWinsZipf) {
		t.Errorf("winner should flip with workload: LRU wins WS=%v, LFU wins Zipf=%v", lruWinsWS, lfuWinsZipf)
	}
}

// TestWorkingSetCliffThenPlateau: LRU's hit-rate rises sharply as the cache grows to
// hold the working set, then plateaus — more cache buys little once the set fits. This
// is the "is 2× the cache worth it?" lesson; assert the plateau (doubling cache past
// the working set adds little) is real.
func TestWorkingSetCliffThenPlateau(t *testing.T) {
	const ws = 50
	refs := stream(wlWorkingSet, ws).Refs
	belowCliff := hitRateLRU(refs, ws/2)     // cache smaller than the set
	atCliff := hitRateLRU(refs, ws+ws/5)     // cache just fits the set
	doubled := hitRateLRU(refs, 2*(ws+ws/5)) // 2× the cache

	if !(atCliff-belowCliff > 0.15) {
		t.Errorf("expected a cliff: hit-rate should jump crossing the working set (%.3f → %.3f)", belowCliff, atCliff)
	}
	if doubled-atCliff > 0.10 {
		t.Errorf("expected a plateau: doubling cache past the set should add little (%.3f → %.3f)", atCliff, doubled)
	}
}

// TestViewsRender: the chart is SVG with both curves labeled; the verdict readout is
// HTML (else the client hides it — the pid/Readout lesson).
func TestViewsRender(t *testing.T) {
	cv := curves(stream(wlWorkingSet, 50))

	ch := rateChart(cv, 50).Render()
	if !strings.HasPrefix(ch.Data, "<svg") {
		t.Fatal("chart not SVG")
	}
	for _, w := range []string{"LRU", "LFU", "hit-rate vs cache size", "size 50"} {
		if !strings.Contains(ch.Data, w) {
			t.Errorf("chart missing %q", w)
		}
	}

	vd := verdict(cv, 50, wlWorkingSet).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html (else the client hides it)", vd.MIME)
	}
	for _, w := range []string{"workload", "LRU hit-rate", "LFU hit-rate", "winner"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
