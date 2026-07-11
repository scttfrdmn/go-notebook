package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/go-notebook/engine"
)

// chartValue is a test output that renders to SVG-like data, using its OWN
// Rendered-shaped type (no engine import) — exactly the notebook's situation,
// so this exercises the reflection probe.
type chartValue struct{ n int }

type nbRendered struct {
	MIME string
	Data string
}

func (c chartValue) Render() nbRendered {
	return nbRendered{MIME: "image/svg+xml", Data: "<svg>" + itoa(c.n) + "</svg>"}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// testRuntime builds a tiny notebook: leaf n -> chart (renders SVG from n).
func testRuntime(t *testing.T) (*engine.Runtime, []engine.CellMeta) {
	t.Helper()
	chart := fnNode{
		id: "chart", in: []engine.Symbol{"n"}, out: []engine.Symbol{"c"}, pure: true,
		run: func(_ context.Context, in engine.Inputs) (engine.Outputs, error) {
			return engine.Outputs{"c": chartValue{n: in["n"].(int)}}, nil
		},
	}
	head := engine.NewHead()
	head.Set("n", 0) // seed the leaf so the initial wave doesn't hit a nil input
	rt := engine.NewRuntime(engine.Config{
		Nodes:  []engine.Node{chart},
		Leaves: []engine.LeafID{"n"},
	}, head, engine.NewMemoStore())
	meta := []engine.CellMeta{
		{ID: "n", Label: "count", Directives: map[string]string{"slider": "", "min": "0", "max": "100"}},
		{ID: "chart", Label: "chart"},
	}
	return rt, meta
}

// fnNode mirrors the engine test helper (package-local copy for the server test).
type fnNode struct {
	id   engine.CellID
	in   []engine.Symbol
	out  []engine.Symbol
	pure bool
	run  func(ctx context.Context, in engine.Inputs) (engine.Outputs, error)
}

func (n fnNode) ID() engine.CellID    { return n.id }
func (n fnNode) In() []engine.Symbol  { return n.in }
func (n fnNode) Out() []engine.Symbol { return n.out }
func (n fnNode) Pure() bool           { return n.pure }
func (n fnNode) Run(ctx context.Context, in engine.Inputs) (engine.Outputs, error) {
	return n.run(ctx, in)
}

// TestServerEditRepaints drives the real HTTP surface: open the SSE stream, POST
// a leaf edit, and confirm a rendered SVG event for the downstream chart arrives.
// This is the slider→repaint path KC3 measures, exercised end-to-end.
func TestServerEditRepaints(t *testing.T) {
	rt, meta := testRuntime(t)
	srv := httptest.NewServer(New(rt, meta, nil).Handler())
	defer srv.Close()

	// Open the event stream.
	req, _ := http.NewRequest("GET", srv.URL+"/events", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Post an edit.
	go func() {
		time.Sleep(50 * time.Millisecond)
		body := bytes.NewBufferString(`{"leaf":"n","value":42}`)
		_, _ = http.Post(srv.URL+"/set", "application/json", body)
	}()

	// Scan the stream for a rendered chart event carrying the SVG.
	scanner := bufio.NewScanner(resp.Body)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for a rendered chart event")
		default:
		}
		if !scanner.Scan() {
			t.Fatal("event stream closed before a chart repaint")
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev wireEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		if ev.Cell == "chart" && ev.State == "done" && ev.MIME == "image/svg+xml" {
			if !strings.Contains(ev.Data, "<svg>") {
				t.Errorf("chart data not SVG: %q", ev.Data)
			}
			return // repaint observed
		}
	}
}

// TestKC3RepaintLatency measures slider→repaint latency: from a leaf edit to
// the downstream rendered event. It measures at the engine's event boundary
// (the value the transport serializes), which is deterministic — an HTTP
// scanner can stall on buffering and would measure the test harness, not the
// system. The transport adds a localhost round-trip (~sub-ms, measured
// separately in TestSetRoundTrip); the meaningful cost is the wave + render.
//
// Reports p50/p95 over sequential, non-overlapping edits — overlapping drags
// are deliberately coalesced (a superseded edit's repaint never arrives), so a
// per-edit latency benchmark must not overlap them. Target p95 < 50ms.
func TestKC3RepaintLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("latency measurement skipped in -short mode")
	}
	rt, _ := testRuntime(t)
	sub := rt.Subscribe()

	const samples = 50
	var latencies []time.Duration
	for i := 1; i <= samples; i++ {
		val := i * 7
		want := "<svg>" + itoa(val) + "</svg>"
		start := time.Now()
		rt.Set(context.Background(), "n", val)
		// Drain events until the chart's rendered repaint for this value.
		deadline := time.After(2 * time.Second)
		for {
			select {
			case ev := <-sub:
				if ev.Cell == "chart" && ev.State == engine.StateDone && ev.Out != nil &&
					strings.Contains(ev.Out.Data, want) {
					latencies = append(latencies, time.Since(start))
					goto next
				}
			case <-deadline:
				t.Fatalf("edit %d (value %d): no repaint within 2s", i, val)
			}
		}
	next:
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)/2]
	p95 := latencies[int(float64(len(latencies))*95/100)]
	t.Logf("KC3 slider→repaint (engine boundary) over %d samples: p50=%v p95=%v (target p95<50ms)", samples, p50, p95)
	if p95 > 50*time.Millisecond {
		t.Errorf("KC3 p95 = %v, want < 50ms", p95)
	}
}

// TestSetRoundTrip measures the transport overhead KC3 omits: one POST /set
// localhost round-trip. Added to KC3's engine-boundary number, this is the true
// end-to-end slider→repaint.
func TestSetRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("latency measurement skipped in -short mode")
	}
	rt, meta := testRuntime(t)
	srv := httptest.NewServer(New(rt, meta, nil).Handler())
	defer srv.Close()

	const samples = 50
	var rtt []time.Duration
	for i := 0; i < samples; i++ {
		start := time.Now()
		body := bytes.NewBufferString(`{"leaf":"n","value":1}`)
		resp, err := http.Post(srv.URL+"/set", "application/json", body)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		rtt = append(rtt, time.Since(start))
	}
	sort.Slice(rtt, func(i, j int) bool { return rtt[i] < rtt[j] })
	t.Logf("POST /set localhost round-trip: p50=%v p95=%v", rtt[len(rtt)/2], rtt[samples*95/100])
}
