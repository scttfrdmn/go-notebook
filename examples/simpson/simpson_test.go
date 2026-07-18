package simpson

import (
	"strings"
	"testing"
)

// TestParadoxFiresOnDefaults is the teaching claim, checked numerically: on the
// real 1986 kidney-stone figures, Treatment A wins BOTH subgroups yet loses the
// pool — Simpson's paradox. If this ever stops holding, the notebook's headline
// is a lie.
func TestParadoxFiresOnDefaults(t *testing.T) {
	aS, aL := aSmallRate(), aLargeRate()
	bS, bL := bSmallRate(), bLargeRate()
	aOv := aPooled(aSmallCases(), aLargeCases(), aS, aL)
	bOv := bPooled(bSmallCases(), bLargeCases(), bS, bL)

	if !(aS > bS) || !(aL > bL) {
		t.Fatalf("A must win both subgroups: small %v>%v, large %v>%v", aS, bS, aL, bL)
	}
	if !(aOv < bOv) {
		t.Errorf("pooled: A %.1f should LOSE to B %.1f (the paradox)", float64(aOv), float64(bOv))
	}
	v := paradox(aS, aL, bS, bL, aOv, bOv)
	if !v.Reversed {
		t.Error("verdict should report Reversed=true on the default (paradoxical) mix")
	}
}

// TestNoParadoxWhenMixMatches confirms the paradox is a property of the MIX, not
// the rates: give both treatments the SAME case mix and the reversal must vanish
// (the pooled winner agrees with the subgroups). This is what makes the sliders
// meaningful — the paradox switches off.
func TestNoParadoxWhenMixMatches(t *testing.T) {
	aS, aL := aSmallRate(), aLargeRate()
	bS, bL := bSmallRate(), bLargeRate()
	// Identical mix for both treatments (200 easy, 200 hard each).
	aOv := aPooled(200, 200, aS, aL)
	bOv := bPooled(200, 200, bS, bL)
	if aOv <= bOv {
		t.Errorf("with equal mix, A (%.1f) should out-pool B (%.1f) — no reversal", float64(aOv), float64(bOv))
	}
	if paradox(aS, aL, bS, bL, aOv, bOv).Reversed {
		t.Error("equal mix must NOT be reported as a paradox")
	}
}

// TestPooledIsCaseWeighted pins the pooling math: the overall rate is the
// case-weighted average of the group rates, not the simple mean.
func TestPooledIsCaseWeighted(t *testing.T) {
	// 100 cases at 90%, 300 at 50% → (100·90 + 300·50)/400 = 60, not 70.
	got := pool(100, 300, 90, 50)
	if float64(got) < 59.9 || float64(got) > 60.1 {
		t.Errorf("weighted pool = %.2f, want 60.0 (not the simple mean 70)", float64(got))
	}
}

// TestTableRendersReversal confirms the HTML view carries the figures and the
// verdict — the design is deferred to HTML, but the reveal must reach the page.
func TestTableRendersReversal(t *testing.T) {
	aS, aL := aSmallRate(), aLargeRate()
	bS, bL := bSmallRate(), bLargeRate()
	aOv := aPooled(aSmallCases(), aLargeCases(), aS, aL)
	bOv := bPooled(bSmallCases(), bLargeCases(), bS, bL)
	v := paradox(aS, aL, bS, bL, aOv, bOv)
	tbl := table(aSmallCases(), aLargeCases(), bSmallCases(), bLargeCases(), aS, aL, bS, bL, aOv, bOv, v)

	html := tbl.Render()
	if html.MIME != "text/html" {
		t.Fatalf("table MIME = %q, want text/html", html.MIME)
	}
	for _, want := range []string{"pooled", "93.0%", "82.9%", "Simpson's paradox"} {
		if !strings.Contains(html.Data, want) {
			t.Errorf("rendered table missing %q", want)
		}
	}
}
