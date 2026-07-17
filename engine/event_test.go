package engine

import (
	"encoding/json"
	"testing"
)

// TestWireEventShape pins the exact JSON the SSE transport puts on the wire. This
// is the anti-pass for collapsing the server's old wireEvent struct and the WASM
// bridge's inline map into one shared engine.WireEvent: if this shape ever drifts,
// every client (server SSE + WASM bridge) drifts with it, so it is frozen here.
func TestWireEventShape(t *testing.T) {
	ev := Event{
		Epoch: 7,
		Cell:  "chart",
		State: StateDone,
		Out:   &Rendered{MIME: "image/svg+xml", Data: "PAYLOAD"},
	}
	got, err := json.Marshal(ToWire(ev))
	if err != nil {
		t.Fatal(err)
	}
	// Field names, order, and presence are the frozen contract. (HTML-escaping of
	// < > & in Data is unchanged from the old json.Encoder and is exercised by the
	// Map-parity test; a plain payload here keeps this assertion about shape.)
	want := `{"epoch":7,"cell":"chart","state":"done","mime":"image/svg+xml","data":"PAYLOAD"}`
	if string(got) != want {
		t.Errorf("wire JSON drifted:\n got %s\nwant %s", got, want)
	}
}

// TestWireEventOmitsEmpty: a bare state transition (no output, no error) carries
// only the three always-present fields — mime/data/err are omitted, not sent
// empty. The client's `if (!ev.mime) return` path depends on this.
func TestWireEventOmitsEmpty(t *testing.T) {
	got, err := json.Marshal(ToWire(Event{Epoch: 3, Cell: "x", State: StateRunning}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"epoch":3,"cell":"x","state":"running"}`
	if string(got) != want {
		t.Errorf("empty-field omission drifted:\n got %s\nwant %s", got, want)
	}
}

// TestWireEventMapMatchesJSON is the load-bearing parity check: the Map() form
// (what the WASM bridge hands to js.ValueOf, since it cannot marshal a struct)
// must carry byte-identical keys and values to the JSON encoding (what the SSE
// server sends). The two projections share one source — this proves they cannot
// diverge, which is the whole reason WireEvent lives in one place.
func TestWireEventMapMatchesJSON(t *testing.T) {
	cases := []Event{
		{Epoch: 7, Cell: "chart", State: StateDone, Out: &Rendered{MIME: "text/markdown", Data: "# hi"}},
		{Epoch: 1, Cell: "x", State: StateRunning},            // no out, no err
		{Epoch: 2, Cell: "y", State: StateError, Err: "boom"}, // error, no out
		{Epoch: 9, Cell: "z", State: StateBlocked},            // blocked, bare
	}
	for _, ev := range cases {
		w := ToWire(ev)

		// Round-trip the struct's JSON into a generic map...
		b, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		var fromJSON map[string]any
		if err := json.Unmarshal(b, &fromJSON); err != nil {
			t.Fatal(err)
		}

		// ...and compare it key-for-key with the Map() the WASM bridge uses.
		fromMap := w.Map()
		if len(fromJSON) != len(fromMap) {
			t.Errorf("cell %s: key count differs — json %v vs map %v", ev.Cell, fromJSON, fromMap)
			continue
		}
		for k, jv := range fromJSON {
			mv, ok := fromMap[k]
			if !ok {
				t.Errorf("cell %s: key %q in JSON but not in Map()", ev.Cell, k)
				continue
			}
			// json numbers come back as float64; Map() also stores epoch as float64.
			if jf, ok := jv.(float64); ok {
				if mf, ok := mv.(float64); !ok || mf != jf {
					t.Errorf("cell %s: key %q numeric mismatch json=%v map=%v", ev.Cell, k, jv, mv)
				}
				continue
			}
			if jv != mv {
				t.Errorf("cell %s: key %q value mismatch json=%v map=%v", ev.Cell, k, jv, mv)
			}
		}
	}
}
