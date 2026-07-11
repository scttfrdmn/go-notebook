package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/gen"
	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// cmdBuild implements `notebook build <dir|file>`: analyze → codegen → overlay
// → `go build`, emitting a binary. The user's source tree is never modified.
//
// Flags:
//
//	-o <path>   output binary path (default: ./<pkgname>)
//	--timing    print codegen + build wall time to stderr
func cmdBuild(args []string) int {
	var (
		out    string
		timing bool
		target string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-o":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "notebook build: -o needs an argument")
				return 2
			}
			i++
			out = args[i]
		case "--timing":
			timing = true
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "notebook build: unexpected extra argument %q\n", a)
				return 2
			}
			target = a
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "notebook build: need a directory or file")
		return 2
	}

	dir, err := resolveDir(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}
	// Errors block the build; notices (deferred features) are printed but the
	// runnable subset still compiles.
	var errCount int
	for _, d := range res.Diagnostics {
		fmt.Fprintln(os.Stderr, d.String())
		if d.Severity == graph.Error {
			errCount++
		}
	}
	if errCount > 0 {
		fmt.Fprintf(os.Stderr, "\nnotebook build: %d error(s); not building\n", errCount)
		return 1
	}

	moduleRoot, err := findModuleRoot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	genStart := time.Now()
	overlay, err := gen.Build(res.Graph, res.Package, moduleRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: codegen: %v\n", err)
		return 1
	}
	defer overlay.Cleanup()
	genElapsed := time.Since(genStart)

	if out == "" {
		out = "./" + res.Package.Name
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	buildStart := time.Now()
	cmd := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", absOut, overlay.MainDir)
	cmd.Dir = moduleRoot
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: go build: %v\n", err)
		return 1
	}
	buildElapsed := time.Since(buildStart)

	fmt.Fprintf(os.Stderr, "built %s\n", absOut)
	if timing {
		fmt.Fprintf(os.Stderr, "codegen: %v   go build: %v   total: %v\n",
			genElapsed, buildElapsed, genElapsed+buildElapsed)
	}
	return 0
}

// findModuleRoot walks up from dir to the directory containing go.mod.
func findModuleRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		abs = parent
	}
}
