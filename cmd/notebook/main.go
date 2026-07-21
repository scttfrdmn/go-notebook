// Command notebook is the go-notebook toolchain, invoked as `go tool notebook`.
//
// This milestone ships one subcommand:
//
//	notebook check <dir|file>   analyze only: print the graph, report
//	                            diagnostics, exit non-zero on error.
//
// Later milestones add `run` (serve) and `build` (emit a binary).
package main

import (
	"fmt"
	"os"

	"github.com/scttfrdmn/go-notebook/internal/gen"
)

// version is the build version, injected by the release build (GoReleaser
// ldflags: -X main.version=…). It stays "dev" for a plain `go build`/`go run`,
// so `notebook version` reports honestly whether it is a released binary.
var version = "dev"

// usage is printed for -h and on argument errors.
const usage = `notebook — a reactive notebook toolchain

usage:
  notebook check   <dir|file>   analyze a notebook and print its dependency graph
  notebook build   <dir|file>   analyze, generate a registry, and compile a binary
  notebook run     <dir|file>   build and serve the notebook in a browser
  notebook version               print the toolchain version

Run "notebook run ./examples/capacity" to open a notebook.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches a subcommand and returns the process exit code, so it is
// testable without spawning a process.
func run(args []string) int {
	// Stamp the tool version into every artifact codegen produces this run, so a
	// built notebook records which go-notebook generated it. A plain dev build
	// reports "dev"; provenance treats that as un-versioned (empty), so only a
	// released tool stamps a real version.
	if version != "dev" {
		gen.ToolVersion = version
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "check":
		return cmdCheck(args[1:])
	case "build":
		return cmdBuild(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("notebook %s\n", version)
		return 0
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "notebook: unknown subcommand %q\n\n%s", args[0], usage)
		return 2
	}
}
