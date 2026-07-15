//go:notebook
//
// The critical path — where speeding up a task does nothing.
//
// A build pipeline is a DAG of tasks with durations and dependencies: you can't test
// before you compile, can't deploy before you package. The wall-clock time to finish
// is not the sum of the tasks and it's not the longest single task — it's the longest
// *dependency chain*, the **critical path**. Every task off that path has **slack**:
// room to run late (or slow) without delaying the finish at all.
//
// The consequence is the thing this notebook exists to show, because it is where
// intuition fails: **speeding up a task with slack changes nothing.** Drag the lint
// slider down to zero and the pipeline finishes at the exact same time, because lint
// was never on the critical path — it was already waiting on compile. The only way to
// finish sooner is to shorten a task that is *on* the path. And when you do, the
// critical path can jump to a different chain — the bottleneck moves, and now a task
// that used to have slack is the one that matters.
//
// This is the classic project-scheduling method (CPM: forward pass for earliest
// start, backward pass for latest start, slack = latest − earliest), and it is a real
// tool — a CI engineer reads it to know which job to parallelize or cache. It is also
// the reflexive notebook: go-notebook derives a dependency graph from *this file's*
// cells and draws it at the top, and the content of the notebook is *another*
// dependency graph, the build DAG, drawn below. Same shape, one level apart.
//
// Pure: earliest/latest/slack are a forward and backward pass over the fixed DAG, a
// function of the duration sliders alone. No fold; scrub freely.

package critpath

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// The build DAG — fixed topology, slider-driven durations (seconds).
// ---------------------------------------------------------------------------
//
// checkout → deps → compile → test → package → deploy
//                 ↘ lint  ↗          (lint and compile both feed test)
// The interesting structure: compile and lint run in parallel after deps and both
// gate test, so which of them is on the critical path depends on their durations.

// Checkout duration (seconds). The root; everything waits on it.
//
//notebook:slider min=1 max=30 step=1
func checkout() (checkout int) { return 5 }

// Dependency fetch/restore duration (seconds).
//
//notebook:slider min=1 max=60 step=1
func deps() (deps int) { return 20 }

// Compile duration (seconds). Runs in parallel with lint; usually the long pole.
//
//notebook:slider min=1 max=120 step=1
func compile() (compile int) { return 60 }

// Lint duration (seconds). Parallel with compile — drag it around and watch whether
// it ever touches the critical path (it doesn't, until it's longer than compile).
//
//notebook:slider min=1 max=120 step=1
func lint() (lint int) { return 15 }

// Test duration (seconds). Waits on BOTH compile and lint.
//
//notebook:slider min=1 max=120 step=1
func test() (test int) { return 40 }

// Package duration (seconds).
//
//notebook:slider min=1 max=60 step=1
func pkg() (pkg int) { return 12 }

// Deploy duration (seconds).
//
//notebook:slider min=1 max=60 step=1
func deploy() (deploy int) { return 8 }

// ---------------------------------------------------------------------------
// Compute (Go) — CPM forward/backward pass, pure.
// ---------------------------------------------------------------------------

// schedule builds the task graph from the durations, runs the critical-path method
// (forward pass → earliest start/finish; backward pass → latest start/finish; slack
// = latest − earliest), and marks the critical path (the tasks with zero slack).
// Pure in the seven durations.
func schedule(checkout, deps, compile, lint, test, pkg, deploy int) (plan Plan) {
	// nodes in a fixed topological order; edges are dependency lists by index.
	dur := []int{checkout, deps, compile, lint, test, pkg, deploy}
	name := []string{"checkout", "deps", "compile", "lint", "test", "package", "deploy"}
	depsOf := [][]int{
		{},     // checkout
		{0},    // deps ← checkout
		{1},    // compile ← deps
		{1},    // lint ← deps
		{2, 3}, // test ← compile, lint
		{4},    // package ← test
		{5},    // deploy ← package
	}
	n := len(dur)

	// forward pass: earliest finish = max over deps of their earliest finish, + dur.
	ef := make([]int, n)
	es := make([]int, n)
	for i := 0; i < n; i++ {
		start := 0
		for _, d := range depsOf[i] {
			if ef[d] > start {
				start = ef[d]
			}
		}
		es[i] = start
		ef[i] = start + dur[i]
	}
	// total = max earliest finish (the sinks).
	total := 0
	for i := 0; i < n; i++ {
		if ef[i] > total {
			total = ef[i]
		}
	}

	// successors, for the backward pass.
	succ := make([][]int, n)
	for i := 0; i < n; i++ {
		for _, d := range depsOf[i] {
			succ[d] = append(succ[d], i)
		}
	}
	// backward pass: latest finish = min over successors of their latest start;
	// a sink's latest finish is the total.
	lf := make([]int, n)
	ls := make([]int, n)
	for i := range lf {
		lf[i] = total
	}
	for i := n - 1; i >= 0; i-- {
		if len(succ[i]) > 0 {
			m := total
			for _, s := range succ[i] {
				if ls[s] < m {
					m = ls[s]
				}
			}
			lf[i] = m
		}
		ls[i] = lf[i] - dur[i]
	}

	tasks := make([]Task, n)
	for i := 0; i < n; i++ {
		tasks[i] = Task{
			Name: name[i], Dur: dur[i],
			ES: es[i], EF: ef[i], Slack: ls[i] - es[i],
			Deps: depsOf[i],
		}
	}
	return Plan{Tasks: tasks, Total: total}
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// The pipeline as a timeline: each task a bar placed at its earliest start, width =
// duration. Critical-path tasks (zero slack) are solid; tasks with slack are lighter
// and show their slack as a hollow tail — the room they have to slip. The finish line
// is the critical path's length.
//
//notebook:height=360
func timeline(plan Plan) (chart Chart) {
	return Chart{Plan: plan}
}

// The numbers: total pipeline time, which tasks are on the critical path, and the
// slack on the rest. Drag a slack task's slider and the total won't move; drag a
// critical one and it will.
func readout(plan Plan) (report Readout) {
	var crit []string
	for _, t := range plan.Tasks {
		if t.Slack == 0 {
			crit = append(crit, t.Name)
		}
	}
	return Readout{Cards: []Card{
		{Label: "pipeline time", Value: strconv.Itoa(plan.Total) + " s", Caption: "the critical path's length, not the sum"},
		{Label: "critical path", Value: strings.Join(crit, " → "), Caption: "zero slack — the only tasks worth speeding up"},
		{Label: "most slack", Value: mostSlack(plan)},
	}}
}

// The critical path — where speeding up a task does nothing.
func intro() (md Markdown) {
	return `A build pipeline is a DAG of tasks. The time to finish isn't the sum, and
isn't the longest task — it's the longest dependency *chain*, the **critical path**.
Everything off it has **slack**.

So here's the trap: **drag lint down to zero and the pipeline finishes at the same
time.** Lint runs parallel to compile and was already waiting — it has slack, so
speeding it up buys nothing. Only shortening a task *on* the critical path helps —
and when you do, the path can jump to another chain, moving the bottleneck. The
readout marks which tasks are critical (the only ones worth optimizing) and how much
slack the rest have.

It's the classic CPM method — forward pass, backward pass, slack = latest − earliest
— and pure, so scrub freely. Note the shape: the graph up top is *this notebook's*
cells; the graph below is the build it's scheduling. Same idea, one level apart.`
}

// ===========================================================================
// Helpers
// ===========================================================================

func mostSlack(plan Plan) string {
	best := Task{Slack: -1}
	for _, t := range plan.Tasks {
		if t.Slack > best.Slack {
			best = t
		}
	}
	if best.Slack <= 0 {
		return "none — every task is critical"
	}
	return best.Name + " (" + strconv.Itoa(best.Slack) + " s to spare)"
}

// ===========================================================================
// Types
// ===========================================================================

// Task is one node: its duration, earliest start/finish, slack, and dependency
// indices. Slack 0 ⇒ on the critical path.
type Task struct {
	Name   string
	Dur    int
	ES, EF int
	Slack  int
	Deps   []int
}

// Plan is the scheduled DAG plus the total (critical-path) time.
type Plan struct {
	Tasks []Task
	Total int
}

// Chart draws the pipeline as a slack-annotated timeline.
type Chart struct{ Plan Plan }

func (c Chart) Render() Rendered {
	p := c.Plan
	const w, pad, rowH, top = 820.0, 90.0, 34.0, 40.0
	h := top + float64(len(p.Tasks))*rowH + 30
	scale := 0.0
	span := p.Total + maxSlack(p)
	if span > 0 {
		scale = (w - pad - 30) / float64(span)
	}
	sx := func(t int) float64 { return pad + float64(t)*scale }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f">`, w, h)
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#fff"/>`, w, h)

	for i, t := range p.Tasks {
		y := top + float64(i)*rowH
		// task name
		fmt.Fprintf(&b, `<text x="%.0f" y="%.1f" font-family="sans-serif" font-size="12" fill="#1b3a6b" text-anchor="end">%s</text>`,
			pad-8, y+rowH*0.5, t.Name)
		x0 := sx(t.ES)
		bw := float64(t.Dur) * scale
		fill, stroke := "#c7d2fe", "#4338ca"
		if t.Slack == 0 {
			fill, stroke = "#4338ca", "#312e81" // critical: solid, dark
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" fill=%q stroke=%q stroke-width="1.5"/>`,
			x0, y+4, bw, rowH-14, fill, stroke)
		// duration label inside/after the bar
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="11" fill="#0f172a">%ds</text>`,
			x0+bw+4, y+rowH*0.5+2, t.Dur)
		// slack tail (hollow), the room to slip
		if t.Slack > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" fill="none" stroke="#94a3b8" stroke-width="1" stroke-dasharray="3 2"/>`,
				x0+bw, y+4, float64(t.Slack)*scale, rowH-14)
		}
	}
	// finish line at the critical-path total
	fx := sx(p.Total)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dc2626" stroke-width="2" stroke-dasharray="4 3"/>`,
		fx, top-6, fx, h-24)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="sans-serif" font-size="12" font-weight="700" fill="#dc2626" text-anchor="middle">finish: %d s</text>`,
		fx, h-8, p.Total)
	b.WriteString(`</svg>`)
	return Rendered{MIME: "image/svg+xml", Data: b.String()}
}

func maxSlack(p Plan) int {
	m := 0
	for _, t := range p.Tasks {
		if t.Slack > m {
			m = t.Slack
		}
	}
	return m
}

type Card struct{ Label, Value, Caption string }
type Readout struct{ Cards []Card }

type Rendered struct{ MIME, Data string }

type Markdown string

func (m Markdown) Render() Rendered { return Rendered{"text/markdown", string(m)} }
