package homefeed

import "testing"

// TestLightPolicy pins the control logic: the light is on only with motion AND
// dark, and brightness scales with darkness. This is the notebook's whole
// decision — if it drifts, the house does the wrong thing.
func TestLightPolicy(t *testing.T) {
	dark := 200
	cases := []struct {
		motion, ambient int
		want            bool
	}{
		{1, 80, true},   // motion + dark → on
		{0, 80, false},  // dark but nobody there → off
		{1, 400, false}, // motion but bright enough → off
		{0, 400, false}, // neither → off
	}
	for _, c := range cases {
		if got := lightOn(c.motion, c.ambient, dark); got != c.want {
			t.Errorf("lightOn(motion=%d, ambient=%d, dark=%d) = %v, want %v", c.motion, c.ambient, dark, got, c.want)
		}
	}
}

// TestBrightnessScalesWithDark confirms the lamp is dimmer nearer the threshold
// and brighter in the dark, and zero when off.
func TestBrightnessScalesWithDark(t *testing.T) {
	dark := 200
	off := brightness(false, 10, dark)
	if off != 0 {
		t.Errorf("brightness when off = %d, want 0", off)
	}
	veryDark := brightness(true, 0, dark)
	nearThreshold := brightness(true, 190, dark)
	if !(veryDark > nearThreshold) {
		t.Errorf("brightness should fall toward the threshold: dark=%d near=%d", veryDark, nearThreshold)
	}
	if veryDark < 20 || veryDark > 100 {
		t.Errorf("brightness out of the 20..100 band: %d", veryDark)
	}
}

// TestHeatCallsBelowSetpoint pins the thermostat logic.
func TestHeatCallsBelowSetpoint(t *testing.T) {
	if !heat(200, 210) {
		t.Error("20.0°C below a 21.0°C setpoint should call for heat")
	}
	if heat(215, 210) {
		t.Error("21.5°C above a 21.0°C setpoint should NOT call for heat")
	}
}

// TestPlanCommandRoundTrips confirms the plan renders the machine-readable APPLY
// line the driver parses back off /events — the OUT half of the loop. If the
// format drifts, the driver silently stops applying (running is not passing).
func TestPlanCommandRoundTrips(t *testing.T) {
	html := plan(true, 68, false).Render()
	for _, want := range []string{"APPLY", "light=on", "bright=68", "heat=off"} {
		if !contains(html.Data, want) {
			t.Errorf("plan command missing %q in:\n%s", want, html.Data)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
