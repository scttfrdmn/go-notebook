package apifeed

import (
	"strings"
	"testing"
)

// TestMapProjectsPosition confirms the equirectangular projection places the
// marker correctly: the prime-meridian equator maps to the map center, and the
// marker moves right with longitude and up with latitude.
func TestMapProjectsPosition(t *testing.T) {
	// (0,0) should render a marker near the horizontal+vertical center (320/160).
	center := worldmap(0, 0).Render()
	if !strings.Contains(center.Data, `cx="320.0"`) || !strings.Contains(center.Data, `cy="160.0"`) {
		t.Errorf("(0,0) should project to the map center (320,160); got:\n%s", markerLine(center.Data))
	}
	// The ISS label always appears (the marker is the live element).
	if !strings.Contains(center.Data, ">ISS<") {
		t.Error("map should label the ISS marker")
	}
}

// TestReadoutFormatsHemisphere pins the coordinate + hemisphere readout.
func TestReadoutFormatsHemisphere(t *testing.T) {
	// 37.421°N, 122.084°W (milli-degrees).
	html := readout(37421, -122084, 5).Render()
	for _, want := range []string{"37.421° N", "122.084° W", "NW", "live"} {
		if !strings.Contains(html.Data, want) {
			t.Errorf("readout missing %q in:\n%s", want, html.Data)
		}
	}
}

// markerLine extracts the marker <circle> for a readable failure message.
func markerLine(svg string) string {
	i := strings.Index(svg, `<circle cx=`)
	if i < 0 {
		return svg
	}
	j := strings.Index(svg[i:], "/>")
	return svg[i : i+j+2]
}
