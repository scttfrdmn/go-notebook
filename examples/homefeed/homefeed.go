//go:notebook
//
// A smart-home controller — sensors in, lights out, over the one port.
//
// This is the live-feed pattern in BOTH directions. A driver talking to a
// Matter/Hue bridge pushes sensor readings IN (motion, temperature, ambient
// light) through the notebook's `set` port; the notebook computes, with pure
// cells, what the lights SHOULD do — and the driver reads those decisions OUT of
// the `/events` stream and applies them to the bulbs. The notebook is the control
// logic; the bridge is the world; the driver is the wire between them.
//
// The notebook stays pure and portable: it fetches nothing, controls no hardware,
// and has no timer. "Turn the light on" is not a side effect a cell performs — it
// is a boolean a cell COMPUTES, which the driver observes and acts on. That is the
// F3 rule (the transport owns the impure boundary; a cell is subscribed, never
// pushes) applied to actuators, not just sensors: the light is an OUTPUT you
// subscribe to, exactly symmetric with the sensor you set.
//
// Run it:
//
//	go tool notebook run ./examples/homefeed          # the controller UI
//	go run ./examples/homefeed/driver                 # the bridge feed (separate shell)
//
// The driver ships a SIMULATED bridge (fake devices; motion comes and goes, the
// sun sets) so it runs with no hardware and no keys. A comment marks exactly where
// a real huego / Matter SDK client would slot in.
//
//notebook:layout intro
//notebook:layout sensors | policy
//notebook:layout plan

package homefeed

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Sensor leaves — driven IN by the bridge feed. Sliders too, so the notebook is
// usable by hand with no driver (be the house yourself).
// ---------------------------------------------------------------------------

// Motion detected in the room, right now (0 = still, 1 = motion).
//
//notebook:slider min=0 max=1 step=1 area=sensors
func motion() (moving int) { return 0 }

// Ambient light, lux (0 = dark, ~500 = bright office, ~1000 = daylight).
//
//notebook:slider min=0 max=1000 step=10 area=sensors
func lux() (ambient int) { return 400 }

// Room temperature, °C ×10 (so the slider is integer-clean; 215 = 21.5°C).
//
//notebook:slider min=100 max=320 step=1 area=sensors
func tempDeci() (tempC10 int) { return 215 }

// ---------------------------------------------------------------------------
// Policy inputs — the human's preferences (real sliders a person sets).
// ---------------------------------------------------------------------------

// Turn lights on below this ambient level (lux). Above it, the room is bright
// enough on its own.
//
//notebook:slider min=0 max=1000 step=10 area=policy
func darkThreshold() (darkLux int) { return 200 }

// Comfort setpoint, °C ×10 — below this the notebook calls for heat.
//
//notebook:slider min=150 max=280 step=1 area=policy
func setpointDeci() (setC10 int) { return 210 }

// ---------------------------------------------------------------------------
// Actuator cells — pure functions of sensors + policy. The DRIVER reads these
// off /events and applies them to the bulbs/thermostat. A cell computes the
// decision; it never performs the action.
// ---------------------------------------------------------------------------

// Should the light be on? On when there is motion AND the room is dark. A pure
// boolean — the driver subscribes to it and switches the bulb.
func lightOn(moving, ambient, darkLux int) (on bool) {
	return moving == 1 && ambient < darkLux
}

// Target brightness, 0..100 — dimmer the darker it is (only meaningful when on).
func brightness(on bool, ambient, darkLux int) (level int) {
	if !on {
		return 0
	}
	// Full brightness in the dark, easing off as ambient approaches the threshold.
	frac := float64(darkLux-ambient) / float64(darkLux)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return 20 + int(frac*80) // 20..100
}

// Call for heat? When the room is below the comfort setpoint.
func heat(tempC10, setC10 int) (heating bool) { return tempC10 < setC10 }

// The plan the driver applies: the computed device states, as a compact command
// the driver parses back and pushes to the bridge. Pure — a projection of the
// actuator cells above.
//
//notebook:height=140 area=plan
func plan(on bool, level int, heating bool) (command Command) {
	return Command{LightOn: on, Brightness: level, Heat: heating}
}

// A human-readable panel of what the house is doing and why.
//
//notebook:height=200 area=policy
func board(moving, ambient, tempC10, darkLux int, on bool, level int, heating bool) (report Readout) {
	reason := "off"
	switch {
	case on:
		reason = "motion + dark"
	case moving == 1:
		reason = "motion, but bright enough"
	}
	return Readout{Cards: []Card{
		{Label: "light", Value: onoff(on) + tail(on, " · "+itoa(level)+"%"), Tone: toneOn(on)},
		{Label: "why", Value: reason},
		{Label: "heat", Value: onoff(heating), Tone: toneOn(heating)},
		{Label: "room", Value: deci(tempC10) + "°C · " + itoa(ambient) + " lux"},
	}}
}

// Orientation.
func intro() (md Markdown) {
	return `## Smart-home controller

Sensors (motion, ambient light, temperature) are pushed IN by a bridge feed; the
notebook decides, with **pure** cells, what the lights and heat should do; the
driver reads those decisions OUT and applies them to the devices. Turning a light
on isn't something a cell *does* — it's a boolean a cell *computes* and the driver
*subscribes to*. Set a sensor slider yourself, or run the driver
(` + "`go run ./examples/homefeed/driver`" + `, a simulated Hue/Matter bridge) and watch
the room run itself.`
}

// ===========================================================================
// Helpers + types.
// ===========================================================================

func itoa(n int) string { return strconv.Itoa(n) }
func onoff(b bool) string {
	if b {
		return "ON"
	}
	return "off"
}
func tail(b bool, s string) string {
	if b {
		return s
	}
	return ""
}
func toneOn(b bool) int {
	if b {
		return good
	}
	return muted
}
func deci(d int) string { return strconv.FormatFloat(float64(d)/10, 'f', 1, 64) }

const (
	muted = iota
	good
)

// Command is the computed device plan the driver applies. Rendered as a compact,
// machine-readable line the driver parses back off /events, plus a human label.
type Command struct {
	LightOn    bool
	Brightness int
	Heat       bool
}

func (c Command) Render() Rendered {
	// A machine line the driver greps out of the stream, then a human caption.
	// Format: "APPLY light=on bright=80 heat=off" — stable and trivial to parse.
	light := "off"
	if c.LightOn {
		light = "on"
	}
	heat := "off"
	if c.Heat {
		heat = "on"
	}
	line := fmt.Sprintf("APPLY light=%s bright=%d heat=%s", light, c.Brightness, heat)
	var b strings.Builder
	b.WriteString(`<div style="font:13px/1.5 -apple-system,system-ui,sans-serif">`)
	fmt.Fprintf(&b, `<code style="background:#0f1524;color:#e6ebf5;padding:.4rem .6rem;border-radius:6px;display:inline-block">%s</code>`, line)
	b.WriteString(`<div style="color:#5b6472;margin-top:.4rem">the driver reads this off the event stream and pushes it to the bridge</div>`)
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Card struct {
	Label, Value string
	Tone         int
}
type Readout struct{ Cards []Card }

func (r Readout) Render() Rendered {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:12px;flex-wrap:wrap">`)
	for _, c := range r.Cards {
		color := "#1b3a6b"
		switch c.Tone {
		case good:
			color = "#0ca30c"
		case muted:
			color = "#5b6472"
		}
		b.WriteString(`<div style="flex:1;min-width:120px;border:1px solid #e7ebf0;border-radius:8px;padding:10px 12px">`)
		fmt.Fprintf(&b, `<div style="font-size:12px;color:#5b6472">%s</div>`, c.Label)
		fmt.Fprintf(&b, `<div style="font:700 20px/1.2 -apple-system,system-ui,sans-serif;color:%s">%s</div>`, color, c.Value)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return Rendered{MIME: "text/html", Data: b.String()}
}

type Rendered struct{ MIME, Data string }
type Markdown string

func (m Markdown) Render() Rendered { return Rendered{MIME: "text/markdown", Data: string(m)} }
