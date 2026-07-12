package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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

	// launch starts a built binary serving on a fresh internal port and returns
	// the process plus its base URL. The supervisor owns the user-facing addr
	// and proxies here; the child never learns it is behind a proxy.
	launch := func(bin string) (*exec.Cmd, string, error) {
		childAddr, err := pickFreePort()
		if err != nil {
			return nil, "", fmt.Errorf("pick child port: %w", err)
		}
		nb := exec.Command(bin, "--addr", childAddr, "--head", headPath)
		nb.Stdout = os.Stdout
		nb.Stderr = os.Stderr
		if err := nb.Start(); err != nil {
			return nil, "", fmt.Errorf("launch: %w", err)
		}
		return nb, "http://" + childAddr, nil
	}

	// The supervisor owns the port and always answers — building, build-failed,
	// or crashed — so the browser never hits a dead port or a blank page.
	st := &status{phase: phaseBuilding}
	go func() {
		if err := http.ListenAndServe(*addr, newSupervisor(st)); err != nil {
			fmt.Fprintf(os.Stderr, "notebook run: supervisor: %v\n", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "notebook serving on http://%s\n", *addr)
	if !*noOpen {
		openBrowser("http://" + *addr)
	}

	buildStart := time.Now()
	bin, cleanup, err := build()
	if err != nil {
		// First build failed: the supervisor shows the error; keep watching so a
		// fix recovers without a restart.
		st.set(phaseBuildFailed, err.Error())
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		return watchAndRebuild(res.Package.GoFiles, build, launch, nil, nil, st, *timing)
	}
	proc, childURL, err := launch(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "notebook run: %v\n", err)
		cleanup()
		return 1
	}
	waitChildReady(childURL)
	st.setChild(childURL)
	if *timing {
		fmt.Fprintf(os.Stderr, "startup: %v\n", time.Since(buildStart))
	}

	return watchAndRebuild(res.Package.GoFiles, build, launch, proc, cleanup, st, *timing)
}

// waitChildReady blocks until the child's port answers, so the supervisor only
// flips to OK once the child can actually serve — never proxying to a
// not-yet-listening socket. Bounded so a wedged child can't hang startup.
func waitChildReady(baseURL string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", strings.TrimPrefix(baseURL, "http://"), 100*time.Millisecond); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
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
	launch func(string) (*exec.Cmd, string, error),
	proc *exec.Cmd,
	cleanup func(),
	st *status,
	timing bool,
) int {
	watch := watchFiles(files)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	// crash carries the identity of a child that exited on its own (a hard
	// panic) so the supervisor can show "the process died" instead of a dead
	// port. An INTENTIONAL kill (during a swap or on SIGINT) must NOT be reported
	// as a crash, so the exit watcher checks whether this exact process is the
	// current one before raising it — a swapped-out child is no longer current.
	crash := make(chan crashInfo, 1)
	watchExit := func(p *exec.Cmd) {
		if p == nil {
			return
		}
		go func() { crash <- crashInfo{proc: p, err: p.Wait()} }()
	}
	watchExit(proc)

	for {
		select {
		case <-sig:
			if proc != nil {
				p := proc
				proc = nil // clear first so the exit watcher treats the kill as intentional
				_ = p.Process.Kill()
			}
			if cleanup != nil {
				cleanup()
			}
			return 0
		case c := <-crash:
			// Only a crash if the exited process is STILL the current one. A child
			// we killed during a swap is already replaced, so proc != c.proc and we
			// ignore it — the intentional-kill vs crash distinction.
			if proc != nil && c.proc == proc {
				st.set(phaseCrashed, childExitDetail(c.err))
				fmt.Fprintf(os.Stderr, "notebook run: notebook process exited: %v\n", c.err)
				proc = nil
			}
		case <-watch:
			fmt.Fprintln(os.Stderr, "notebook run: change detected, rebuilding…")
			// Mark the outgoing child stopped BEFORE the swap kills it, so its exit
			// is recognized as intentional (proc is cleared; the killed child no
			// longer equals the current proc).
			old := proc
			proc = nil
			newProc, newCleanup, ok := rebuildAndSwap(build, launch, old, cleanup, st, timing)
			if !ok {
				// Rebuild failed: the old child (if any) is still alive and serving,
				// so restore it as current — but rebuildAndSwap only kills on a
				// SUCCESSFUL build, so old is untouched here.
				proc = old
				continue
			}
			proc, cleanup = newProc, newCleanup
			watchExit(proc)
		}
	}
}

// crashInfo pairs an exited child with its exit error, so the watcher can tell
// whether the process that died is still the current one (a crash) or one we
// already swapped away (an intentional kill).
type crashInfo struct {
	proc *exec.Cmd
	err  error
}

// childExitDetail renders a child process's exit into a human reason.
func childExitDetail(err error) string {
	if err == nil {
		return "the notebook process exited unexpectedly"
	}
	return "the notebook process exited: " + err.Error()
}

// rebuildAndSwap builds a new binary (old one still serving), then stops the old
// and launches the new, driving the supervisor's status through each outcome.
// ok is false when the rebuild failed (old child kept, error shown); on a
// successful rebuild it returns the new process + cleanup.
func rebuildAndSwap(
	build func() (string, func(), error),
	launch func(string) (*exec.Cmd, string, error),
	old *exec.Cmd,
	oldCleanup func(),
	st *status,
	timing bool,
) (proc *exec.Cmd, cleanup func(), ok bool) {
	buildT := time.Now()
	newBin, newCleanup, err := build()
	if err != nil {
		// Build failed: keep the last-good child serving and overlay the error.
		// This is the "never blank the page on a broken save" case.
		st.set(phaseBuildFailed, err.Error())
		fmt.Fprintf(os.Stderr, "notebook run: rebuild failed (still serving previous build): %v\n", err)
		return nil, nil, false
	}
	buildDur := time.Since(buildT)

	swapT := time.Now()
	if old != nil {
		_ = old.Process.Kill()
		_, _ = old.Process.Wait()
	}
	if oldCleanup != nil {
		oldCleanup()
	}
	newProc, childURL, err := launch(newBin)
	if err != nil {
		st.set(phaseCrashed, "relaunch failed: "+err.Error())
		fmt.Fprintf(os.Stderr, "notebook run: relaunch failed: %v\n", err)
		newCleanup()
		return nil, nil, false
	}
	waitChildReady(childURL)
	st.setChild(childURL) // flips the supervisor back to OK, clearing any error
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
	// Tee the compiler output to a buffer as well as the terminal, so the actual
	// message (not just "exit status 1") reaches the browser status. A go build
	// failure here is almost always a codegen bug — user errors (type errors,
	// graph errors) are caught earlier by analyze.LoadPackage and already point
	// at the user's file. That distinction is why the posmap is unnecessary: this
	// path SHOULD name the generated source, because that's where such a bug is.
	var buildErr bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &buildErr)
	if err := cmd.Run(); err != nil {
		overlay.Cleanup()
		msg := strings.TrimSpace(buildErr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", nil, fmt.Errorf("go build failed:\n%s", msg)
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
