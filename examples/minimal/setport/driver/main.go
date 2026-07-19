// Command driver is the minimal live feed: it increments an integer once a
// second and POSTs it to a served setport notebook's data-in port. That is the
// whole feed pattern — a program with its hand on the knob. The impurity (the
// clock, the socket) lives here, in a separate process, never in a cell.
//
//	go tool notebook run ./examples/minimal/setport --no-open   # serve first
//	go run ./examples/minimal/setport/driver                    # then drive
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", "http://127.0.0.1:8080", "the served notebook's base URL")
	flag.Parse()

	log.Printf("driving %s/set with n = 0,1,2,… once a second (Ctrl-C to stop)", *addr)
	for v := 0; ; v++ {
		set(*addr, "n", v%101) // 0..100, matching the leaf's slider bounds
		time.Sleep(time.Second)
	}
}

// set posts one leaf edit — the data-in half of the one port. The body is
// {"leaf": <result name>, "value": <json>}, exactly what a slider drag sends.
func set(base, leaf string, value any) {
	body, _ := json.Marshal(map[string]any{"leaf": leaf, "value": value})
	resp, err := http.Post(base+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("set %s: %v (is the notebook serving at %s?)", leaf, err, base)
		return
	}
	_ = resp.Body.Close()
}
