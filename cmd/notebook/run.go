package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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

	// Persist the head next to the notebook source so restarts restore sliders.
	headPath := filepath.Join(dir, "notebook-head.json")

	// buildAndLaunch rebuilds the notebook and starts the binary, returning the
	// running process. The caller stops the previous one first.
	buildAndLaunch := func() (*exec.Cmd, func(), error) {
		res, err := analyze.LoadPackage(dir)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range res.Diagnostics {
			if d.Severity == graph.Error {
				return nil, nil, fmt.Errorf("%s", d.String())
			}
			fmt.Fprintln(os.Stderr, d.String())
		}
		buildStart := time.Now()
		overlay, err := gen.Build(res.Graph, res.Package, moduleRoot)
		if err != nil {
			return nil, nil, err
		}
		bin := filepath.Join(overlay.TempDir(), "notebook-bin")
		build := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", bin, overlay.MainDir)
		build.Dir = moduleRoot
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			overlay.Cleanup()
			return nil, nil, fmt.Errorf("go build: %w", err)
		}
		if timing {
			fmt.Fprintf(os.Stderr, "build: %v\n", time.Since(buildStart))
		}
		nb := exec.Command(bin, "--addr", addr, "--head", headPath)
		nb.Stdout = os.Stdout
		nb.Stderr = os.Stderr
		if err := nb.Start(); err != nil {
			overlay.Cleanup()
			return nil, nil, fmt.Errorf("launch: %w", err)
		}
		return nb, overlay.Cleanup, nil
	}

	proc, cleanup, err := buildAndLaunch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}

	if !noOpen {
		time.Sleep(150 * time.Millisecond)
		openBrowser("http://" + addr)
	}

	// Watch the source and rebuild-on-save. Polling os.Stat mtime at 100ms is
	// dependency-free and completely adequate for a package's handful of files —
	// no fsnotify. On a change: stop the process, rebuild, relaunch. The head is
	// persisted to disk, so the new process restores the sliders (restart is a
	// non-event). This is the interactive edit loop; without it there is no loop.
	watch := watchFiles(res.Package.GoFiles)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for {
		select {
		case <-sig:
			_ = proc.Process.Kill()
			cleanup()
			return 0
		case <-watch:
			fmt.Fprintln(os.Stderr, "notebook run: change detected, rebuilding…")
			_ = proc.Process.Kill()
			_, _ = proc.Process.Wait()
			cleanup()
			proc, cleanup, err = buildAndLaunch()
			if err != nil {
				// A broken edit shouldn't kill the loop — report and keep
				// watching so the next save can fix it.
				fmt.Fprintf(os.Stderr, "notebook run: rebuild failed: %v\n", err)
				proc, cleanup = nil, func() {}
				continue
			}
		}
	}
}

// watchFiles polls the given files' mtimes every 100ms and sends on the
// returned channel when any changes. Dependency-free (os.Stat), adequate for a
// package's files. The goroutine runs for the process lifetime.
func watchFiles(files []string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	last := make(map[string]time.Time, len(files))
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			last[f] = fi.ModTime()
		}
	}
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			for _, f := range files {
				fi, err := os.Stat(f)
				if err != nil {
					continue
				}
				if mt := fi.ModTime(); mt.After(last[f]) {
					last[f] = mt
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	return ch
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
