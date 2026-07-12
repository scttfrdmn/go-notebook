package main

import (
	"flag"
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
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timing := fs.Bool("timing", false, "print KC1 (graph-derivation) wall time")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir, code := targetDir(fs, "check")
	if code != 0 {
		return code
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

	if *timing {
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

// targetDir reads the single positional argument (a dir or file) from a parsed
// flag set and resolves it to the package directory. It reports a usage error
// (exit 2) if the argument is missing or duplicated, or a load error (exit 1) if
// the path is bad; a zero code means dir is valid.
func targetDir(fs *flag.FlagSet, cmd string) (dir string, code int) {
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintf(os.Stderr, "notebook %s: need a directory or file\n", cmd)
		return "", 2
	}
	if len(rest) > 1 {
		fmt.Fprintf(os.Stderr, "notebook %s: unexpected extra argument %q\n", cmd, rest[1])
		return "", 2
	}
	dir, err := resolveDir(rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook %s: %v\n", cmd, err)
		return "", 1
	}
	return dir, 0
}

// reportDiagnostics prints diagnostics to stderr and reports whether any are
// blocking errors (as opposed to notices for deferred features). A true return
// means the command should stop with a non-zero exit.
func reportDiagnostics(diags []graph.Diagnostic, cmd string) (hasErrors bool) {
	var errCount int
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
		if d.Severity == graph.Error {
			errCount++
		}
	}
	if errCount > 0 {
		fmt.Fprintf(os.Stderr, "\nnotebook %s: %d error(s); not proceeding\n", cmd, errCount)
		return true
	}
	return false
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
