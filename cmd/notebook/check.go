package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// cmdCheck implements `notebook check <dir|file>`: it analyzes the notebook,
// prints its dependency graph, reports any diagnostics, and returns a non-zero
// exit code if the notebook has errors.
//
// Flags:
//
//	--timing   print KC1 (graph-derivation) wall time to stderr
func cmdCheck(args []string) int {
	var timing bool
	var target string
	for _, a := range args {
		switch a {
		case "--timing":
			timing = true
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "notebook check: unexpected extra argument %q\n", a)
				return 2
			}
			target = a
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "notebook check: need a directory or file")
		return 2
	}

	dir, err := resolveDir(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook check: %v\n", err)
		return 1
	}

	start := time.Now()
	g, diags, err := analyze.TypesAnalyzer{}.Analyze(dir)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook check: %v\n", err)
		return 1
	}

	io.WriteString(os.Stdout, renderGraph(g)) //nolint:errcheck // stdout

	if len(diags) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, d := range diags {
			fmt.Fprintln(os.Stderr, d.String())
		}
		fmt.Fprintf(os.Stderr, "\n%d diagnostic(s)\n", len(diags))
	}

	if timing {
		fmt.Fprintf(os.Stderr, "\nKC1 graph derivation: %v\n", elapsed)
	}

	if len(diags) > 0 {
		return 1
	}
	return 0
}

// resolveDir turns a target that may be a directory or a file into the
// directory to analyze. A package is the unit go/packages loads, so a file
// target is reduced to its parent directory.
func resolveDir(target string) (string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return target, nil
	}
	return filepath.Dir(target), nil
}

// renderGraph produces a human-readable rendering of the graph: cells in source
// order, each with its label, purity, parameters (with edge sources), and
// results. The format is meant to be read at a glance and to make the wiring
// obvious.
func renderGraph(g *graph.Graph) string {
	var w strings.Builder
	fmt.Fprintf(&w, "graph: %d cells\n", len(g.Cells))
	for _, id := range g.Order {
		c := g.Cells[id]
		purity := "pure"
		if !c.Pure {
			purity = "impure"
		}
		fmt.Fprintf(&w, "\n  %s  [%s]\n", id, purity)
		if c.Label != "" && c.Label != string(id) {
			fmt.Fprintf(&w, "    label: %s\n", c.Label)
		}
		for _, p := range c.Params {
			switch p.Kind {
			case graph.Wired:
				src := "?"
				if producer, ok := g.Producer[p.Name]; ok {
					src = string(producer)
				}
				fmt.Fprintf(&w, "    in   %s %s  <- %s\n", p.Name, p.Type, src)
			case graph.Injected:
				fmt.Fprintf(&w, "    in   %s %s  (injected)\n", p.Name, p.Type)
			case graph.Delayed:
				fmt.Fprintf(&w, "    in   %s %s  (delayed/prev)\n", p.Name, p.Type)
			}
		}
		for _, r := range c.Results {
			if r.IsError {
				fmt.Fprintf(&w, "    err  %s %s\n", r.Name, r.Type)
				continue
			}
			fmt.Fprintf(&w, "    out  %s %s\n", r.Name, r.Type)
		}
	}
	if len(g.Helpers) > 0 {
		fmt.Fprintf(&w, "\nhelpers (not cells — they name no result): ")
		for i, h := range g.Helpers {
			if i > 0 {
				fmt.Fprint(&w, ", ")
			}
			fmt.Fprint(&w, string(h))
		}
		fmt.Fprintln(&w)
	}
	return w.String()
}
