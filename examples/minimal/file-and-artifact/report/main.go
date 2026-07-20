// Command report writes the file-and-artifact notebook's chart to a file.
//
// This is the OUT boundary of the recipe: a cell cannot write a file (I/O would
// break its purity), so materializing an artifact is a separate program's job.
// It imports the notebook package and calls its cells as the ordinary Go
// functions they are — `notebook.Chart(notebook.Rows())` is the whole pipeline —
// then does the one impure thing, os.WriteFile, here, outside the graph.
//
// The chart's Render() returns SVG markup (the same bytes the notebook UI paints);
// writing them to a .svg file makes a standalone image you can open in a browser
// or drop into a report. Point --out elsewhere to write it somewhere else.
//
//	go tool notebook run ./examples/minimal/file-and-artifact   # see it interactively
//	go run ./examples/minimal/file-and-artifact/report          # write downloads.svg
//	go run ./examples/minimal/file-and-artifact/report --out /tmp/dl.svg
package main

import (
	"flag"
	"log"
	"os"

	notebook "github.com/scttfrdmn/go-notebook/examples/minimal/file-and-artifact"
)

func main() {
	out := flag.String("out", "downloads.svg", "path to write the chart SVG to")
	flag.Parse()

	// Call the cells directly — no runtime, no server, no wiring. The notebook is
	// an ordinary Go package, so its graph is just function composition here.
	rows := notebook.Rows()
	svg := notebook.Chart(rows).Render().Data

	if err := os.WriteFile(*out, []byte(svg), 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s (%d rows, %d bytes of SVG)", *out, len(rows), len(svg))
}
