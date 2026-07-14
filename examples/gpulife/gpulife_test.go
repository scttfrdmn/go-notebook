package gpulife

import (
	"strings"
	"testing"
)

// TestInitialPure confirms the seed grid is a pure function of (n, pct, s): same
// sliders → identical grid. This is the corpus's no-hidden-state rule — the reason
// the notebook is reproducible and the cell caches. Go owns this half.
func TestInitialPure(t *testing.T) {
	a := initial(64, 30, 42)
	b := initial(64, 30, 42)
	if len(a.Cells) != len(b.Cells) {
		t.Fatalf("length differs")
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Fatalf("initial not pure at %d: %d vs %d", i, a.Cells[i], b.Cells[i])
		}
	}
}

// TestInitialShape checks the grid is n×n and cells are strictly 0/1.
func TestInitialShape(t *testing.T) {
	g := initial(128, 30, 7)
	if g.N != 128 || len(g.Cells) != 128*128 {
		t.Fatalf("shape wrong: N=%d len=%d", g.N, len(g.Cells))
	}
	for i, c := range g.Cells {
		if c > 1 {
			t.Fatalf("cell %d = %d, must be 0 or 1", i, c)
		}
	}
}

// TestDensityMatches confirms the density slider actually controls the live fraction:
// the alive share is close to pct/100, and higher pct gives more live cells. If this
// drifts, the slider is decoration.
func TestDensityMatches(t *testing.T) {
	frac := func(pct int) float64 {
		g := initial(200, pct, 3)
		var live int
		for _, c := range g.Cells {
			if c == 1 {
				live++
			}
		}
		return float64(live) / float64(len(g.Cells))
	}
	lo, hi := frac(10), frac(50)
	if lo < 0.06 || lo > 0.14 {
		t.Errorf("10%% density gave %.1f%% live, want ~10%%", lo*100)
	}
	if hi < 0.44 || hi > 0.56 {
		t.Errorf("50%% density gave %.1f%% live, want ~50%%", hi*100)
	}
	if hi <= lo {
		t.Errorf("higher density did not give more live cells: %.2f vs %.2f", hi, lo)
	}
}

// TestSeedMatters confirms different seeds give different grids (else the seed
// slider does nothing).
func TestSeedMatters(t *testing.T) {
	a := initial(64, 30, 1)
	b := initial(64, 30, 2)
	same := 0
	for i := range a.Cells {
		if a.Cells[i] == b.Cells[i] {
			same++
		}
	}
	// Two random grids agree on ~58% of cells by chance (p²+(1-p)² at p=0.3);
	// identical grids would agree on 100%. Guard well below that.
	if float64(same)/float64(len(a.Cells)) > 0.8 {
		t.Errorf("seeds 1 and 2 gave near-identical grids (%d/%d agree)", same, len(a.Cells))
	}
}

// TestSceneHTMLStructure is the escape-hatch wiring test: the rendered HTML must be
// text/html, carry the seed grid + size in the canvas dataset, bootstrap from an
// onerror handler (innerHTML <script> doesn't run), acquire WebGPU, define a compute
// shader, and degrade honestly when WebGPU is absent. These are what make the GPU
// actually receive the grid; WGSL *execution* is verified by the browser screenshot.
func TestSceneHTMLStructure(t *testing.T) {
	r := scene(initial(8, 30, 1)).Render()
	if r.MIME != "text/html" {
		t.Fatalf("MIME = %q, want text/html", r.MIME)
	}
	for _, want := range []string{
		`<canvas`,       // the draw target
		`data-n="8"`,    // grid size for the shaders
		`data-cells=`,   // the seed grid payload
		`onerror=`,      // bootstrap trigger (script tags don't run via innerHTML)
		`navigator.gpu`, // WebGPU entry
		`@compute`,      // the compute shader — the whole point
		`createComputePipeline`,
		`dispatchWorkgroups`, // the parallel dispatch
		`not available`,      // the honest fallback when WebGPU is missing
	} {
		if !strings.Contains(r.Data, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

// TestSeedGridPayloadLength confirms the packed cell string is exactly n*n chars of
// 0/1 — the JS reads it by index, so a wrong length would corrupt the upload.
func TestSeedGridPayloadLength(t *testing.T) {
	n := 16
	r := scene(initial(n, 40, 5)).Render()
	// data-cells='...'; extract between data-cells=" and the next "
	i := strings.Index(r.Data, `data-cells="`)
	if i < 0 {
		t.Fatal("no data-cells attribute")
	}
	rest := r.Data[i+len(`data-cells="`):]
	j := strings.IndexByte(rest, '"')
	payload := rest[:j]
	if len(payload) != n*n {
		t.Errorf("payload length %d, want %d (n*n)", len(payload), n*n)
	}
	for k := 0; k < len(payload); k++ {
		if payload[k] != '0' && payload[k] != '1' {
			t.Fatalf("payload char %d is %q, want 0 or 1", k, payload[k])
		}
	}
}

// TestEscapeAttr guards the attribute boundary: the payload and the handler live in
// HTML attributes, so quotes/brackets must be escaped or a value breaks out.
func TestEscapeAttr(t *testing.T) {
	got := escapeAttr(`a"b'c<d>e&f`)
	for _, raw := range []string{`"`, `'`, `<`, `>`} {
		if strings.Contains(got, raw) {
			t.Errorf("escapeAttr left a raw %q in %q", raw, got)
		}
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("escapeAttr did not escape &: %q", got)
	}
}
