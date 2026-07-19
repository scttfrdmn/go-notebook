// Command driver is the poller for the apifeed notebook. It fetches the ISS's
// live position from a public JSON API every few seconds and pushes it to the
// notebook's set port; the notebook plots it, purely. The HTTP call and the
// interval clock live here, never in a cell.
//
// The endpoint (wheretheiss.at) is keyless and public. If it's unreachable, the
// driver falls back to a simulated orbit so the example still runs offline. The
// `fetchPosition` function is the API seam — point it at any REST feed.
//
// Usage:
//
//	go tool notebook run ./examples/apifeed        # the tracker UI (shell 1)
//	go run ./examples/apifeed/driver               # the poller (shell 2)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"math"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", "http://127.0.0.1:8080", "the served notebook's base URL")
	every := flag.Duration("every", 3*time.Second, "poll interval")
	flag.Parse()
	log.Printf("apifeed ISS poller → %s (every %s); Ctrl-C to stop", *addr, *every)

	n := 0
	for range time.Tick(*every) {
		n++
		lat, lon, live := fetchPosition(n) // ← the API seam
		set(*addr, "latMilli", int(lat*1000))
		set(*addr, "lonMilli", int(lon*1000))
		set(*addr, "n", n)
		src := "live"
		if !live {
			src = "simulated (API unreachable)"
		}
		log.Printf("poll %d [%s]: lat=%.3f lon=%.3f", n, src, lat, lon)
	}
}

// fetchPosition returns the ISS latitude/longitude. It tries the public API; on
// any error it falls back to a simulated great-circle orbit so the demo runs
// offline. Replace the URL/parse to poll a different REST feed.
func fetchPosition(n int) (lat, lon float64, live bool) {
	client := http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("https://api.wheretheiss.at/v1/satellites/25544")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if json.NewDecoder(resp.Body).Decode(&body) == nil {
			return body.Latitude, body.Longitude, true
		}
	}
	// Offline fallback: a simulated orbit — inclination ~51.6°, circling fast.
	t := float64(n)
	lon = math.Mod(t*22, 360) - 180 // sweeps west→east, wraps
	lat = 51.6 * math.Sin(t/3.0)    // oscillates within the ISS inclination band
	return lat, lon, false
}

func set(base, leaf string, value any) {
	body, _ := json.Marshal(map[string]any{"leaf": leaf, "value": value})
	resp, err := http.Post(base+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("set %s: %v", leaf, err)
		return
	}
	_ = resp.Body.Close()
}
