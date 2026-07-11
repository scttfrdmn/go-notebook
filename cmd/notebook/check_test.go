package main

import "testing"

// TestCheckCleanNotebook exercises the check command end to end on the capacity
// example fixture (a valid notebook): it must exit 0.
func TestCheckCleanNotebook(t *testing.T) {
	if code := cmdCheck([]string{"../../examples/capacity"}); code != 0 {
		t.Errorf("check on a clean notebook returned exit %d, want 0", code)
	}
}

// TestCheckBadNotebook confirms a notebook with graph errors exits non-zero.
func TestCheckBadNotebook(t *testing.T) {
	if code := cmdCheck([]string{"../../internal/analyze/testdata/errors/missing"}); code == 0 {
		t.Error("check on a broken notebook returned exit 0, want non-zero")
	}
}

// TestCheckMissingArg confirms usage errors return exit 2.
func TestCheckMissingArg(t *testing.T) {
	if code := cmdCheck(nil); code != 2 {
		t.Errorf("check with no target returned exit %d, want 2", code)
	}
}

// TestRunUnknownSubcommand confirms an unknown subcommand returns exit 2.
func TestRunUnknownSubcommand(t *testing.T) {
	if code := run([]string{"frobnicate"}); code != 2 {
		t.Errorf("unknown subcommand returned exit %d, want 2", code)
	}
}
