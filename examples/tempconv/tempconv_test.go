package tempconv

import (
	"strings"
	"testing"
)

// TestConversion pins the Celsius→Fahrenheit formula at the boundaries a reader
// will check: freezing, boiling, and the default.
func TestConversion(t *testing.T) {
	cases := []struct{ c, f int }{
		{0, 32}, {100, 212}, {20, 68}, {-40, -40},
	}
	for _, tc := range cases {
		if got := fahrenheit(tc.c); got != tc.f {
			t.Errorf("fahrenheit(%d) = %d, want %d", tc.c, got, tc.f)
		}
	}
}

// TestGaugeRenders confirms the thermometer draws SVG with both readouts — the
// view has a Render method, so it is never a blank cell.
func TestGaugeRenders(t *testing.T) {
	data := gauge(20, 68).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("gauge did not render SVG")
	}
	for _, want := range []string{"20°C", "68°F"} {
		if !strings.Contains(data, want) {
			t.Errorf("gauge SVG missing %q", want)
		}
	}
}

// TestReadingText pins the plain-text readout beside the gauge.
func TestReadingText(t *testing.T) {
	if got := string(reading(20, 68)); got != "20 °C  =  68 °F" {
		t.Errorf("reading = %q", got)
	}
}
