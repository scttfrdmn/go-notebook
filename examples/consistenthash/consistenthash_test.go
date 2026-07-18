package consistenthash

import (
	"strings"
	"testing"
)

// TestAddingServerMovesFewKeys is the headline claim, checked numerically:
// adding one server to an N-server ring moves roughly 1/(N+1) of the keys under
// consistent hashing, while plain key % N moves the large majority. If this
// stops holding, the notebook is lying.
func TestAddingServerMovesFewKeys(t *testing.T) {
	n, k, v := servers(), keys(), vnodes()
	base := buildRing(n, k, v)
	grown := buildRing(n+1, k, v)

	moved := 0
	for i := 0; i < k; i++ {
		if base.KeyOwner[i] != grown.KeyOwner[i] {
			moved++
		}
	}
	frac := float64(moved) / float64(k)
	// Ideal is 1/(N+1) ≈ 0.20 for N=4; allow generous slack for a finite ring.
	if frac > 0.45 {
		t.Errorf("consistent hashing moved %.0f%% of keys adding one server — want ~1/(N+1)≈20%%, not a reshuffle", 100*frac)
	}

	// Modulo baseline: key % N vs key % (N+1) moves the large majority.
	mm := 0
	for i := 0; i < k; i++ {
		if i%n != i%(n+1) {
			mm++
		}
	}
	if float64(mm)/float64(k) < 0.6 {
		t.Errorf("modulo baseline moved only %.0f%% — expected the large majority (that's the contrast)", 100*float64(mm)/float64(k))
	}
	if moved >= mm {
		t.Errorf("consistent hashing (%d moved) must beat modulo (%d moved) — that is the whole point", moved, mm)
	}
}

// TestKeysAreStableWithoutChange confirms that with no server added, no key
// moves — the ring is deterministic and the churn is exactly the added server's
// arc, nothing spurious.
func TestKeysAreStableWithoutChange(t *testing.T) {
	a := buildRing(4, 120, 1)
	b := buildRing(4, 120, 1)
	for i := range a.KeyOwner {
		if a.KeyOwner[i] != b.KeyOwner[i] {
			t.Fatalf("the ring is not deterministic: key %d owned by S%d then S%d", i, a.KeyOwner[i], b.KeyOwner[i])
		}
	}
}

// TestHashSpreadsEvenly guards the avalanche fix: with a single virtual node per
// server, the four servers should each own a non-trivial share — not one server
// swallowing the ring (the clustering bug that faked pathological churn).
func TestHashSpreadsEvenly(t *testing.T) {
	r := buildRing(4, 400, 1)
	count := map[int]int{}
	for _, o := range r.KeyOwner {
		count[o]++
	}
	if len(count) != 4 {
		t.Fatalf("only %d of 4 servers own any keys — the ring clustered", len(count))
	}
	for s, c := range count {
		if c == 0 || c > 300 {
			t.Errorf("server S%d owns %d/400 keys — badly imbalanced (hash not avalanching)", s, c)
		}
	}
}

// TestVerdictRenders confirms the churn readout reaches the page with both lines
// once a server is added (running is not passing).
func TestVerdictRenders(t *testing.T) {
	n, k, v := servers(), keys(), vnodes()
	base := buildRing(n, k, v)
	grown := buildRing(n+1, k, v)
	c := churn(base, grown, k, n, 1)
	html := verdict(c).Render()
	if html.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html", html.MIME)
	}
	for _, want := range []string{"consistent hashing", "key % N"} {
		if !strings.Contains(html.Data, want) {
			t.Errorf("verdict missing %q", want)
		}
	}
}
