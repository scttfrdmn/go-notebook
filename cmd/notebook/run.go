package main

import (
	"flag"
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
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	noOpen := fs.Bool("no-open", false, "don't open a browser")
	timing := fs.Bool("timing", false, "print build/swap timing to stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir, code := targetDir(fs, "run")
	if code != 0 {
		return code
	}

	res, err := analyze.LoadPackage(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}
	if reportDiagnostics(res.Diagnostics, "run") {
		return 1
	}

	moduleRoot, err := findModuleRoot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return 1
	}

	// Persist the head next to the notebook source so restarts restore sliders.
	headPath := filepath.Join(dir, "notebook-head.json")

	build := func() (string, func(), error) { return buildBinary(dir, moduleRoot) }

	// launch starts a built binary serving on addr.
	launch := func(bin string) (*exec.Cmd, error) {
		nb := exec.Command(bin, "--addr", *addr, "--head", headPath)
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
	if *timing {
		fmt.Fprintf(os.Stderr, "startup: %v\n", time.Since(buildStart))
	}

	if !*noOpen {
		time.Sleep(150 * time.Millisecond)
		openBrowser("http://" + *addr)
	}

	return watchAndRebuild(res.Package.GoFiles, build, launch, proc, cleanup, *timing)
}

// watchAndRebuild serves the notebook and, on every source save, rebuilds the
// binary while the old one keeps serving, then swaps to it. Blocks until
// interrupted (SIGINT), returning the process exit code.
//
// The rebuild OVERLAPS the running binary (#22): building happens while the old
// binary answers, so the notebook stays interactive during a rebuild instead of
// going dark. It does not shrink time-to-reflect-an-edit — that is fundamentally
// build + exec — but the old result stays live and responsive until the new one
// is ready. The head is persisted, so the swapped-in process restores the
// sliders. (os.Stat polling at 100ms, dependency-free.)
func watchAndRebuild(
	files []string,
	build func() (string, func(), error),
	launch func(string) (*exec.Cmd, error),
	proc *exec.Cmd,
	cleanup func(),
	timing bool,
) int {
	watch := watchFiles(files)
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
			newProc, newCleanup, ok := rebuildAndSwap(build, launch, proc, cleanup, timing)
			if !ok {
				continue // rebuild failed; keep serving the previous binary
			}
			if newProc == nil {
				return 1 // relaunch failed; the old binary is already gone
			}
			proc, cleanup = newProc, newCleanup
		}
	}
}

// rebuildAndSwap builds a new binary (old one still serving), then stops the old
// and launches the new. ok is false when the rebuild failed (old binary kept);
// a nil proc with ok true means the relaunch failed after the old was stopped.
func rebuildAndSwap(
	build func() (string, func(), error),
	launch func(string) (*exec.Cmd, error),
	old *exec.Cmd,
	oldCleanup func(),
	timing bool,
) (proc *exec.Cmd, cleanup func(), ok bool) {
	buildT := time.Now()
	newBin, newCleanup, err := build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: rebuild failed (still serving previous build): %v\n", err)
		return nil, nil, false
	}
	buildDur := time.Since(buildT)

	swapT := time.Now()
	_ = old.Process.Kill()
	_, _ = old.Process.Wait()
	oldCleanup()
	newProc, err := launch(newBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: relaunch failed: %v\n", err)
		newCleanup()
		return nil, nil, true
	}
	if timing {
		fmt.Fprintf(os.Stderr, "rebuild+warm: %v   swap: %v   (edit reflected in %v)\n",
			buildDur, time.Since(swapT), time.Since(buildT))
	}
	return newProc, newCleanup, true
}

// buildBinary produces a fresh notebook binary: analyze → codegen → go build.
// It returns the binary path and a cleanup for its overlay temp dir. Errors in
// the notebook block the build; notices (deferred features) are printed.
//
// NOTE: an earlier version "warmed" the new binary with a throwaway headless run
// to pay the OS first-exec cost before the swap. Measured on lego, that was a
// net loss — warming runs a full wave (~460ms) for more than the ~180ms
// first-exec it saved. Removed; first-exec is paid at swap time, which is
// cheaper and honest.
func buildBinary(dir, moduleRoot string) (bin string, cleanup func(), err error) {
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
