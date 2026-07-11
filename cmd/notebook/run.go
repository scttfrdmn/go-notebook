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

	// build produces a fresh binary (analyze → codegen → go build) while the old
	// one keeps serving. Returns the binary path and a cleanup for its overlay
	// temp dir.
	//
	// NOTE: an earlier version "warmed" the new binary with a throwaway headless
	// run to pay off the OS first-exec cost before the swap. Measured on lego,
	// that was a net LOSS — warming runs a full wave (~460ms for lego), far more
	// than the ~180ms first-exec it saves. Removed. The first-exec cost is paid
	// at swap time, which is honest and cheaper than pre-warming.
	build := func() (bin string, cleanup func(), err error) {
		res, err := analyze.LoadPackage(dir)
		if err != nil {
			return "", nil, err
		}
		for _, d := range res.Diagnostics {
			if d.Severity == graph.Error {
				return "", nil, fmt.Errorf("%s", d.String())
			}
			fmt.Fprintln(os.Stderr, d.String())
		}
		overlay, err := gen.Build(res.Graph, res.Package, moduleRoot)
		if err != nil {
			return "", nil, err
		}
		bin = filepath.Join(overlay.TempDir(), "notebook-bin")
		cmd := exec.Command("go", "build", "-overlay="+overlay.JSONPath, "-o", bin, overlay.MainDir)
		cmd.Dir = moduleRoot
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			overlay.Cleanup()
			return "", nil, fmt.Errorf("go build: %w", err)
		}
		return bin, overlay.Cleanup, nil
	}

	// launch starts a built binary serving on addr.
	launch := func(bin string) (*exec.Cmd, error) {
		nb := exec.Command(bin, "--addr", addr, "--head", headPath)
		nb.Stdout = os.Stdout
		nb.Stderr = os.Stderr
		if err := nb.Start(); err != nil {
			return nil, fmt.Errorf("launch: %w", err)
		}
		return nb, nil
	}

	buildStart := time.Now()
	bin, cleanup, err := build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}
	proc, err := launch(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		cleanup()
		return 1
	}
	if timing {
		fmt.Fprintf(os.Stderr, "startup: %v\n", time.Since(buildStart))
	}

	if !noOpen {
		time.Sleep(150 * time.Millisecond)
		openBrowser("http://" + addr)
	}

	// Watch the source and rebuild-on-save (os.Stat poll, 100ms, zero deps).
	//
	// The rebuild OVERLAPS the running binary (#22): on change we build AND warm
	// the new binary while the old one keeps serving — the user stays
	// interactive through the ~270ms build + ~180ms first-exec. Only once the
	// new binary is ready do we kill the old and launch the new, so the
	// user-visible latency is the swap (~a warm re-exec + reconnect), not
	// build + first-exec + reconnect. The head is persisted, so the new process
	// restores the sliders. This is what gives the edit loop margin at scale.
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
			// Build + warm the NEW binary while the OLD one keeps serving, so
			// the notebook stays interactive (sliders still respond) during the
			// rebuild rather than going dark. This does NOT shrink time-to-
			// reflect-the-edit — that is fundamentally build + exec — but it
			// keeps the old result live and responsive until the new one is
			// ready.
			buildT := time.Now()
			newBin, newCleanup, err := build()
			if err != nil {
				// A broken edit shouldn't kill the loop or the running notebook;
				// keep serving the old binary and wait for the next save.
				fmt.Fprintf(os.Stderr, "notebook run: rebuild failed (still serving previous build): %v\n", err)
				continue
			}
			buildDur := time.Since(buildT)
			// Swap: stop the old, launch the new (already warm, ~fast re-exec).
			swapT := time.Now()
			_ = proc.Process.Kill()
			_, _ = proc.Process.Wait()
			cleanup()
			newProc, err := launch(newBin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "notebook run: relaunch failed: %v\n", err)
				newCleanup()
				return 1
			}
			proc, cleanup = newProc, newCleanup
			if timing {
				fmt.Fprintf(os.Stderr, "rebuild+warm: %v   swap: %v   (edit reflected in %v)\n",
					buildDur, time.Since(swapT), time.Since(buildT))
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
