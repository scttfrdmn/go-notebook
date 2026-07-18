//go:notebook
//
// A signal, three ways — and why the window matters.
//
// Build a signal from a few sine components plus a rising chirp, and look at it the
// three ways signal processing always does, all from the one source:
//
//   - the **waveform** — amplitude over time;
//   - the **spectrum** — the magnitude of its Fourier transform, the tones it's made of;
//   - the **spectrogram** — the spectrum computed in a sliding window, so you can see
//     frequency *change over time* (the chirp climbs; a pure tone is a flat band).
//
// The three are cells that all read the one `signal`. Change a slider and all three
// redraw from the same recomputed samples — the dependency graph forks
// signal → {waveform, spectrum, spectrogram}, the reactive thesis as three linked views.
//
// The teaching payload is the **window**. A DFT assumes its input repeats forever; a
// finite chunk almost never joins up end to end, and that discontinuity smears each
// pure tone into a skirt of false neighbours — *spectral leakage*. Switch the window
// from **rectangular** (just chop the chunk) to **Hann** (taper the ends to zero) and
// watch the skirts collapse: the same tone, far sharper, because the taper removes the
// discontinuity the transform was reacting to. Leakage is not noise in the signal —
// it is an artifact of the window, and you can toggle it.
//
// Pure Go throughout: the DFT is hand-rolled over complex128, so it compiles to wasm.
// The spectrum and spectrogram are pure functions of (signal, window), so a slider
// scrubs exactly. The three panels share `//notebook:area=panels`.

package spectrogram

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strconv"
	"strings"
)

const (
	n        = 512 // samples in the signal
	sampleHz = 512 // sample rate, so frequency bins read as Hz directly
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// Frequency of the first pure tone, in Hz. Its spectral peak sits here; its leakage
// skirt (with a rectangular window) spreads either side.
//
//notebook:slider min=8 max=120 step=1
func toneA() (fa int) { return 40 }

// Frequency of the second pure tone, in Hz. Put it near the first and the leakage
// skirts overlap — the reason leakage matters: it can bury a weak neighbour.
//
//notebook:slider min=8 max=120 step=1
func toneB() (fb int) { return 64 }

// Chirp depth — how far a rising sweep climbs across the record, in Hz. Starts at
// zero so the notebook opens on the clean two-tone leakage story; turn it up and the
// spectrogram shows the sweep as a diagonal streak. (A chirp's energy legitimately
// spans many bins, so it muddies the spectrum's leakage readout — hence off by
// default: one idea at a time.)
//
//notebook:slider min=0 max=160 step=5
func chirpDepth() (depth int) { return 0 }

// The analysis window. Rectangular leaks; Hann tapers the ends to zero and tightens
// every peak. Switch it and watch the skirts collapse.
func windowChoice() (win Select[Window]) {
	return Select[Window]{All: []Window{Rectangular, Hann}, Value: Rectangular}
}

// ---------------------------------------------------------------------------
// Compute (Go) — the signal and its transforms, pure.
// ---------------------------------------------------------------------------

// The signal: two pure tones plus a linear chirp, sampled to n points. A pure
// function of the sliders — the one source the three views share.
func signal(fa int, fb int, depth int) (sig Signal) {
	s := make([]float64, n)
	// Offset the tones by half a bin (+0.5 Hz). A tone exactly on a DFT bin has no
	// leakage regardless of window — the demo would show nothing. Between bins is the
	// honest, common case: the tone's period doesn't fit the record a whole number of
	// times, so a rectangular window smears it and a Hann window concentrates it.
	// That half-bin offset is what makes the window switch visible.
	freqA := float64(fa) + 0.5
	freqB := float64(fb) + 0.5
	for i := range s {
		t := float64(i) / float64(sampleHz) // seconds
		// two tones
		s[i] = math.Sin(2*math.Pi*freqA*t) + 0.8*math.Sin(2*math.Pi*freqB*t)
		// a linear chirp, only when depth > 0 (depth 0 = no chirp, not a 10 Hz tone):
		// instantaneous frequency rises from 10 Hz by `depth` Hz across the record.
		if depth > 0 {
			f0, f1 := 10.0, 10.0+float64(depth)
			inst := f0 + (f1-f0)*t/(float64(n)/float64(sampleHz))
			s[i] += 0.7 * math.Sin(2*math.Pi*inst*t)
		}
	}
	return Signal{Samples: s}
}

// The spectrum: magnitude of the windowed DFT of the whole record. Pure in (signal,
// window). This is where leakage lives — a rectangular window leaves each tone with
// a wide skirt; a Hann window tightens it.
func spectrum(sig Signal, win Select[Window]) (spec Spectrum) {
	w := applyWindow(sig.Samples, win.Value)
	mags := dftMag(w)
	// keep the lower half (real signal → symmetric spectrum); bins read as Hz.
	half := len(mags) / 2
	return Spectrum{Mag: mags[:half]}
}

// The spectrogram: the windowed DFT over a sliding window, stacked into a
// time×frequency grid. Pure in (signal, window). The chirp shows as a diagonal; a
// steady tone as a horizontal band.
func spectrogram(sig Signal, win Select[Window]) (grid Spectrogram) {
	const (
		frame = 128 // window length
		hop   = 16  // step between windows
	)
	half := frame / 2
	var cols [][]float64
	for start := 0; start+frame <= len(sig.Samples); start += hop {
		w := applyWindow(sig.Samples[start:start+frame], win.Value)
		m := dftMag(w)
		cols = append(cols, m[:half])
	}
	return Spectrogram{Cols: cols, Bins: half}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The waveform — amplitude over time. The raw signal all three views come from.
//
//notebook:area=panels
//notebook:height=300
func waveform(sig Signal) (wave WaveChart) {
	return WaveChart{Samples: sig.Samples}
}

// The spectrum — the tones the signal is made of. Watch each peak's skirt collapse
// when you switch to the Hann window: that skirt was leakage, not signal.
//
//notebook:area=panels
//notebook:height=300
func spectrumView(spec Spectrum) (chart SpectrumChart) {
	return SpectrumChart{Mag: spec.Mag}
}

// The spectrogram — frequency over time. The chirp climbs as a diagonal; the tones
// are horizontal bands. Time runs left to right, low frequency at the bottom.
// Returns the Spectrogram directly (it has a Render method the engine calls) —
// calling Render in this cell body would pull fmt into the cell's call graph and
// trip the fmt→os WASM gate, the same lesson as the other image notebooks.
//
//notebook:height=300
func spectrogramView(grid Spectrogram) (view SpectrogramView) {
	return SpectrogramView{Grid: grid}
}

// The numbers: which window is active, and how concentrated the spectrum is — the
// fraction of energy sitting within 4 bins of a local peak, vs leaked into the gaps
// between. Hann raises this: less leakage, more energy where the tones actually are.
func readout(spec Spectrum, win Select[Window]) (report Readout) {
	return Readout{Cards: []Card{
		{Label: "window", Value: win.Value.Label()},
		{Label: "energy on-peak", Value: pct(concentration(spec.Mag)), Caption: "higher = less leakage (try Hann)"},
	}}
}

// A signal, three ways — and why the window matters.
func intro() (md Markdown) {
	return `Build a signal from two tones and a rising chirp, and see it three ways —
**waveform**, **spectrum**, **spectrogram** — all from the one ` + "`signal`" + ` cell.
Change any slider and all three redraw together: the graph forks
signal → {waveform, spectrum, spectrogram}.

Then switch the **window** from rectangular to Hann and watch the spectral peaks
sharpen. A DFT assumes its input repeats forever; a finite chunk doesn't join up, and
that discontinuity smears each tone into a skirt of false frequencies — **spectral
leakage**. The Hann window tapers the ends to zero, removing the discontinuity, and
the skirts collapse. Leakage is an artifact of the window, not the signal — so you
can toggle it.`
}

// ===========================================================================
// DSP
// ===========================================================================

// applyWindow multiplies the samples by the chosen window function.
func applyWindow(s []float64, w Window) []float64 {
	out := make([]float64, len(s))
	m := len(s)
	for i, v := range s {
		switch w {
		case Hann:
			out[i] = v * (0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(m-1)))
		default: // Rectangular
			out[i] = v
		}
	}
	return out
}

// dftMag returns the magnitude spectrum of a real signal via a direct DFT.
func dftMag(s []float64) []float64 {
	m := len(s)
	mags := make([]float64, m)
	for k := 0; k < m; k++ {
		var re, im float64
		for t, v := range s {
			ang := -2 * math.Pi * float64(k) * float64(t) / float64(m)
			re += v * math.Cos(ang)
			im += v * math.Sin(ang)
		}
		mags[k] = math.Hypot(re, im) / float64(m)
	}
	return mags
}

// concentration returns the fraction of spectral energy within ±4 bins of a local
// peak — high when tones are sharp, low when leakage has smeared energy into the
// gaps between them. This is the quantity a Hann window improves and a rectangular
// window (on a between-bins tone) does not: leakage measured, not asserted.
func concentration(mag []float64) float64 {
	if len(mag) == 0 {
		return 0
	}
	// local peaks: a bin taller than both neighbours and above a noise floor.
	peak := maxOf(mag)
	if peak == 0 {
		return 0
	}
	onPeak := make([]bool, len(mag))
	for i := 1; i < len(mag)-1; i++ {
		if mag[i] > mag[i-1] && mag[i] >= mag[i+1] && mag[i] > 0.08*peak {
			for d := -4; d <= 4; d++ {
				j := i + d
				if j >= 0 && j < len(mag) {
					onPeak[j] = true
				}
			}
		}
	}
	var near, total float64
	for i, v := range mag {
		e := v * v
		total += e
		if onPeak[i] {
			near += e
		}
	}
	if total == 0 {
		return 0
	}
	return near / total
}

// ===========================================================================
// Helpers
// ===========================================================================

func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, v := range xs {
		if v > m {
			m = v
		}
	}
	return m
}

// ===========================================================================
// Types
// ===========================================================================

// Window is the analysis window function — the Select's option type.
type Window int

const (
	Rectangular Window = iota
	Hann
)

func (w Window) Label() string {
	if w == Hann {
		return "Hann"
	}
	return "rectangular"
}

type Signal struct{ Samples []float64 }
type Spectrum struct{ Mag []float64 }
type Spectrogram struct {
	Cols [][]float64
	Bins int
}

// Select[T] — the option widget (lego's shape). Its value is one of All; the client
// shows a dropdown of Label()s. First original to use it, to switch the window.
type Select[T interface{ Label() string }] struct {
	All   []T
	Value T
}

func (s Select[T]) Options() []string {
	out := make([]string, len(s.All))
	for i, o := range s.All {
		out[i] = o.Label()
	}
	return out
}

// Reconcile falls back to the default if the saved label is gone (it never is here —
// the two windows are fixed — but the widget contract wants it).
func (s Select[T]) Reconcile(saved any) any {
	label, ok := saved.(string)
	if !ok {
		return s
	}
	for _, o := range s.All {
		if o.Label() == label {
			s.Value = o
			return s
		}
	}
	return s
}

func (s Select[T]) WidgetView() WidgetView {
	return WidgetView{Value: s.Value.Label(), Options: s.Options()}
}

type WidgetView struct {
	Value   any
	Options []string
	Lo, Hi  *float64
	Max     *int
}

// WaveChart draws the waveform.
type WaveChart struct{ Samples []float64 }

func (c WaveChart) Render() Rendered {
	const w, h, pad = 440.0, 300.0, 20.0
	amp := maxOf(absAll(c.Samples))
	if amp == 0 {
		amp = 1
	}
	sx := func(i int) float64 { return pad + float64(i)/float64(len(c.Samples)-1)*(w-2*pad) }
	sy := func(v float64) float64 { return h/2 - v/amp*(h/2-pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#e7ebf0"/>`, pad, h/2, w-pad, h/2)
	var d strings.Builder
	for i, v := range c.Samples {
		verb := " L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&d, "%s%.1f %.1f", verb, sx(i), sy(v))
	}
	fmt.Fprintf(&b, `<path d=%q fill="none" stroke="#2a78d6" stroke-width="1"/>`, d.String())
	fmt.Fprintf(&b, `<text x="%.0f" y="16" font-family="sans-serif" font-size="12" fill="#1b3a6b">waveform</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// SpectrumChart draws the magnitude spectrum as bars.
type SpectrumChart struct{ Mag []float64 }

func (c SpectrumChart) Render() Rendered {
	const w, h, pad = 440.0, 300.0, 24.0
	ymax := maxOf(c.Mag)
	if ymax == 0 {
		ymax = 1
	}
	nb := len(c.Mag)
	bw := (w - 2*pad) / float64(nb)
	sy := func(v float64) float64 { return h - pad - v/ymax*(h-2*pad) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)
	for i, v := range c.Mag {
		x := pad + float64(i)*bw
		top := sy(v)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#2a78d6"/>`,
			x, top, math.Max(bw-0.5, 0.5), h-pad-top)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="16" font-family="sans-serif" font-size="12" fill="#1b3a6b">spectrum (magnitude vs Hz)</text>`, pad)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// SpectrogramView wraps a Spectrogram so the VIEW cell is the renderable one (the
// compute cell returns a bare Spectrogram, which is not rendered directly).
type SpectrogramView struct{ Grid Spectrogram }

// Render draws the spectrogram as a PNG heatmap embedded in an SVG: time on x, low
// frequency at the bottom, magnitude as brightness.
func (v SpectrogramView) Render() Rendered {
	g := v.Grid
	cols := len(g.Cols)
	if cols == 0 || g.Bins == 0 {
		return Rendered{MIME: "image/svg+xml", Data: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"></svg>`}
	}
	// normalize by the global max (log-scaled for contrast)
	gmax := 0.0
	for _, c := range g.Cols {
		gmax = math.Max(gmax, maxOf(c))
	}
	if gmax == 0 {
		gmax = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, cols, g.Bins))
	for x, col := range g.Cols {
		for f := 0; f < g.Bins; f++ {
			v := math.Log1p(col[f]/gmax*20) / math.Log1p(20) // 0..1, log-scaled
			r, gg, bl := magColor(v)
			y := g.Bins - 1 - f // low freq at the bottom
			o := (y*cols + x) * 4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, gg, bl, 255
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	const box = 900.0
	const bh = 300.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, box, bh)
	fmt.Fprintf(&b, `<image x="0" y="0" width="%.0f" height="%.0f" href=%q preserveAspectRatio="none" style="image-rendering:pixelated"/>`,
		box, bh, uri)
	fmt.Fprintf(&b, `<text x="8" y="18" font-family="sans-serif" font-size="12" fill="#fff">spectrogram — frequency (up) over time (right)</text>`)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

// magColor maps a normalized magnitude to a dark-blue→cyan→yellow ramp (a legible
// heatmap on a dark ground).
func magColor(v float64) (uint8, uint8, uint8) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	switch {
	case v < 0.5:
		t := v / 0.5
		return lerp(15, 20, t), lerp(21, 150, t), lerp(40, 170, t) // navy → teal
	default:
		t := (v - 0.5) / 0.5
		return lerp(20, 250, t), lerp(150, 230, t), lerp(170, 90, t) // teal → yellow
	}
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

func absAll(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, v := range xs {
		out[i] = math.Abs(v)
	}
	return out
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
