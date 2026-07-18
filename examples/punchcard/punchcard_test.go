package punchcard

import (
	"strings"
	"testing"
)

// TestBusyPatternMatchesModel pins the teaching claim: on the defaults the
// cluster is busiest during weekday business hours and has the most headroom on a
// weekend night. If the model stops producing that shape, the punchcard is
// decorative, not informative.
func TestBusyPatternMatchesModel(t *testing.T) {
	g := grid(weekdayPeak(), weekendFraction(), batchIntensity(), baseline())

	// A weekday mid-afternoon cell should be busier than a weekend pre-dawn cell.
	monAfternoon := g.Cells[0][14] // Mon 14:00
	satNight := g.Cells[5][5]      // Sat 05:00
	if monAfternoon <= satNight {
		t.Errorf("Mon 2pm (%d%%) should be busier than Sat 5am (%d%%)", monAfternoon, satNight)
	}

	// The extremes cell should name a weekday hour as busiest and a low value as
	// the most-headroom slot.
	r := extremes(g)
	if len(r.Cards) != 2 {
		t.Fatalf("extremes should report 2 cards, got %d", len(r.Cards))
	}
	if !strings.Contains(r.Cards[0].Label, "busiest") {
		t.Error("first card should be the busiest hour")
	}
}

// TestWeekendIsQuieter confirms the weekend fraction actually scales the weekend
// down: at 0% the weekend business bump vanishes; at 100% it matches weekdays.
func TestWeekendIsQuieter(t *testing.T) {
	quiet := grid(90, 0, 0, 0)   // weekend fraction 0, no batch/floor
	equal := grid(90, 100, 0, 0) // weekend fraction 100

	// Saturday 2pm: near-zero when fraction is 0, substantial when 100.
	if quiet.Cells[5][14] > 5 {
		t.Errorf("weekend at 0%% fraction should be ~idle at 2pm, got %d%%", quiet.Cells[5][14])
	}
	if equal.Cells[5][14] <= quiet.Cells[5][14] {
		t.Error("weekend at 100%% fraction should be busier than at 0%%")
	}
	// A weekday is unaffected by the weekend fraction.
	if grid(90, 0, 0, 0).Cells[0][14] != grid(90, 100, 0, 0).Cells[0][14] {
		t.Error("the weekend fraction must not change weekday load")
	}
}

// TestBatchWindowIsNightly confirms the batch spike lands in the small hours,
// every day (including weekends), independent of the business cycle.
func TestBatchWindowIsNightly(t *testing.T) {
	withBatch := grid(0, 0, 100, 0) // only the batch window, nothing else
	// 2am should be lit on every day; 2pm should be ~dark.
	for d := 0; d < 7; d++ {
		if withBatch.Cells[d][2] == 0 {
			t.Errorf("day %d: 2am batch window should be lit", d)
		}
		if withBatch.Cells[d][14] != 0 {
			t.Errorf("day %d: 2pm should be dark with only the batch window on (got %d)", d, withBatch.Cells[d][14])
		}
	}
}

// TestValuesClampTo100 guards against a cell exceeding 100% when the bumps stack
// (business + batch + floor) — a heatmap tint over 100 would misread.
func TestValuesClampTo100(t *testing.T) {
	g := grid(100, 100, 100, 60) // everything maxed
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			if g.Cells[d][h] > 100 || g.Cells[d][h] < 0 {
				t.Fatalf("cell [%d][%d] = %d, out of 0..100", d, h, g.Cells[d][h])
			}
		}
	}
}

// TestHeatmapRenders confirms the CSS-grid view reaches the page with the grid,
// the hover titles, and the legend — the design is deferred to HTML/CSS, but the
// picture must actually render (running is not passing).
func TestHeatmapRenders(t *testing.T) {
	g := grid(weekdayPeak(), weekendFraction(), batchIntensity(), baseline())
	html := punchcard(g).Render()
	if html.MIME != "text/html" {
		t.Fatalf("punchcard MIME = %q, want text/html", html.MIME)
	}
	for _, want := range []string{"display:grid", "title=", "utilized", "Mon", "idle", "busy"} {
		if !strings.Contains(html.Data, want) {
			t.Errorf("rendered punchcard missing %q", want)
		}
	}
}
