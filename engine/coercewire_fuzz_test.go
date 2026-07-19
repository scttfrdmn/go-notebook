package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzCoerceWire drives the wire-coercion path with arbitrary JSON — the same
// shape a POST /set or a wasm notebookSet delivers. CoerceWire is the one place
// untrusted client input crosses into the engine, so its only obligations are:
// never panic, and never claim success (ok==true) while returning a value that
// still carries a wire shape (json.Number, []any, map with un-homogenized
// values). A malformed selection must fail loud (ok==false), never silently.
func FuzzCoerceWire(f *testing.F) {
	for _, seed := range []string{
		`0.7`, `true`, `"hpc7i"`, `null`,
		`[1,2,3]`, `["City","Duplo"]`, `[]`,
		`{"Source":"t.csv","Rows":7}`,
		`[1,"mixed",true]`, `{"a":{"b":[1,2]}}`,
		`1e400`, `[[1,2],[3,4]]`, `"  "`, `{}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		var decoded any
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber() // matches the /set path: numbers arrive as json.Number
		if err := dec.Decode(&decoded); err != nil {
			return // not valid JSON — not an input CoerceWire is asked to handle
		}

		got, ok := CoerceWire(decoded) // must not panic on any decoded JSON value
		if !ok {
			return
		}
		// On success, the result must be fully homogenized — no wire encoding
		// left. Assert recursively that no json.Number survives and slices/maps
		// hold only clean primitives.
		assertHomogenized(t, got)
	})
}

// assertHomogenized fails if v still contains a wire-shaped value (json.Number)
// that CoerceWire claimed to have cleaned.
func assertHomogenized(t *testing.T, v any) {
	t.Helper()
	switch x := v.(type) {
	case json.Number:
		t.Fatalf("CoerceWire returned ok=true but left a json.Number %v", x)
	case []any:
		for _, e := range x {
			assertHomogenized(t, e)
		}
	case map[string]any:
		for _, e := range x {
			assertHomogenized(t, e)
		}
	case []string, []float64, []map[string]any, string, bool, float64, nil:
		// clean shapes CoerceWire is allowed to return
	}
}
