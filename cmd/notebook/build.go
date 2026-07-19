package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/gen"
)

// cmdBuild implements `notebook build <dir|file>`: analyze → codegen → overlay
// → `go build`, emitting a binary. The user's source tree is never modified.
//
// Flags:
//
//	-o <path>   output binary path (default: ./<pkgname>)
//	--timing    print codegen + build wall time to stderr
func cmdBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "output path (binary, or a directory for --target=wasm)")
	target := fs.String("target", "native", "build target: native | wasm")
	timing := fs.Bool("timing", false, "print codegen + build wall time")
	showcase := fs.Bool("showcase", false, "(wasm) lead with the dependency graph open, for gallery demos")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir, code := targetDir(fs, "build")
	if code != 0 {
		return code
	}

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}
	// Errors block the build; notices (deferred features) are printed but the
	// runnable subset still compiles.
	if reportDiagnostics(res.Diagnostics, "build") {
		return 1
	}

	moduleRoot, err := findModuleRoot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: %v\n", err)
		return 1
	}

	if *target == "wasm" {
		return buildWASM(res, moduleRoot, *out, *timing, *showcase)
	}

	genStart := time.Now()
	overlay, err := gen.Build(res.Graph, res.Package, moduleRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook build: codegen: %v\n", err)
		return 1
	}
	defer overlay.Cleanup()
	genElapsed := time.Since(genStart)

	outPath := *out
	if outPath == "" {
		outPath = "./" + res.Package.Name
	}
	absOut, err := filepath.Abs(outPath)
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
	if *timing {
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
