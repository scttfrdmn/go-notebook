// Command driver is the bidirectional bridge feed for the homefeed notebook. It
// is the wire between a smart-home bridge (Hue / Matter) and the notebook's one
// port, in BOTH directions:
//
//   - IN:  it reads sensors (motion, ambient light, temperature) and pushes each
//     reading to the notebook via POST /set.
//   - OUT: it subscribes to the notebook's /events stream, reads the computed
//     "APPLY light=… bright=… heat=…" command the plan cell emits, and
//     applies it to the devices.
//
// The notebook is the pure control logic; this program owns every impurity — the
// clock, the network, the hardware. Turning a light on is something the DRIVER
// does in response to a value the notebook COMPUTED, never something a cell does.
//
// The bridge here is SIMULATED (fake devices, no hardware, no keys) so the
// example runs anywhere. The one function `readSensors` and the one function
// `applyToBridge` are the entire seam: replace their bodies with a huego client
// (https://github.com/amimof/huego) or a Matter controller and the same notebook
// drives real bulbs, unchanged.
//
// Usage:
//
//	go tool notebook run ./examples/homefeed      # the controller (shell 1)
//	go run ./examples/homefeed/driver             # the bridge feed (shell 2)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

func main() {
	addr := flag.String("addr", "http://127.0.0.1:8080", "the served notebook's base URL")
	flag.Parse()
	log.Printf("homefeed bridge (simulated) → %s; Ctrl-C to stop", *addr)

	// OUT direction: subscribe to the notebook's decisions and apply them. Run in
	// the background so it streams while we push sensors on the main loop.
	go applyLoop(*addr)

	// IN direction: sample the (simulated) sensors once a second and set the leaves.
	tick := 0
	for range time.Tick(time.Second) {
		tick++
		s := readSensors(tick) // ← swap this body for a real bridge read
		set(*addr, "moving", boolToInt(s.motion))
		set(*addr, "ambient", s.lux)
		set(*addr, "tempC10", s.tempDeci)
		log.Printf("sensors: motion=%v lux=%d temp=%.1f°C", s.motion, s.lux, float64(s.tempDeci)/10)
	}
}

// sensors is one reading from the bridge.
type sensors struct {
	motion   bool
	lux      int
	tempDeci int
}

// readSensors is the IN seam. Simulated: motion pulses roughly every ~7s, the sun
// sets over the run (lux falls), temperature drifts. Replace the body with a
// huego / Matter read to drive from real devices.
func readSensors(tick int) sensors {
	return sensors{
		motion:   tick%7 < 2,                                // a couple of seconds of motion, then still
		lux:      int(500 + 480*math.Cos(float64(tick)/20)), // "daylight" rising and falling
		tempDeci: 205 + int(10*math.Sin(float64(tick)/15)),  // ~20.5°C drifting ±1
	}
}

// applyLoop is the OUT seam. It reads the notebook's /events SSE stream, pulls the
// plan cell's "APPLY …" command, and applies it to the bridge — only logging when
// the command actually changes, so the output reads like a device controller.
func applyLoop(base string) {
	resp, err := http.Get(base + "/events")
	if err != nil {
		log.Printf("subscribe: %v (is the notebook serving at %s?)", err, base)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	applyRe := regexp.MustCompile(`APPLY light=(\w+) bright=(\d+) heat=(\w+)`)
	last := ""
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		m := applyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cmd := m[0]
		if cmd == last {
			continue // no change — don't re-apply
		}
		last = cmd
		bright, _ := strconv.Atoi(m[2])
		applyToBridge(m[1] == "on", bright, m[3] == "on") // ← swap for a real bridge write
	}
}

// applyToBridge is where the computed plan meets the hardware. Simulated: it just
// logs. Replace with huego (light.On()/light.Bri()) or a Matter command.
func applyToBridge(lightOn bool, brightness int, heat bool) {
	log.Printf("APPLY → light=%v brightness=%d%% heat=%v", lightOn, brightness, heat)
}

// set posts one leaf edit — the data-in half of the one port.
func set(base, leaf string, value any) {
	body, _ := json.Marshal(map[string]any{"leaf": leaf, "value": value})
	resp, err := http.Post(base+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("set %s: %v", leaf, err)
		return
	}
	_ = resp.Body.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
