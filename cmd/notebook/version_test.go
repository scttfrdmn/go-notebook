package main

import (
	"runtime/debug"
	"testing"
)

// TestResolveVersion pins #248: the documented `go tool notebook` path builds
// from module source with no ldflags, so the version must come from the build
// info — normalized to the same form GoReleaser stamps — or provenance silently
// loses toolVersion.
func TestResolveVersion(t *testing.T) {
	mod := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/scttfrdmn/go-notebook", Version: v}}
	}
	cases := []struct {
		name, ldflag string
		info         *debug.BuildInfo
		ok           bool
		want         string
	}{
		{"release ldflag wins", "0.5.1", mod("v9.9.9"), true, "0.5.1"},
		{"go tool: pinned module version, v trimmed", "dev", mod("v0.5.1"), true, "0.5.1"},
		{"go tool: pseudo-version kept", "dev", mod("v0.5.2-0.20261107010000-abcdef123456"), true, "0.5.2-0.20261107010000-abcdef123456"},
		{"source tree build", "dev", mod("(devel)"), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.ldflag, c.info, c.ok); got != c.want {
			t.Errorf("%s: resolveVersion = %q, want %q", c.name, got, c.want)
		}
	}
}
