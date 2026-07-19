// Command driver is the price feed for the tickerfeed notebook. It streams trade
// ticks and pushes each one to the notebook's set port; the notebook does the
// (pure) analysis. It maintains the rolling window here, because a cell is pure
// and holds no state.
//
// The stream is SIMULATED (a random walk) so the example runs offline with no API
// key. The `nextTick` function is the entire seam: replace its body with a read
// off a real exchange WebSocket (e.g. gorilla/websocket to Coinbase's
// wss://ws-feed.exchange.coinbase.com "matches" channel) and the same notebook
// analyzes real trades, unchanged.
//
// Usage:
//
//	go tool notebook run ./examples/tickerfeed     # the ticker UI (shell 1)
//	go run ./examples/tickerfeed/driver            # the price feed (shell 2)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "http://127.0.0.1:8080", "the served notebook's base URL")
	flag.Parse()
	log.Printf("tickerfeed driver (simulated stream) → %s; Ctrl-C to stop", *addr)

	const windowLen = 80
	var window []int
	priceCents := 6500000 // $65,000.00 start

	// A deterministic LCG for the random walk — no clock, no math/rand global, so
	// the demo is reproducible run to run.
	var rng uint64 = 0x9e3779b97f4a7c15
	step := func() int {
		rng = rng*6364136223846793005 + 1442695040888963407
		return int(rng>>40)%2001 - 1000 // ±$10.00 in cents
	}

	for range time.Tick(400 * time.Millisecond) {
		priceCents = nextTick(priceCents, step) // ← swap for a real WS read
		if priceCents < 1 {
			priceCents = 1
		}
		window = append(window, priceCents)
		if len(window) > windowLen {
			window = window[len(window)-windowLen:]
		}
		set(*addr, "cents", priceCents)
		set(*addr, "series", csv(window))
	}
}

// nextTick returns the next trade price. Simulated: a bounded random walk around
// the last price. Replace with a WebSocket read to stream real trades.
func nextTick(last int, step func() int) int { return last + step() }

func set(base, leaf string, value any) {
	body, _ := json.Marshal(map[string]any{"leaf": leaf, "value": value})
	resp, err := http.Post(base+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("set %s: %v", leaf, err)
		return
	}
	_ = resp.Body.Close()
}

func csv(vals []int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
