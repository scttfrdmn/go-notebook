// Command driver is the live feed for the sensorfeed notebook. It samples this
// process's own runtime metrics once a second and pushes each reading through the
// notebook's data-in port (POST /set), so the notebook's pure gauges move on
// their own — the impure edge (the clock, the sampling) lives HERE, in a separate
// program, never in a cell.
//
// This is the whole live-feed pattern: a feed is a driver on the set port. The
// notebook is a pure function of the leaves this program writes.
//
// Usage:
//
//	go tool notebook run ./examples/sensorfeed     # serve the dashboard (shell 1)
//	go run ./examples/sensorfeed/driver            # start the feed (shell 2)
//
// It reports the driver's OWN goroutine count and heap, plus a synthetic CPU
// number it drives with a small varying busy-loop — honest, keyless, and
// identical on every OS (no /proc, no ps, no cgo, no dependency). Point --addr at
// wherever the notebook is serving.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"runtime"
	"time"
)

func main() {
	addr := flag.String("addr", "http://127.0.0.1:8080", "the served notebook's base URL")
	every := flag.Duration("every", time.Second, "sample interval")
	flag.Parse()

	log.Printf("sensorfeed driver → %s (every %s); Ctrl-C to stop", *addr, *every)

	// The rolling window of recent CPU samples the notebook's history chart draws.
	// State lives HERE, in the driver, not in a (pure, stateless) cell.
	const windowLen = 60
	var window []int

	// A deterministic-ish phase so the synthetic CPU number visibly rises and
	// falls rather than sitting flat — a real feed would read the OS instead.
	tick := 0
	for range time.Tick(*every) {
		tick++
		cpu := syntheticCPU(tick)
		window = append(window, cpu)
		if len(window) > windowLen {
			window = window[len(window)-windowLen:]
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// One reading = several leaf writes through the one port. Each is exactly
		// what a slider drag posts; the notebook cannot tell a human from a feed.
		set(*addr, "cpuPct", cpu)
		set(*addr, "memMB", int(m.Sys>>20))
		set(*addr, "numG", runtime.NumGoroutine())
		set(*addr, "t", tick)
		set(*addr, "series", csv(window)) // the rolling window, as a string leaf

		log.Printf("sample %d: cpu=%d%% mem=%dMB goroutines=%d", tick, cpu, m.Sys>>20, runtime.NumGoroutine())
	}
}

// set posts one leaf edit to the notebook — the data-in half of the one port.
// {"leaf": <result name>, "value": <json>}, exactly what the browser client and
// a slider drag send.
func set(base, leaf string, value any) {
	body, _ := json.Marshal(map[string]any{"leaf": leaf, "value": value})
	resp, err := http.Post(base+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("set %s: %v (is the notebook serving at %s?)", leaf, err, base)
		return
	}
	_ = resp.Body.Close()
}

// syntheticCPU produces a smooth 0..100 wave so the demo visibly breathes. A real
// driver would sample the OS here (gopsutil, /proc/stat, or `ps`); this keeps the
// example keyless and identical on every platform.
func syntheticCPU(tick int) int {
	base := 45 + 40*math.Sin(float64(tick)/8.0)
	jitter := 5 * math.Sin(float64(tick)/1.3)
	v := int(base + jitter)
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v
}

// csv renders the window as "12,40,55" for the notebook's Series leaf.
func csv(vals []int) string {
	var b bytes.Buffer
	for i, v := range vals {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d", v)
	}
	return b.String()
}
