package spectrogram

import (
	"strings"
	"testing"
)

func rect() Select[Window] {
	return Select[Window]{All: []Window{Rectangular, Hann}, Value: Rectangular}
}
func hann() Select[Window] { return Select[Window]{All: []Window{Rectangular, Hann}, Value: Hann} }

// TestSignalPure confirms the signal is a pure function of the sliders.
func TestSignalPure(t *testing.T) {
	a := signal(40, 64, 0)
	b := signal(40, 64, 0)
	for i := range a.Samples {
		if a.Samples[i] != b.Samples[i] {
			t.Fatalf("signal not pure at %d", i)
		}
	}
}

// TestSpectrumFindsTheTones: with no chirp, the two tallest spectral peaks sit at
// the two tone frequencies (±1 bin, since the tones are offset half a bin). Confirms
// the DFT actually resolves the signal's content.
func TestSpectrumFindsTheTones(t *testing.T) {
	spec := spectrum(signal(40, 64, 0), rect())
	// find the two tallest local peaks
	type pk struct {
		bin int
		mag float64
	}
	var peaks []pk
	m := spec.Mag
	for i := 1; i < len(m)-1; i++ {
		if m[i] > m[i-1] && m[i] >= m[i+1] {
			peaks = append(peaks, pk{i, m[i]})
		}
	}
	// sort descending by magnitude (simple selection — few peaks)
	for i := range peaks {
		for j := i + 1; j < len(peaks); j++ {
			if peaks[j].mag > peaks[i].mag {
				peaks[i], peaks[j] = peaks[j], peaks[i]
			}
		}
	}
	if len(peaks) < 2 {
		t.Fatalf("expected at least two peaks, got %d", len(peaks))
	}
	near := func(bin, want int) bool { return bin >= want-1 && bin <= want+1 }
	b0, b1 := peaks[0].bin, peaks[1].bin
	ok := (near(b0, 40) && near(b1, 64)) || (near(b0, 64) && near(b1, 40))
	if !ok {
		t.Errorf("top two peaks at bins %d,%d — expected near 40 and 64 Hz", b0, b1)
	}
}

// TestHannReducesLeakage is the teaching claim, quantified: on the two-tone signal
// (no chirp), the Hann window concentrates MORE energy on the peaks than the
// rectangular window — leakage reduced. This is the whole point of the window switch;
// if it inverted, the notebook's text would be a lie.
func TestHannReducesLeakage(t *testing.T) {
	sig := signal(40, 64, 0)
	r := concentration(spectrum(sig, rect()).Mag)
	h := concentration(spectrum(sig, hann()).Mag)
	if h <= r {
		t.Errorf("Hann on-peak %.3f did not beat rectangular %.3f — leakage not reduced", h, r)
	}
	// and rectangular should show *some* leakage (< 100%) so there's an effect to see.
	if r >= 0.999 {
		t.Errorf("rectangular shows no leakage (%.3f) — the demo has nothing to demonstrate", r)
	}
}

// TestTransformsPure: spectrum and spectrogram are pure in (signal, window).
func TestTransformsPure(t *testing.T) {
	sig := signal(40, 64, 90)
	if a, b := spectrum(sig, rect()).Mag, spectrum(sig, rect()).Mag; !eq(a, b) {
		t.Error("spectrum not pure")
	}
	g1 := spectrogram(sig, rect())
	g2 := spectrogram(sig, rect())
	if len(g1.Cols) != len(g2.Cols) {
		t.Fatal("spectrogram column count not pure")
	}
	for c := range g1.Cols {
		if !eq(g1.Cols[c], g2.Cols[c]) {
			t.Fatalf("spectrogram column %d not pure", c)
		}
	}
}

// TestChirpFillsSpectrogramColumns: with a chirp, the peak frequency shifts across
// the spectrogram's time columns (the diagonal streak). With no chirp it stays put.
func TestChirpMovesAcrossTime(t *testing.T) {
	argmax := func(xs []float64) int {
		bi := 0
		for i := range xs {
			if xs[i] > xs[bi] {
				bi = i
			}
		}
		return bi
	}
	withChirp := spectrogram(signal(40, 64, 140), rect())
	first := argmax(withChirp.Cols[0])
	last := argmax(withChirp.Cols[len(withChirp.Cols)-1])
	if first == last {
		t.Errorf("chirp's dominant bin did not move over time (%d → %d) — no diagonal", first, last)
	}
}

// TestViewsRenderSVG: all three views render (waveform/spectrum as SVG, spectrogram
// as an SVG-wrapped PNG).
func TestViewsRenderSVG(t *testing.T) {
	sig := signal(40, 64, 90)
	if !strings.HasPrefix(waveform(sig).Render().Data, "<svg") {
		t.Error("waveform not SVG")
	}
	if !strings.HasPrefix(spectrumView(spectrum(sig, rect())).Render().Data, "<svg") {
		t.Error("spectrum not SVG")
	}
	sg := spectrogramView(spectrogram(sig, rect())).Render().Data
	if !strings.HasPrefix(sg, "<svg") || !strings.Contains(sg, "data:image/png;base64,") {
		t.Error("spectrogram not an SVG-wrapped PNG")
	}
}

func eq(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
