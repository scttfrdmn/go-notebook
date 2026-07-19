package chart_test

import (
	"fmt"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb/chart"
)

// A cell returns a chart value; the engine calls its Render. Line names each
// series at its own end, so two series need no legend box.
func ExampleLine() {
	c := chart.Line(
		chart.Series{Name: "2024", XY: []chart.Pt{{1, 10}, {2, 14}, {3, 19}}},
		chart.Series{Name: "2025", XY: []chart.Pt{{1, 12}, {2, 17}, {3, 25}}},
	)
	out := c.Render()
	fmt.Println(out.MIME)
	fmt.Println(strings.HasPrefix(out.Data, "<svg"))
	// Output:
	// image/svg+xml
	// true
}

// The summary statistics are pure functions, usable with or without the charts.
func ExampleLinFit() {
	xs := []float64{0, 1, 2, 3}
	ys := []float64{1, 4, 7, 10} // y = 3x + 1
	slope, intercept := chart.LinFit(xs, ys)
	fmt.Printf("y = %.0fx + %.0f\n", slope, intercept)
	// Output:
	// y = 3x + 1
}

// Table reflects a slice of structs into an HTML table: field names become
// headers, numeric columns right-align.
func ExampleRows() {
	type Row struct {
		Product string
		Price   float64
	}
	c := chart.Rows([]Row{{"Widget", 4.50}, {"Gadget", 12.00}})
	fmt.Println(c.Render().MIME)
	// Output:
	// text/html
}
