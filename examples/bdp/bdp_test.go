package bdp

import (
	"math"
	"strings"
	"testing"
)

// fatLong is the archetype "fat long pipe": 10 Gb, 150 ms — a huge BDP the default
// window can't fill.
func fatLong() Link { return link(10000, 150) }

// TestBDPFormula: BDP = bandwidth × RTT, expressed in KB. Check against a hand value:
// 10000 Mbit/s × 0.150 s = 1.5e9 bits = 187.5 MB = 192000 KB.
func TestBDPFormula(t *testing.T) {
	got := float64(fatLong().bdp())
	want := 10000 * 1e6 * 0.150 / 8 / 1024 // bits → bytes → KB
	if math.Abs(got-want) > 1 {
		t.Errorf("BDP = %.1f KB, want %.1f", got, want)
	}
}

// TestWindowBelowBDPStarvesTheLink is THE teaching claim: a window smaller than the
// BDP is window-limited and achieves far less than line rate — the fat-long-pipe
// starvation. A 64 KB window on the 10 Gb/150 ms link should get a tiny fraction.
func TestWindowBelowBDPStarvesTheLink(t *testing.T) {
	l := fatLong()
	tp := float64(l.throughput(Kilobytes(64)))
	util := tp / float64(l.Bandwidth)
	if util > 0.01 {
		t.Errorf("64 KB window on a fat long pipe should starve it (<1%%), got %.1f%%", util*100)
	}
	// and it must be strictly window-limited (well below the BDP).
	if float64(l.bdp()) <= 64 {
		t.Fatalf("test premise broken: BDP %.0f KB should dwarf the 64 KB window", float64(l.bdp()))
	}
}

// TestWindowAtBDPFillsTheLink is the other half: once the window reaches the BDP, the
// link runs at (essentially) line rate — bandwidth-limited, not window-limited.
func TestWindowAtBDPFillsTheLink(t *testing.T) {
	l := fatLong()
	bdp := float64(l.bdp())
	tp := float64(l.throughput(Kilobytes(bdp)))
	if tp < float64(l.Bandwidth)*0.999 {
		t.Errorf("a window == BDP should fill the link: got %.1f of %.1f Mb/s", tp, float64(l.Bandwidth))
	}
}

// TestThroughputIsMinBandwidthOrWindow: throughput never exceeds bandwidth (a bigger
// window past the BDP buys nothing) and never exceeds window/RTT (a fatter link past
// the window buys nothing). The min() is the whole model.
func TestThroughputIsMinBandwidthOrWindow(t *testing.T) {
	l := fatLong()
	// window well past BDP: capped at bandwidth.
	big := float64(l.throughput(Kilobytes(float64(l.bdp()) * 4)))
	if big > float64(l.Bandwidth)+1e-6 {
		t.Errorf("throughput %.1f should never exceed bandwidth %.1f", big, float64(l.Bandwidth))
	}
	// tiny window: capped at window/RTT, independent of how fat the link is.
	small := float64(l.throughput(Kilobytes(16)))
	windowLimited := Kilobytes(16).bits() / l.RTT.seconds() / 1e6
	if math.Abs(small-windowLimited) > 1e-6 {
		t.Errorf("window-limited throughput %.3f should equal window/RTT %.3f", small, windowLimited)
	}
}

// TestBiggerWindowNeverHurts: throughput is monotonic non-decreasing in window size —
// the curve climbs then plateaus, it never dips. Guards a broken min/curve.
func TestBiggerWindowNeverHurts(t *testing.T) {
	l := fatLong()
	prev := -1.0
	for kb := 16.0; kb <= 262144; kb *= 2 {
		tp := float64(l.throughput(Kilobytes(kb)))
		if tp < prev-1e-9 {
			t.Errorf("throughput dipped as window grew: %.3f then %.3f at %.0f KB", prev, tp, kb)
		}
		prev = tp
	}
}

// TestShortLinkFillsWithSmallWindow: the contrast case — on a LAN (low RTT) the BDP is
// tiny, so even a small window nearly fills the link. Same window, opposite outcome —
// it's the RTT (via the BDP) that decides, not the window alone.
func TestShortLinkFillsWithSmallWindow(t *testing.T) {
	lan := link(1000, 1) // 1 Gb, 1 ms
	fat := link(1000, 200)
	win := Kilobytes(256)
	lanUtil := float64(lan.throughput(win)) / float64(lan.Bandwidth)
	fatUtil := float64(fat.throughput(win)) / float64(fat.Bandwidth)
	if !(lanUtil > fatUtil) {
		t.Errorf("same window should fill the short-RTT link better: LAN %.2f vs fat %.2f", lanUtil, fatUtil)
	}
	if lanUtil <= 0.9 {
		t.Errorf("a 256 KB window should nearly fill a 1 Gb/1 ms LAN, got %.2f", lanUtil)
	}
}

// TestViewsRender: the chart is SVG with its markers/labels; the verdict readout is
// HTML (else the client hides it — the pid/Readout lesson).
func TestViewsRender(t *testing.T) {
	r := analyze(fatLong(), 64)

	ch := curveChart(r).Render()
	if !strings.HasPrefix(ch.Data, "<svg") {
		t.Fatal("chart not SVG")
	}
	for _, w := range []string{"BDP", "line rate", "window-limited", "your window"} {
		if !strings.Contains(ch.Data, w) {
			t.Errorf("chart missing %q", w)
		}
	}

	vd := verdict(r).Render()
	if vd.MIME != "text/html" {
		t.Fatalf("verdict MIME = %q, want text/html", vd.MIME)
	}
	for _, w := range []string{"bandwidth-delay product", "your window", "throughput", "utilization"} {
		if !strings.Contains(vd.Data, w) {
			t.Errorf("verdict missing %q", w)
		}
	}
}
