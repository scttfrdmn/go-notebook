package critpath

import (
	"strings"
	"testing"
)

// def schedules the notebook's default durations.
func def() Plan { return schedule(5, 20, 60, 15, 40, 12, 8) }

// find returns a task by name.
func find(p Plan, name string) Task {
	for _, t := range p.Tasks {
		if t.Name == name {
			return t
		}
	}
	panic("no task " + name)
}

// TestSchedulePure confirms the CPM computation is a pure function of the durations.
func TestSchedulePure(t *testing.T) {
	a, b := def(), def()
	if a.Total != b.Total {
		t.Fatalf("total not pure: %d vs %d", a.Total, b.Total)
	}
}

// TestTotalIsLongestChain: the finish time is the longest dependency chain, not the
// sum and not the longest task. Default chain checkout+deps+compile+test+package+
// deploy = 5+20+60+40+12+8 = 145.
func TestTotalIsLongestChain(t *testing.T) {
	p := def()
	if p.Total != 145 {
		t.Errorf("total = %d, want 145 (the critical chain)", p.Total)
	}
	// sanity: it's less than the sum of ALL tasks (which includes the parallel lint).
	sum := 5 + 20 + 60 + 15 + 40 + 12 + 8
	if p.Total >= sum {
		t.Errorf("total %d should be less than the sum of all tasks %d — lint runs in parallel", p.Total, sum)
	}
}

// TestLintHasSlack is the teaching claim: lint runs parallel to compile, so it has
// slack (compile 60 − lint 15 = 45), while everything on the compile chain has zero.
func TestLintHasSlack(t *testing.T) {
	p := def()
	if s := find(p, "lint").Slack; s != 45 {
		t.Errorf("lint slack = %d, want 45 (compile 60 − lint 15)", s)
	}
	for _, n := range []string{"checkout", "deps", "compile", "test", "package", "deploy"} {
		if s := find(p, n).Slack; s != 0 {
			t.Errorf("%s should be critical (slack 0), got %d", n, s)
		}
	}
}

// TestSpeedingUpSlackTaskDoesNothing is the trap, quantified: with lint off the
// critical path, changing its duration (down OR up, within its slack) leaves the
// total unchanged.
func TestSpeedingUpSlackTaskDoesNothing(t *testing.T) {
	base := schedule(5, 20, 60, 15, 40, 12, 8).Total
	faster := schedule(5, 20, 60, 1, 40, 12, 8).Total  // lint → 1
	slower := schedule(5, 20, 60, 55, 40, 12, 8).Total // lint → 55 (still < 60)
	if faster != base || slower != base {
		t.Errorf("changing lint within its slack moved the total: base=%d faster=%d slower=%d", base, faster, slower)
	}
}

// TestCriticalPathJumps: push lint past compile and the critical path moves onto lint
// — the bottleneck shifts, and the total finally grows.
func TestCriticalPathJumps(t *testing.T) {
	p := schedule(5, 20, 60, 100, 40, 12, 8) // lint 100 > compile 60
	if find(p, "lint").Slack != 0 {
		t.Errorf("with lint=100 (> compile), lint should be ON the critical path (slack 0), got %d", find(p, "lint").Slack)
	}
	if find(p, "compile").Slack == 0 {
		t.Error("with lint=100, compile should now have slack (path jumped off it)")
	}
	if p.Total != 5+20+100+40+12+8 {
		t.Errorf("total = %d, want %d (critical chain now via lint)", p.Total, 5+20+100+40+12+8)
	}
}

// TestEarliestStartsRespectDeps: no task starts before all its dependencies finish.
func TestEarliestStartsRespectDeps(t *testing.T) {
	p := def()
	for _, task := range p.Tasks {
		for _, di := range task.Deps {
			dep := p.Tasks[di]
			if task.ES < dep.EF {
				t.Errorf("%s starts at %d but its dep %s finishes at %d", task.Name, task.ES, dep.Name, dep.EF)
			}
		}
	}
}

// TestTimelineRenders: the chart renders SVG, shows the finish line, and draws a bar
// per task.
func TestTimelineRenders(t *testing.T) {
	data := timeline(def()).Render().Data
	if !strings.HasPrefix(data, "<svg") {
		t.Fatal("timeline is not SVG")
	}
	for _, want := range []string{"finish: 145 s", "checkout", "compile", "lint", "deploy"} {
		if !strings.Contains(data, want) {
			t.Errorf("timeline missing %q", want)
		}
	}
}
