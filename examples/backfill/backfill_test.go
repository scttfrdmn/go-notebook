package backfill

import (
	"strings"
	"testing"
)

const (
	fifo     = 0
	backfill = 1
)

// TestScheduleIsPure: schedule is a pure function of (work, policy) — same inputs,
// identical plan. No clock, no RNG at schedule time.
func TestScheduleIsPure(t *testing.T) {
	q := queue(24, 8)
	a := schedule(q, backfill)
	b := schedule(q, backfill)
	for i := range a.Start {
		if a.Start[i] != b.Start[i] {
			t.Fatalf("schedule not pure: job %d start %d vs %d", i, a.Start[i], b.Start[i])
		}
	}
}

// TestJobSetIsPolicyInvariant is the anti-pass guard: the ONLY thing that may differ
// between the two Gantt charts is the placement, never the work. Same seed ⇒ the job
// set (arrivals, widths, durations) is byte-identical regardless of policy. If this
// ever fails, any wait-time comparison is comparing two different experiments.
func TestJobSetIsPolicyInvariant(t *testing.T) {
	q := queue(24, 8)
	f := schedule(q, fifo)
	b := schedule(q, backfill)
	if len(f.Jobs) != len(b.Jobs) {
		t.Fatalf("job counts differ: %d vs %d", len(f.Jobs), len(b.Jobs))
	}
	for i := range f.Jobs {
		if f.Jobs[i] != b.Jobs[i] {
			t.Errorf("job %d differs across policy: %+v vs %+v — the job set must be invariant",
				i, f.Jobs[i], b.Jobs[i])
		}
	}
}

// TestWaitsAreRealNotAssumed: every job's wait is start − arrival, and start is where
// it actually landed on the grid. Assert the schedule never places a job before it
// arrived, and that its bar fits within capacity at every tick it occupies.
func TestWaitsAreRealNotAssumed(t *testing.T) {
	q := queue(24, 8)
	for _, pol := range []int{fifo, backfill} {
		p := schedule(q, pol)
		used := make([]int, horizon)
		for _, j := range p.Jobs {
			st := p.Start[j.ID]
			if st < j.Arrive {
				t.Errorf("policy %d: job %d starts at %d before arrival %d", pol, j.ID, st, j.Arrive)
			}
			for tk := st; tk < st+j.Dur; tk++ {
				used[tk] += j.Width
				if used[tk] > q.Nodes {
					t.Fatalf("policy %d: overcommit at tick %d — %d used > %d nodes", pol, tk, used[tk], q.Nodes)
				}
			}
		}
	}
}

// TestBackfillCutsMedianWait is THE teaching claim: on the same jobs and the same
// nodes, EASY backfill produces a lower median wait than FIFO.
func TestBackfillCutsMedianWait(t *testing.T) {
	q := queue(24, 8)
	f := schedule(q, fifo)
	b := schedule(q, backfill)
	if !(b.MedianWait < f.MedianWait) {
		t.Errorf("backfill should cut median wait: FIFO=%.1f backfill=%.1f", f.MedianWait, b.MedianWait)
	}
}

// TestBackfillDoesNotDelayTheHead is the EASY safety guarantee — the property that
// makes backfill safe to turn on, and the one the notebook's headline claim rests on:
// the head job (first in queue order) starts no LATER under backfill than under FIFO.
// If backfill ever pushed the blocking job out, the "nobody at the front pays" claim
// would be a lie.
func TestBackfillDoesNotDelayTheHead(t *testing.T) {
	q := queue(24, 8)
	f := schedule(q, fifo)
	b := schedule(q, backfill)

	// head = earliest arrival, ties by ID (the queue order the scheduler uses).
	head := q.Jobs[0]
	for _, j := range q.Jobs {
		if j.Arrive < head.Arrive || (j.Arrive == head.Arrive && j.ID < head.ID) {
			head = j
		}
	}
	if b.Start[head.ID] > f.Start[head.ID] {
		t.Errorf("backfill delayed the head job #%d: FIFO start=%d, backfill start=%d — violates the EASY guarantee",
			head.ID, f.Start[head.ID], b.Start[head.ID])
	}
}

// TestBackfillDelaysNobody is the stronger honest form of the guarantee for this job
// set: no job waits longer under backfill than it did under FIFO. (EASY guarantees the
// reservation holder isn't delayed; for these deterministic sets no job is, and the
// notebook's "nobody pays" framing should hold — if a future default breaks it, this
// catches it so the prose can be corrected rather than shipped false.)
func TestBackfillDelaysNobody(t *testing.T) {
	q := queue(24, 8)
	f := schedule(q, fifo)
	b := schedule(q, backfill)
	delayed := 0
	for _, j := range q.Jobs {
		fw := f.Start[j.ID] - j.Arrive
		bw := b.Start[j.ID] - j.Arrive
		if bw > fw {
			delayed++
		}
	}
	if delayed > 0 {
		t.Errorf("%d job(s) wait longer under backfill than FIFO — the 'nobody pays' claim needs qualifying", delayed)
	}
}

// TestFifoBlocksBehindWideJobs: sanity that FIFO's head-of-line blocking is real — its
// mean wait should be at least as large as backfill's (the queue backs up behind the
// hole). Guards against a FIFO that silently already backfills.
func TestFifoBlocksMore(t *testing.T) {
	q := queue(24, 8)
	f := schedule(q, fifo)
	b := schedule(q, backfill)
	if !(f.MeanWait >= b.MeanWait) {
		t.Errorf("FIFO mean wait %.1f should be ≥ backfill %.1f (head-of-line blocking)", f.MeanWait, b.MeanWait)
	}
}

// TestGanttAndStatsRender: the Gantt is SVG with its orienting labels; the stats
// readout is HTML (else the client hides it — the pid/Readout lesson).
func TestGanttAndStatsRender(t *testing.T) {
	p := schedule(queue(24, 8), backfill)

	gd := gantt(p).Render()
	if !strings.HasPrefix(gd.Data, "<svg") {
		t.Fatal("gantt not SVG")
	}
	for _, w := range []string{"Gantt", "time", "nodes", "<rect"} {
		if !strings.Contains(gd.Data, w) {
			t.Errorf("gantt missing %q", w)
		}
	}

	sd := stats(p).Render()
	if sd.MIME != "text/html" {
		t.Fatalf("stats MIME = %q, want text/html (else the client hides it)", sd.MIME)
	}
	for _, w := range []string{"median wait", "mean wait", "makespan"} {
		if !strings.Contains(sd.Data, w) {
			t.Errorf("stats missing %q", w)
		}
	}
}
