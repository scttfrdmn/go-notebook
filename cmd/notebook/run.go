package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/scttfrdmn/go-notebook/internal/analyze"
	"github.com/scttfrdmn/go-notebook/internal/gen"
	"github.com/scttfrdmn/go-notebook/internal/graph"
)

// cmdRun implements `notebook run <dir|file>`: analyze → codegen → build →
// launch the notebook binary → open a browser. It is the interactive entry
// point.
//
// Flags:
//
//	--addr <host:port>   listen address (default 127.0.0.1:8080)
//	--no-open            don't open a browser
//	--timing             print build wall time to stderr
func cmdRun(args []string) int {
	var (
		addr   = "127.0.0.1:8080"
		noOpen bool
		timing bool
		target string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--addr":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "notebook run: --addr needs an argument")
				return 2
			}
			i++
			addr = args[i]
		case "--no-open":
			noOpen = true
		case "--timing":
			timing = true
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "notebook run: unexpected extra argument %q\n", a)
				return 2
			}
			target = a
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "notebook run: need a directory or file")
		return 2
	}

	dir, err := resolveDir(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}
	var errCount int
	for _, d := range res.Diagnostics {
		fmt.Fprintln(os.Stderr, d.String())
		if d.Severity == graph.Error {
			errCount++
		}
	}
	if errCount > 0 {
		fmt.Fprintf(os.Stderr, "\nnotebook run: %d error(s); not running\n", errCount)
		return 1
	}

	moduleRoot, err := findModuleRoot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}

	buildStart := time.Now()
	overlay, err := gen.Build(res.Graph, res.Package, moduleRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: codegen: %v\n", err)
		return 1
	}
	defer overlay.Cleanup()

	bin := filepath.Join(overlay.TempDir(), "notebook-bin")
	build := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", bin, overlay.MainDir)
	build.Dir = moduleRoot
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: go build: %v\n", err)
		return 1
	}
	if timing {
		fmt.Fprintf(os.Stderr, "build: %v\n", time.Since(buildStart))
	}

	// Persist the head next to the notebook source so restarts restore sliders.
	headPath := filepath.Join(dir, "notebook-head.json")

	nb := exec.Command(bin, "--addr", addr, "--head", headPath)
	nb.Stdout = os.Stdout
	nb.Stderr = os.Stderr
	if err := nb.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: launch: %v\n", err)
		return 1
	}

	if !noOpen {
		// Give the server a moment to bind before opening the browser.
		time.Sleep(150 * time.Millisecond)
		openBrowser("http://" + addr)
	}

	if err := nb.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}
	return 0
}

// openBrowser opens url in the platform default browser, best-effort.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}
